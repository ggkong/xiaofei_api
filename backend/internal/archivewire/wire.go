// Package archivewire defines the bounded archive-only protocol used over SSH.
package archivewire

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
)

const Version = 1
const MaxRequest = 64 << 10
const MaxResponse = 96 << 20
const MaxBlock = 32 << 20

type Request struct {
	Operation string   `json:"operation"`
	AfterID   int64    `json:"after_id,omitempty"`
	UpperID   int64    `json:"upper_id,omitempty"`
	ID        int64    `json:"id,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	Hashes    []string `json:"hashes,omitempty"`
	Digest    string   `json:"digest,omitempty"`
	Confirmed int64    `json:"confirmed,omitempty"`
}
type Item struct {
	ID        int64  `json:"id"`
	RequestID string `json:"request_id"`
	DeviceID  string `json:"device_id,omitempty"`
	Digest    string `json:"digest,omitempty"`
}
type Entry struct {
	ID              int64           `json:"id"`
	RequestID       string          `json:"request_id"`
	StorageVersion  int             `json:"storage_version"`
	Metadata        json.RawMessage `json:"metadata"`
	RequestSnapshot json.RawMessage `json:"request_snapshot"`
	ReplySnapshot   json.RawMessage `json:"reply_snapshot"`
}
type Block struct {
	Hash     string `json:"hash"`
	Payload  []byte `json:"payload"`
	RawBytes int    `json:"raw_bytes"`
}
type Response struct {
	Version        int      `json:"version"`
	Error          string   `json:"error,omitempty"`
	Items          []Item   `json:"items,omitempty"`
	UpperID        int64    `json:"upper_id,omitempty"`
	NextID         int64    `json:"next_id,omitempty"`
	Entry          *Entry   `json:"entry,omitempty"`
	Digest         string   `json:"digest,omitempty"`
	Blocks         []Block  `json:"blocks,omitempty"`
	Remaining      []string `json:"remaining,omitempty"`
	Moved          bool     `json:"moved"`
	OffloadEnabled bool     `json:"offload_enabled"`
	KeepDays       int      `json:"keep_days"`
}

func Write(w io.Writer, value any, limit int) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(raw) > limit {
		return errors.New("archive frame exceeds limit")
	}
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(raw)))
	if _, err = io.Copy(w, bytes.NewReader(size[:])); err != nil {
		return err
	}
	_, err = io.Copy(w, bytes.NewReader(raw))
	return err
}
func Read(r io.Reader, value any, limit int) error {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || uint64(size) > uint64(limit) {
		return errors.New("invalid archive frame size")
	}
	raw := make([]byte, int(size))
	if _, err := io.ReadFull(r, raw); err != nil {
		return err
	}
	return json.Unmarshal(raw, value)
}
func Hash(raw []byte) string { digest := sha256.Sum256(raw); return hex.EncodeToString(digest[:]) }
func ValidHash(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	_, err := hex.DecodeString(hash)
	return err == nil
}
func (e Entry) Bytes() ([]byte, error)  { return json.Marshal(e) }
func (e Entry) Digest() (string, error) { raw, err := e.Bytes(); return Hash(raw), err }
func (e Entry) Refs() ([]string, error) {
	if e.ID < 1 || e.RequestID == "" || !json.Valid(e.Metadata) || !json.Valid(e.RequestSnapshot) || !json.Valid(e.ReplySnapshot) || (e.StorageVersion != 0 && e.StorageVersion != 1) {
		return nil, errors.New("invalid archive entry")
	}
	refs := []string{}
	var identity struct {
		ID        int64  `json:"id"`
		RequestID string `json:"request_id"`
	}
	if json.Unmarshal(e.Metadata, &identity) != nil || identity.ID != e.ID || identity.RequestID != e.RequestID {
		return nil, errors.New("archive metadata identity mismatch")
	}
	seen := map[string]bool{}
	if e.StorageVersion == 0 {
		return refs, nil
	}
	for _, raw := range []json.RawMessage{e.RequestSnapshot, e.ReplySnapshot} {
		var packed struct {
			Format int      `json:"_format"`
			Refs   []string `json:"_refs"`
		}
		if json.Unmarshal(raw, &packed) != nil || packed.Format != 1 {
			return nil, errors.New("invalid archive snapshot")
		}
		for _, hash := range packed.Refs {
			if !ValidHash(hash) {
				return nil, errors.New("invalid archive reference")
			}
			if !seen[hash] {
				refs = append(refs, hash)
				seen[hash] = true
			}
		}
	}
	return refs, nil
}
func (b Block) Decode() (json.RawMessage, error) {
	if !ValidHash(b.Hash) || b.RawBytes < 0 || b.RawBytes > MaxBlock || len(b.Payload) > MaxBlock+65536 {
		return nil, errors.New("invalid archive block")
	}
	reader, err := gzip.NewReader(bytes.NewReader(b.Payload))
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(reader, int64(b.RawBytes)+1))
	_ = reader.Close()
	if err != nil || len(raw) != b.RawBytes || !json.Valid(raw) || Hash(raw) != b.Hash {
		return nil, errors.New("archive block checksum failed")
	}
	return raw, nil
}

type value struct {
	Kind  string          `json:"kind"`
	Raw   json.RawMessage `json:"raw"`
	Hash  string          `json:"hash"`
	Items []value         `json:"items"`
}

func Restore(raw json.RawMessage, version int, lookup func(string) (json.RawMessage, error)) (json.RawMessage, error) {
	if version == 0 {
		if !json.Valid(raw) {
			return nil, errors.New("invalid archive JSON")
		}
		return raw, nil
	}
	var snapshot struct {
		Format int              `json:"_format"`
		Fields map[string]value `json:"fields"`
		Root   *value           `json:"root"`
	}
	if version != 1 || json.Unmarshal(raw, &snapshot) != nil || snapshot.Format != 1 {
		return nil, errors.New("invalid archive snapshot")
	}
	var restore func(value, int) (json.RawMessage, error)
	restore = func(v value, depth int) (json.RawMessage, error) {
		if depth > 32 {
			return nil, errors.New("archive nesting exceeds limit")
		}
		switch v.Kind {
		case "inline":
			if !json.Valid(v.Raw) {
				return nil, errors.New("invalid inline archive JSON")
			}
			return v.Raw, nil
		case "blob":
			if !ValidHash(v.Hash) {
				return nil, errors.New("invalid archive hash")
			}
			return lookup(v.Hash)
		case "array":
			items := make([]json.RawMessage, 0, len(v.Items))
			for _, item := range v.Items {
				decoded, err := restore(item, depth+1)
				if err != nil {
					return nil, err
				}
				items = append(items, decoded)
			}
			return json.Marshal(items)
		default:
			return nil, errors.New("invalid archive value")
		}
	}
	if snapshot.Root != nil {
		return restore(*snapshot.Root, 0)
	}
	fields := map[string]json.RawMessage{}
	for name, part := range snapshot.Fields {
		decoded, err := restore(part, 0)
		if err != nil {
			return nil, err
		}
		fields[name] = decoded
	}
	return json.Marshal(fields)
}
