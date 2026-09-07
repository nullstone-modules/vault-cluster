package s3

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

type memObjects map[string][]byte

func (m memObjects) Put(_ context.Context, bucket, key string, body []byte) error {
	m[bucket+"/"+key] = append([]byte(nil), body...)
	return nil
}

func (m memObjects) List(_ context.Context, bucket, prefix string) ([]string, error) {
	var out []string
	root := bucket + "/"
	for k := range m {
		if !strings.HasPrefix(k, root+prefix) {
			continue
		}
		key := strings.TrimPrefix(k, root)
		if strings.HasSuffix(key, ".snap") {
			out = append(out, "s3://"+bucket+"/"+key)
		}
	}
	return out, nil
}

func TestObjectKey(t *testing.T) {
	got := ObjectKey("vault-snapshots", "20260101T000000Z")
	if got != "vault-snapshots/vault-20260101T000000Z.snap" {
		t.Fatalf("key %q", got)
	}
}

func TestListSnapshots(t *testing.T) {
	store := memObjects{
		"b/vault-snapshots/vault-1.snap":        []byte("a"),
		"b/vault-snapshots/vault-1.snap.sha256": []byte("x"),
		"b/other/vault-2.snap":                  []byte("c"),
	}
	got, err := ListSnapshots(store, "b", "vault-snapshots")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "s3://b/vault-snapshots/vault-1.snap" {
		t.Fatalf("list %v", got)
	}
}

func TestPutSnapshotWritesChecksum(t *testing.T) {
	store := memObjects{}
	uri, err := PutSnapshot(store, "b", "", []byte("snap"))
	if err != nil {
		t.Fatal(err)
	}
	key := strings.TrimPrefix(uri, "s3://b/")
	if !strings.HasPrefix(key, "vault-snapshots/vault-") {
		t.Fatalf("empty prefix should default: %q", uri)
	}
	if string(store["b/"+key]) != "snap" {
		t.Fatalf("snapshot body %q", store["b/"+key])
	}
	sum := sha256.Sum256([]byte("snap"))
	if got := string(store["b/"+key+".sha256"]); got != hex.EncodeToString(sum[:])+"\n" {
		t.Fatalf("checksum %q", got)
	}
}

func TestPutSnapshotRejectsEmpty(t *testing.T) {
	if _, err := PutSnapshot(memObjects{}, "b", "vault-snapshots", nil); err == nil {
		t.Fatal("expected empty snapshot to fail")
	}
	if _, err := PutSnapshot(memObjects{}, "", "vault-snapshots", []byte("x")); err == nil {
		t.Fatal("expected missing bucket to fail")
	}
}
