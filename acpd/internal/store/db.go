package store

import (
"database/sql"
"fmt"
"os"
"strings"
"time"

_ "github.com/jackc/pgx/v5/stdlib"
_ "github.com/mattn/go-sqlite3"
)

type DB struct {
sql    *sql.DB
driver string
}

// MigrationDir is set by main before calling Open.
var MigrationDir = "migrations"

// Open opens a database connection based on the DSN.
// DSN prefixes:
//   postgres:// or postgresql://  -> pgx (Postgres)
//   :memory: or *.db or file:     -> sqlite3
func Open(dsn string) (*DB, error) {
driver, normalizedDSN := resolveDriver(dsn)

db, err := sql.Open(driver, normalizedDSN)
if err != nil {
return nil, fmt.Errorf("open db (%s): %w", driver, err)
}

if driver == "sqlite3" {
db.SetMaxOpenConns(1) // SQLite: single writer
db.SetMaxIdleConns(1)
} else {
// Postgres: connection pool
db.SetMaxOpenConns(20)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(30 * time.Minute)
db.SetConnMaxIdleTime(5 * time.Minute)
}

if err := db.Ping(); err != nil {
return nil, fmt.Errorf("ping db (%s): %w", driver, err)
}

d := &DB{sql: db, driver: driver}
if err := d.migrate(); err != nil {
return nil, fmt.Errorf("migrate: %w", err)
}
return d, nil
}

func resolveDriver(dsn string) (string, string) {
if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
return "pgx", dsn
}
// SQLite — append pragmas if not already present
if !strings.Contains(dsn, "?") {
dsn += "?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000"
}
return "sqlite3", dsn
}

func (d *DB) IsPostgres() bool { return d.driver == "pgx" }
func (d *DB) IsSQLite() bool   { return d.driver == "sqlite3" }
func (d *DB) Close() error      { return d.sql.Close() }

func (d *DB) migrate() error {
entries, err := os.ReadDir(MigrationDir)
if err != nil {
return fmt.Errorf("read migrations dir %s: %w", MigrationDir, err)
}

// Build set of migration files to run.
// For each base name (e.g. 001_initial), prefer:
//   - <name>.postgres.sql if driver=pgx
//   - <name>.sql otherwise (skip *.postgres.sql files on sqlite)
type migration struct {
version string
file    string
}
seen := map[string]bool{}
var migrations []migration

for _, e := range entries {
if e.IsDir() {
continue
}
name := e.Name()
if !strings.HasSuffix(name, ".sql") {
continue
}
isPostgres := strings.HasSuffix(name, ".postgres.sql")
if isPostgres && !d.IsPostgres() {
continue // skip postgres-specific files on sqlite
}
if !isPostgres && d.IsPostgres() {
// Check if a postgres-specific version exists; if so skip this one
pgName := strings.TrimSuffix(name, ".sql") + ".postgres.sql"
if _, statErr := os.Stat(MigrationDir + "/" + pgName); statErr == nil {
continue
}
}
var version string
if isPostgres {
version = strings.TrimSuffix(name, ".postgres.sql")
} else {
version = strings.TrimSuffix(name, ".sql")
}
if seen[version] {
continue
}
seen[version] = true
migrations = append(migrations, migration{version: version, file: name})
}

for _, m := range migrations {
// Check if already applied — use ? for both drivers (pgx stdlib accepts ?)
var count int
row := d.sql.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version)
if scanErr := row.Scan(&count); scanErr != nil {
// schema_migrations may not exist yet (first migration)
count = 0
}
if count > 0 {
continue
}
sqlBytes, err := os.ReadFile(MigrationDir + "/" + m.file)
if err != nil {
return err
}
if _, err := d.sql.Exec(string(sqlBytes)); err != nil {
return fmt.Errorf("migration %s: %w", m.file, err)
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

// placeholder returns the correct placeholder for the driver.
// SQLite uses ?, Postgres uses $1, $2, ...
// For simplicity in v0.3.0 we use ? everywhere and rely on pgx's
// stdlib compatibility layer which accepts ? as well as $N.
func (d *DB) placeholder(n int) string {
if d.driver == "pgx" {
return fmt.Sprintf("$%d", n)
}
return "?"
}
