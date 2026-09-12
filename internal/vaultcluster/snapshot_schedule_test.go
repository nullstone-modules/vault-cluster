package vaultcluster

import "testing"

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
