package store

import (
"database/sql"
"fmt"
"os"
"time"

_ "github.com/mattn/go-sqlite3"
)

type DB struct {
sql *sql.DB
}

func Open(dsn string) (*DB, error) {
db, err := sql.Open("sqlite3", dsn+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000")
if err != nil {
return nil, fmt.Errorf("open db: %w", err)
}
db.SetMaxOpenConns(1)
db.SetMaxIdleConns(1)
if err := db.Ping(); err != nil {
return nil, fmt.Errorf("ping db: %w", err)
}
d := &DB{sql: db}
if err := d.migrate(); err != nil {
return nil, fmt.Errorf("migrate: %w", err)
}
return d, nil
}

func (d *DB) Close() error { return d.sql.Close() }

// MigrationDir is set by main before calling Open.
var MigrationDir = "migrations"

func (d *DB) migrate() error {
entries, err := os.ReadDir(MigrationDir)
if err != nil {
return fmt.Errorf("read migrations dir %s: %w", MigrationDir, err)
}
for _, e := range entries {
if e.IsDir() {
continue
}
sql, err := os.ReadFile(MigrationDir + "/" + e.Name())
if err != nil {
return err
}
if _, err := d.sql.Exec(string(sql)); err != nil {
return fmt.Errorf("migration %s: %w", e.Name(), err)
}
}
return nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func (d *DB) Tx(fn func(*sql.Tx) error) error {
tx, err := d.sql.Begin()
if err != nil {
return err
}
if err := fn(tx); err != nil {
_ = tx.Rollback()
return err
}
return tx.Commit()
}
