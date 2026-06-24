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
func (d *DB) RawDB() *sql.DB    { return d.sql }

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
if isPostgres {
if !d.IsPostgres() {
continue // skip .postgres.sql on sqlite — use plain .sql instead
}
// On Postgres: use .postgres.sql, skip the plain .sql counterpart
} else if d.IsPostgres() {
// On Postgres: skip plain .sql if a .postgres.sql version exists
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

func now() string {
// SQLite datetime() requires "YYYY-MM-DD HH:MM:SS" (no T, no subseconds).
// Postgres accepts this format too. Cross-driver safe.
return time.Now().UTC().Format("2006-01-02 15:04:05")
}

// rebind rewrites a query with ? placeholders to use $N for Postgres.
func (d *DB) rebind(q string) string {
if d.driver != "pgx" {
return q
}
n := 0
out := make([]byte, 0, len(q)+10)
for i := 0; i < len(q); i++ {
if q[i] == '?' {
n++
out = append(out, []byte(fmt.Sprintf("$%d", n))...)
} else {
out = append(out, q[i])
}
}
return string(out)
}

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

// query, exec, queryRow wrap sql methods with automatic placeholder rebinding.
func (d *DB) query(q string, args ...any) (*sql.Rows, error) {
return d.sql.Query(d.rebind(q), args...)
}
func (d *DB) exec(q string, args ...any) (sql.Result, error) {
return d.sql.Exec(d.rebind(q), args...)
}
func (d *DB) queryRow(q string, args ...any) *sql.Row {
return d.sql.QueryRow(d.rebind(q), args...)
}
func txQuery(d *DB, tx *sql.Tx, q string, args ...any) (*sql.Rows, error) {
return tx.Query(d.rebind(q), args...)
}
func txExec(d *DB, tx *sql.Tx, q string, args ...any) (sql.Result, error) {
return tx.Exec(d.rebind(q), args...)
}
func txQueryRow(d *DB, tx *sql.Tx, q string, args ...any) *sql.Row {
return tx.QueryRow(d.rebind(q), args...)
}

// insertOrReplace returns the correct upsert syntax for the driver.
// SQLite: INSERT OR REPLACE INTO t (cols) VALUES (...)
// Postgres: INSERT INTO t (cols) VALUES (...) ON CONFLICT (pk) DO UPDATE SET ...
func (d *DB) insertOrReplaceSnapshot() string {
if d.IsPostgres() {
return `INSERT INTO workflow_snapshots (workflow_id, seq, state_json, state_hash, created_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (workflow_id, seq) DO UPDATE SET
state_json=EXCLUDED.state_json, state_hash=EXCLUDED.state_hash, created_at=EXCLUDED.created_at`
}
return `INSERT OR REPLACE INTO workflow_snapshots (workflow_id, seq, state_json, state_hash, created_at)
VALUES (?, ?, ?, ?, ?)`
}

func (d *DB) insertOrIgnoreArtifact() string {
if d.IsPostgres() {
return `INSERT INTO artifacts (digest, workflow_id, artifact_json, content_ref, created_at)
VALUES (?, ?, ?, ?, ?) ON CONFLICT (digest) DO NOTHING`
}
return `INSERT OR IGNORE INTO artifacts (digest, workflow_id, artifact_json, content_ref, created_at)
VALUES (?, ?, ?, ?, ?)`
}

func (d *DB) insertOrIgnoreArtifactNoRef() string {
if d.IsPostgres() {
return `INSERT INTO artifacts (digest, workflow_id, artifact_json, created_at)
VALUES (?, ?, ?, ?) ON CONFLICT (digest) DO NOTHING`
}
return `INSERT OR IGNORE INTO artifacts (digest, workflow_id, artifact_json, created_at)
VALUES (?, ?, ?, ?)`
}
