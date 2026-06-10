package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brandonli/cs2-server/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func sampleInstance(id, owner string) *model.Instance {
	return &model.Instance{
		ID:          id,
		BackendID:   "container-" + id,
		OwnerID:     owner,
		Name:        "test",
		Map:         "de_inferno",
		Mode:        "1v1",
		WorkshopMap: "3070253702",
		Status:      model.StatusRunning,
		Public:      true,
		Host:        "1.2.3.4",
		GamePort:    27015,
		RCONPort:    27016,
		RCONPass:    "secret",
		MaxPlayers:  10,
		CreatedAt:   time.Unix(1700000000, 0),
	}
}

func TestPutGetRoundtrip(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	in := sampleInstance("abc123", "owner1")
	if err := st.Put(ctx, in); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, err := st.Get(ctx, "abc123")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.OwnerID != "owner1" || got.GamePort != 27015 || !got.Public {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if got.Mode != "1v1" {
		t.Fatalf("mode not persisted: %q", got.Mode)
	}
	if got.WorkshopMap != "3070253702" {
		t.Fatalf("workshop_map not persisted: %q", got.WorkshopMap)
	}
	if got.RCONPass != "secret" {
		t.Fatalf("rcon pass not persisted")
	}
	if !got.CreatedAt.Equal(time.Unix(1700000000, 0)) {
		t.Fatalf("created_at mismatch: %v", got.CreatedAt)
	}
}

func TestGetNotFound(t *testing.T) {
	st := newTestStore(t)
	if _, err := st.Get(context.Background(), "missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListAndCountByOwner(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	_ = st.Put(ctx, sampleInstance("a", "owner1"))
	_ = st.Put(ctx, sampleInstance("b", "owner1"))
	_ = st.Put(ctx, sampleInstance("c", "owner2"))

	all, err := st.List(ctx, "")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 instances, got %d", len(all))
	}

	o1, err := st.List(ctx, "owner1")
	if err != nil {
		t.Fatalf("list owner1: %v", err)
	}
	if len(o1) != 2 {
		t.Fatalf("expected 2 for owner1, got %d", len(o1))
	}

	n, err := st.CountByOwner(ctx, "owner1")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected count 2, got %d", n)
	}
}

func TestSetStatusAndDelete(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	_ = st.Put(ctx, sampleInstance("x", "owner1"))

	if err := st.SetStatus(ctx, "x", model.StatusStopped); err != nil {
		t.Fatalf("set status: %v", err)
	}
	got, _ := st.Get(ctx, "x")
	if got.Status != model.StatusStopped {
		t.Fatalf("status not updated: %v", got.Status)
	}

	if err := st.SetStatus(ctx, "nope", model.StatusStopped); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound updating missing, got %v", err)
	}

	if err := st.Delete(ctx, "x"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.Get(ctx, "x"); err != ErrNotFound {
		t.Fatalf("expected deleted, got %v", err)
	}
}

// TestMigrateIdempotentReopen ensures re-Opening (and thus re-migrating) an
// existing database succeeds and leaves previously stored rows intact. The
// ALTER TABLE ADD COLUMN migrations must tolerate already-present columns.
func TestMigrateIdempotentReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "reopen.db")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := st.Put(ctx, sampleInstance("keep", "owner1")); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Re-open the same path: migrate() runs again over an existing schema.
	st2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen (migrate not idempotent): %v", err)
	}
	t.Cleanup(func() { _ = st2.Close() })

	got, err := st2.Get(ctx, "keep")
	if err != nil {
		t.Fatalf("get after reopen: %v", err)
	}
	if got.OwnerID != "owner1" || got.Mode != "1v1" || got.WorkshopMap != "3070253702" {
		t.Fatalf("row not intact after reopen: %+v", got)
	}
}

// TestListOrderingAndUnknownOwner verifies List returns rows newest-first by
// created_at and that listing an unknown owner yields an empty slice.
func TestListOrderingAndUnknownOwner(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	older := sampleInstance("older", "owner1")
	older.CreatedAt = time.Unix(1700000000, 0)
	newer := sampleInstance("newer", "owner1")
	newer.CreatedAt = time.Unix(1700000500, 0)

	// Insert older first so insertion order differs from the expected ordering.
	if err := st.Put(ctx, older); err != nil {
		t.Fatalf("put older: %v", err)
	}
	if err := st.Put(ctx, newer); err != nil {
		t.Fatalf("put newer: %v", err)
	}

	all, err := st.List(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(all))
	}
	if all[0].ID != "newer" || all[1].ID != "older" {
		t.Fatalf("expected DESC-by-created_at order [newer, older], got [%s, %s]", all[0].ID, all[1].ID)
	}

	none, err := st.List(ctx, "does-not-exist")
	if err != nil {
		t.Fatalf("list unknown owner: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected 0 instances for unknown owner, got %d", len(none))
	}
}

// TestInstanceJSONOmitsRCONPass asserts the RCON password never serializes to
// clients: model.Instance tags RCONPass as json:"-", so neither the secret
// value nor any rcon_pass key may appear in the marshaled JSON.
func TestInstanceJSONOmitsRCONPass(t *testing.T) {
	in := sampleInstance("abc", "owner1")
	in.RCONPass = "super-secret-value"

	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(b)
	if strings.Contains(out, "super-secret-value") {
		t.Fatalf("RCON password leaked into JSON: %s", out)
	}
	if strings.Contains(out, "rcon_pass") || strings.Contains(out, "RCONPass") {
		t.Fatalf("rcon_pass key present in JSON: %s", out)
	}
}
