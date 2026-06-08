package store

import (
	"context"
	"path/filepath"
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
		ID:         id,
		BackendID:  "container-" + id,
		OwnerID:    owner,
		Name:       "test",
		Map:        "de_inferno",
		Status:     model.StatusRunning,
		Public:     true,
		Host:       "1.2.3.4",
		GamePort:   27015,
		RCONPort:   27016,
		RCONPass:   "secret",
		MaxPlayers: 10,
		CreatedAt:  time.Unix(1700000000, 0),
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
