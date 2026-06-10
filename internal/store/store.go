// Package store persists server-instance metadata in SQLite so the control
// plane can survive restarts and reconcile running containers.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no CGO)

	"github.com/brandonli/cs2-server/internal/model"
)

// ErrNotFound mirrors model.ErrNotFound for store-level lookups.
var ErrNotFound = model.ErrNotFound

// Store is a SQLite-backed instance repository.
type Store struct {
	db *sql.DB
}

// Open opens (and migrates) the SQLite database at path.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("store: create dir: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	// SQLite handles one writer at a time; keep the pool small.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS instances (
	id           TEXT PRIMARY KEY,
	backend_id   TEXT NOT NULL,
	owner_id     TEXT NOT NULL,
	name         TEXT NOT NULL,
	map          TEXT NOT NULL,
	mode         TEXT NOT NULL DEFAULT '',
	workshop_map TEXT NOT NULL DEFAULT '',
	status       TEXT NOT NULL,
	public       INTEGER NOT NULL DEFAULT 0,
	host         TEXT NOT NULL,
	game_port    INTEGER NOT NULL,
	rcon_port    INTEGER NOT NULL,
	rcon_pass    TEXT NOT NULL,
	max_players  INTEGER NOT NULL,
	created_at   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_instances_owner ON instances(owner_id);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	// Add columns introduced after the initial schema. SQLite has no
	// "ADD COLUMN IF NOT EXISTS", so tolerate the duplicate-column error on
	// databases that already have them.
	addCols := []struct{ name, ddl string }{
		{"mode", `ALTER TABLE instances ADD COLUMN mode TEXT NOT NULL DEFAULT ''`},
		{"workshop_map", `ALTER TABLE instances ADD COLUMN workshop_map TEXT NOT NULL DEFAULT ''`},
	}
	for _, c := range addCols {
		if _, err := s.db.ExecContext(ctx, c.ddl); err != nil &&
			!strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("store: migrate add %s: %w", c.name, err)
		}
	}
	return nil
}

// Put inserts or updates an instance.
func (s *Store) Put(ctx context.Context, in *model.Instance) error {
	const q = `
INSERT INTO instances
	(id, backend_id, owner_id, name, map, mode, workshop_map, status, public, host, game_port, rcon_port, rcon_pass, max_players, created_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
	backend_id=excluded.backend_id,
	owner_id=excluded.owner_id,
	name=excluded.name,
	map=excluded.map,
	mode=excluded.mode,
	workshop_map=excluded.workshop_map,
	status=excluded.status,
	public=excluded.public,
	host=excluded.host,
	game_port=excluded.game_port,
	rcon_port=excluded.rcon_port,
	rcon_pass=excluded.rcon_pass,
	max_players=excluded.max_players;
`
	_, err := s.db.ExecContext(ctx, q,
		in.ID, in.BackendID, in.OwnerID, in.Name, in.Map, in.Mode, in.WorkshopMap, string(in.Status),
		boolToInt(in.Public), in.Host, in.GamePort, in.RCONPort, in.RCONPass,
		in.MaxPlayers, in.CreatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("store: put: %w", err)
	}
	return nil
}

// SetStatus updates only the status of an instance.
func (s *Store) SetStatus(ctx context.Context, id string, status model.Status) error {
	res, err := s.db.ExecContext(ctx, `UPDATE instances SET status=? WHERE id=?`, string(status), id)
	if err != nil {
		return fmt.Errorf("store: set status: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Get returns the instance by id.
func (s *Store) Get(ctx context.Context, id string) (*model.Instance, error) {
	row := s.db.QueryRowContext(ctx, selectCols+` WHERE id=?`, id)
	in, err := scanInstance(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return in, err
}

// List returns all instances, or those owned by ownerID when non-empty.
func (s *Store) List(ctx context.Context, ownerID string) ([]*model.Instance, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if ownerID == "" {
		rows, err = s.db.QueryContext(ctx, selectCols+` ORDER BY created_at DESC`)
	} else {
		rows, err = s.db.QueryContext(ctx, selectCols+` WHERE owner_id=? ORDER BY created_at DESC`, ownerID)
	}
	if err != nil {
		return nil, fmt.Errorf("store: list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*model.Instance
	for rows.Next() {
		in, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

// Delete removes an instance record.
func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM instances WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("store: delete: %w", err)
	}
	return nil
}

// CountByOwner returns how many instances an owner currently has recorded.
func (s *Store) CountByOwner(ctx context.Context, ownerID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM instances WHERE owner_id=?`, ownerID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count: %w", err)
	}
	return n, nil
}

const selectCols = `SELECT id, backend_id, owner_id, name, map, mode, workshop_map, status, public, host, game_port, rcon_port, rcon_pass, max_players, created_at FROM instances`

// rowScanner is implemented by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanInstance(r rowScanner) (*model.Instance, error) {
	var (
		in      model.Instance
		public  int
		status  string
		created int64
	)
	if err := r.Scan(
		&in.ID, &in.BackendID, &in.OwnerID, &in.Name, &in.Map, &in.Mode, &in.WorkshopMap, &status,
		&public, &in.Host, &in.GamePort, &in.RCONPort, &in.RCONPass,
		&in.MaxPlayers, &created,
	); err != nil {
		return nil, err
	}
	in.Status = model.Status(status)
	in.Public = public != 0
	in.CreatedAt = time.Unix(created, 0)
	return &in, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
