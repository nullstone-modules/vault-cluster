package s3

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

type memObjects map[string][]byte

func (m memObjects) Put(_ context.Context, bucket, key string, body []byte) error {
	m[bucket+"/"+key] = append([]byte(nil), body...)
	return nil
}

func (m memObjects) PutIfAbsent(_ context.Context, bucket, key string, body []byte) (bool, error) {
	id := bucket + "/" + key
	if _, exists := m[id]; exists {
		return false, nil
	}
	m[id] = append([]byte(nil), body...)
	return true, nil
}

func (m memObjects) Get(_ context.Context, bucket, key string) ([]byte, error) {
	b, ok := m[bucket+"/"+key]
	if !ok {
		return nil, errors.New("missing")
	}
	return append([]byte(nil), b...), nil
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
		"b/vault-snapshots/vault-20260101T000000Z.snap": []byte("old"),
		"b/vault-snapshots/vault-20260102T000000Z.snap": []byte("new"),
		"b/vault-snapshots/vault-1.snap.sha256":         []byte("x"),
		"b/other/vault-2.snap":                          []byte("c"),
	}
	got, err := ListSnapshots(store, "b", "vault-snapshots")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "s3://b/vault-snapshots/vault-20260102T000000Z.snap" {
		t.Fatalf("list newest first: %v", got)
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

func TestClaimInitFirstWins(t *testing.T) {
	store := memObjects{}
	won, err := ClaimInit(store, "b", "vault-snapshots", "n1")
	if err != nil || !won {
		t.Fatalf("first claim: won=%v err=%v", won, err)
	}
	won, err = ClaimInit(store, "b", "vault-snapshots", "n2")
	if err != nil || won {
		t.Fatalf("second node should lose: won=%v err=%v", won, err)
	}
	won, err = ClaimInit(store, "b", "vault-snapshots", "n1")
	if err != nil || !won {
		t.Fatalf("claimer should resume: won=%v err=%v", won, err)
	}
}

func TestClaimInitStealsEmpty(t *testing.T) {
	store := memObjects{"b/vault-snapshots/.init-claim": []byte("")}
	won, err := ClaimInit(store, "b", "vault-snapshots", "n1")
	if err != nil || !won {
		t.Fatalf("empty claim should be stealable: won=%v err=%v", won, err)
	}
}

func TestClaimInitRequiresBucket(t *testing.T) {
	if _, err := ClaimInit(memObjects{}, "", "vault-snapshots", "n1"); err == nil {
		t.Fatal("expected missing bucket to fail")
	}
}

func TestParseObjectURI(t *testing.T) {
	bucket, key, err := ParseObjectURI("s3://b/vault-snapshots/vault-1.snap")
	if err != nil || bucket != "b" || key != "vault-snapshots/vault-1.snap" {
		t.Fatalf("got %q %q %v", bucket, key, err)
	}
	if _, _, err := ParseObjectURI("s3://b/vault-snapshots/vault-1.snap.sha256"); err == nil {
		t.Fatal("checksum object is not a snapshot uri")
	}
	if _, _, err := ParseObjectURI("/tmp/vault.snap"); err == nil {
		t.Fatal("local path is not an s3 uri")
	}
}

func TestGetSnapshotVerifiesChecksum(t *testing.T) {
	sum := sha256.Sum256([]byte("snap"))
	store := memObjects{
		"b/vault-snapshots/vault-1.snap":        []byte("snap"),
		"b/vault-snapshots/vault-1.snap.sha256": []byte(hex.EncodeToString(sum[:]) + "\n"),
	}
	got, err := GetSnapshot(store, "s3://b/vault-snapshots/vault-1.snap")
	if err != nil || string(got) != "snap" {
		t.Fatalf("got %q err=%v", got, err)
	}
}

func TestGetSnapshotRejectsBadChecksum(t *testing.T) {
	store := memObjects{
		"b/vault-snapshots/vault-1.snap":        []byte("snap"),
		"b/vault-snapshots/vault-1.snap.sha256": []byte("deadbeef\n"),
	}
	if _, err := GetSnapshot(store, "s3://b/vault-snapshots/vault-1.snap"); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}

func TestGetSnapshotRequiresChecksum(t *testing.T) {
	store := memObjects{"b/vault-snapshots/vault-1.snap": []byte("snap")}
	if _, err := GetSnapshot(store, "s3://b/vault-snapshots/vault-1.snap"); err == nil {
		t.Fatal("expected missing checksum to fail")
	}
}
