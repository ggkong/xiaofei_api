package conversationarchive

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestGroupingPreservesIdentityAndExactQuestions(t *testing.T) {
	user, key, otherKey := int64(1), int64(11), int64(12)
	date := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	makeRecord := func(question string) Record {
		body, _ := json.Marshal(map[string]any{"messages": []map[string]string{{"role": "user", "content": question}}})
		r := Record{UserID: &user, APIKeyID: &key, RequestID: "original", RequestBody: body, CreatedAt: date}
		prepareGrouping(&r)
		return r
	}
	original := makeRecord(strings.Repeat("long question ", 1000) + "A")
	different := makeRecord(strings.Repeat("long question ", 1000) + "B")
	if original.ConversationKey == different.ConversationKey {
		t.Fatal("different long questions merged by a shared preview prefix")
	}
	same := makeRecord(strings.Repeat("long question ", 1000) + "A")
	if original.ConversationKey != same.ConversationKey || same.GroupSource != "question" {
		t.Fatal("identical question not grouped")
	}
	differentKey := original
	differentKey.APIKeyID = &otherKey
	prepareGrouping(&differentKey)
	if differentKey.ConversationKey == original.ConversationKey {
		t.Fatal("different API keys merged")
	}
	tomorrow := original
	tomorrow.CreatedAt = date.Add(24 * time.Hour)
	prepareGrouping(&tomorrow)
	if tomorrow.ConversationKey == original.ConversationKey {
		t.Fatal("questions from different days merged without a session")
	}
	sessionOne, sessionTwo := original, different
	sessionOne.SessionID = "real-session"
	sessionTwo.SessionID = "real-session"
	prepareGrouping(&sessionOne)
	prepareGrouping(&sessionTwo)
	if sessionOne.ConversationKey != sessionTwo.ConversationKey || sessionOne.GroupSource != "session" {
		t.Fatal("real session was split by question")
	}
	missingOne, missingTwo := original, original
	missingOne.UserID = nil
	missingTwo.UserID = nil
	missingTwo.RequestID = "another"
	prepareGrouping(&missingOne)
	prepareGrouping(&missingTwo)
	if missingOne.ConversationKey == missingTwo.ConversationKey {
		t.Fatal("unknown member identities merged")
	}
}
