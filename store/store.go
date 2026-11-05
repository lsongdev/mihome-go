// Package store is the SQLite database in the data directory. It holds the API's
// cache; everything in it can be downloaded again.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS cache (
	key       TEXT PRIMARY KEY,
	value     BLOB NOT NULL,
	stored_at INTEGER NOT NULL
);`

// schemaVersion is stored in PRAGMA user_version. There are no migrations: a
// database with another version must be removed by hand.
const schemaVersion = 1

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	// The file names the account's homes and devices: owner only, which SQLite
	// also applies to its journal files.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	file.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// A single connection keeps PRAGMAs effective.
	db.SetMaxOpenConns(1)
	if err := initialize(db, path); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func initialize(db *sql.DB, path string) error {
	for _, pragma := range []string{"PRAGMA journal_mode = WAL", "PRAGMA busy_timeout = 5000"} {
		if _, err := db.Exec(pragma); err != nil {
			return fmt.Errorf("configure sqlite: %w", err)
		}
	}
	var version, tables int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'").Scan(&tables); err != nil {
		return err
	}
	if tables > 0 && version != schemaVersion {
		return fmt.Errorf("database %s has an unsupported schema (version %d); move it away to start fresh", path, version)
	}
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	_, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion))
	return err
}

func (s *Store) Close() error { return s.db.Close() }

// Get returns a cached value and when it was stored.
func (s *Store) Get(key string) ([]byte, time.Time, bool, error) {
	var value []byte
	var storedAt int64
	err := s.db.QueryRow("SELECT value, stored_at FROM cache WHERE key = ?", key).Scan(&value, &storedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, false, nil
	}
	if err != nil {
		return nil, time.Time{}, false, err
	}
	return value, time.UnixMilli(storedAt), true, nil
}

// Set stores a value, replacing any previous one.
func (s *Store) Set(key string, value []byte) error {
	_, err := s.db.Exec(`INSERT INTO cache (key, value, stored_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, stored_at = excluded.stored_at`,
		key, value, time.Now().UnixMilli())
	return err
}
