package conversationarchive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/lib/pq"
)

// PromptDigest is computed before the bounded capture is made. A long prompt
// must never be grouped merely because its retained prefix matches another one.
func PromptDigest(body []byte) string {
	prompt := PromptText(body, len(body)+1)
	if prompt == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(hash[:])
}

func prepareGrouping(r *Record) {
	if r.PromptHash == "" {
		r.PromptHash = PromptDigest(r.RequestBody)
	}
	identity := "unknown/" + r.RequestID
	// Formatting pointer addresses would split one member's requests randomly.
	if r.UserID != nil && r.APIKeyID != nil {
		identity = fmt.Sprintf("%d/%d", *r.UserID, *r.APIKeyID)
	} else {
		identity = "unknown/" + r.RequestID
	}
	var key string
	switch {
	case r.SessionID != "":
		r.GroupSource = "session"
		key = identity + "/session/" + r.SessionID
	case r.PromptHash != "":
		r.GroupSource = "question"
		key = identity + "/question/" + r.CreatedAt.In(time.Local).Format("2006-01-02") + "/" + r.PromptHash
	default:
		r.GroupSource = "unavailable"
		key = identity + "/unavailable/" + r.CreatedAt.In(time.Local).Format("2006-01-02")
	}
	hash := sha256.Sum256([]byte(key))
	r.ConversationKey = hex.EncodeToString(hash[:])
}

const groupKeySQL = `CASE WHEN c.conversation_key<>'' THEN c.conversation_key ELSE 'record:'||c.id::text END`

// ListGroups paginates whole groups by their newest call. The filters select
// matching groups; counts and models describe every retained call in that group.
func (s *Store) ListGroups(ctx context.Context, f Filter) ([]Record, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("archive unavailable")
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 30
	}
	before := f.BeforeID
	f.BeforeID = 0
	args := append(filterArgs(f), before)
	query := `WITH matching AS (SELECT DISTINCT ` + groupKeySQL + ` AS group_key FROM conversation_archives c LEFT JOIN users u ON u.id=c.user_id` + archiveWhere + `), grouped AS (
 SELECT MAX(c.id) AS latest_id,COUNT(*) AS call_count,MIN(c.created_at) AS first_at,
 array_remove(array_agg(DISTINCT COALESCE(NULLIF(CASE WHEN c.storage_version=1 THEN c.response_body#>>'{_meta,upstream_model}' ELSE c.response_body#>>'{_sub2api_archive,upstream_model}' END,''),NULLIF(c.model,''))),NULL) AS models,
 COUNT(*) FILTER(WHERE c.http_status=403 AND (c.response_body->>'_blocked'='true' OR c.response_body#>>'{error,code}'='content_policy_violation' OR c.response_body#>>'{error,type}'='content_policy_violation')) AS blocked_count,
 COUNT(*) FILTER(WHERE c.response_body->>'keyword_auto_reply'='true') AS auto_reply_count
 FROM conversation_archives c WHERE c.expires_at>NOW() AND ` + groupKeySQL + ` IN (SELECT group_key FROM matching)
 GROUP BY ` + groupKeySQL + ` HAVING ($12::bigint=0 OR MAX(c.id)<$12)
 ORDER BY latest_id DESC LIMIT $7) SELECT latest_id,call_count,first_at,models,blocked_count,auto_reply_count FROM grouped ORDER BY latest_id DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	type group struct {
		id, count, blocked, replies int64
		first                       time.Time
		models                      pq.StringArray
	}
	groups := []group{}
	ids := []int64{}
	for rows.Next() {
		var g group
		if err = rows.Scan(&g.id, &g.count, &g.first, &g.models, &g.blocked, &g.replies); err != nil {
			_ = rows.Close()
			return nil, err
		}
		groups = append(groups, g)
		ids = append(ids, g.id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []Record{}, nil
	}
	details, err := s.db.QueryContext(ctx, `SELECT `+recordColumns+` FROM conversation_archives c LEFT JOIN users u ON u.id=c.user_id WHERE c.id=ANY($1::bigint[])`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer func() { _ = details.Close() }()
	records := make(map[int64]Record)
	for details.Next() {
		r, err := scanRecord(details, false)
		if err != nil {
			return nil, err
		}
		records[r.ID] = r
	}
	if err = details.Err(); err != nil {
		return nil, err
	}
	result := make([]Record, 0, len(groups))
	for _, g := range groups {
		r, exists := records[g.id]
		if !exists {
			continue
		}
		if r.ConversationKey == "" {
			r.ConversationKey = fmt.Sprintf("record:%d", r.ID)
		}
		r.CallCount, r.FirstAt, r.BlockedCount, r.AutoReplyCount = g.count, &g.first, g.blocked, g.replies
		unique := map[string]bool{}
		for _, model := range g.models {
			if model != "" {
				unique[model] = true
			}
		}
		r.Models = make([]string, 0, len(unique))
		for model := range unique {
			r.Models = append(r.Models, model)
		}
		sort.Strings(r.Models)
		result = append(result, r)
	}
	return result, nil
}
