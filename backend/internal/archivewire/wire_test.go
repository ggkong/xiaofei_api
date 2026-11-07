package archivewire

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
)

func TestBoundedFramesAndVerifiedBlocks(t *testing.T) {
	var frame bytes.Buffer
	if err := Write(&frame, Request{Operation: "hello"}, MaxRequest); err != nil {
		t.Fatal(err)
	}
	var request Request
	if err := Read(&frame, &request, MaxRequest); err != nil || request.Operation != "hello" {
		t.Fatal(err)
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], MaxRequest+1)
	if err := Read(bytes.NewReader(header[:]), &request, MaxRequest); err == nil {
		t.Fatal("oversized frame accepted")
	}
	raw := json.RawMessage(`{"number":9007199254740993,"content":"你好"}`)
	var payload bytes.Buffer
	writer := gzip.NewWriter(&payload)
	if _, err := writer.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	block := Block{Hash: Hash(raw), Payload: payload.Bytes(), RawBytes: len(raw)}
	decoded, err := block.Decode()
	if err != nil || !bytes.Equal(decoded, raw) {
		t.Fatal("lossless block restore", err)
	}
	packed := json.RawMessage(`{"_format":1,"fields":{"messages":{"kind":"array","items":[{"kind":"blob","hash":"` + block.Hash + `"}]}}}`)
	restored, err := Restore(packed, 1, func(string) (json.RawMessage, error) { return block.Decode() })
	if err != nil || !strings.Contains(string(restored), "9007199254740993") {
		t.Fatal("number changed", err)
	}
	block.Hash = strings.Repeat("0", 64)
	if _, err = block.Decode(); err == nil {
		t.Fatal("corrupt block accepted")
	}
	block.Hash = Hash(raw)
	block.RawBytes = MaxBlock + 1
	if _, err = block.Decode(); err == nil {
		t.Fatal("decompression bound missing")
	}
}
func TestEntryIdentityAndRootSnapshot(t *testing.T) {
	entry := Entry{ID: 1, RequestID: "r", Metadata: json.RawMessage(`{"id":2,"request_id":"r"}`), RequestSnapshot: json.RawMessage(`null`), ReplySnapshot: json.RawMessage(`null`)}
	if _, err := entry.Refs(); err == nil {
		t.Fatal("mismatched identity accepted")
	}
	raw, err := Restore(json.RawMessage(`{"_format":1,"root":{"kind":"inline","raw":[1,"x",9007199254740993]}}`), 1, nil)
	if err != nil || string(raw) != `[1,"x",9007199254740993]` {
		t.Fatal("scalar root changed", err)
	}
}
