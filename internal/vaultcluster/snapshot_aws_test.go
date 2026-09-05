package vaultcluster

import (
	"context"
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

func TestSnapshotObjectKey(t *testing.T) {
	got := snapshotObjectKey("vault-snapshots", "20260101T000000Z")
	if got != "vault-snapshots/vault-20260101T000000Z.snap" {
		t.Fatalf("key %q", got)
	}
}

func TestSnapshotListS3(t *testing.T) {
	store := memObjects{
		"b/vault-snapshots/vault-1.snap":        []byte("a"),
		"b/vault-snapshots/vault-1.snap.sha256": []byte("x"),
		"b/other/vault-2.snap":                  []byte("c"),
	}
	got, err := SnapshotListS3(store, "b", "vault-snapshots")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "s3://b/vault-snapshots/vault-1.snap" {
		t.Fatalf("list %v", got)
	}
}

func TestParseBackupSchedule(t *testing.T) {
	off, err := ParseBackupSchedule("")
	if err != nil || off != nil {
		t.Fatalf("empty: %v %v", off, err)
	}
	on, err := ParseBackupSchedule("0 3 * * *")
	if err != nil || on == nil {
		t.Fatalf("cron: %v %v", on, err)
	}
	if _, err := ParseBackupSchedule("not-a-cron"); err == nil {
		t.Fatal("expected invalid cron to fail")
	}
}
