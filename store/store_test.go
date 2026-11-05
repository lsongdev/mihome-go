package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCacheSurvivesReopening(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := s.Get("missing"); ok || err != nil {
		t.Fatalf("missing: %v %v", ok, err)
	}
	before := time.Now().Add(-time.Second)
	for _, value := range []string{"one", "two"} {
		if err := s.Set("key", []byte(value)); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	value, storedAt, ok, err := s.Get("key")
	if err != nil || !ok || string(value) != "two" {
		t.Fatalf("%q %v %v", value, ok, err)
	}
	if storedAt.Before(before) || storedAt.After(time.Now().Add(time.Second)) {
		t.Fatalf("stored at %v", storedAt)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestOtherSchemaVersionIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "unsupported schema") {
		t.Fatalf("unexpected error: %v", err)
	}
}
