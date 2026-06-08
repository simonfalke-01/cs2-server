package ports

import (
	"sync"
	"testing"
)

func TestAcquireReleaseCycle(t *testing.T) {
	a := New(27015, 27017)

	got := map[int]bool{}
	for i := 0; i < 3; i++ {
		p, err := a.Acquire()
		if err != nil {
			t.Fatalf("acquire %d: %v", i, err)
		}
		if p < 27015 || p > 27017 {
			t.Fatalf("port %d out of range", p)
		}
		if got[p] {
			t.Fatalf("duplicate port %d", p)
		}
		got[p] = true
	}

	if _, err := a.Acquire(); err != ErrExhausted {
		t.Fatalf("expected ErrExhausted, got %v", err)
	}

	// Release one and re-acquire it.
	a.Release(27016)
	p, err := a.Acquire()
	if err != nil {
		t.Fatalf("re-acquire: %v", err)
	}
	if p != 27016 {
		t.Fatalf("expected released port 27016, got %d", p)
	}
}

func TestReservedPortsSkipped(t *testing.T) {
	a := New(27015, 27016, 27015)
	p, err := a.Acquire()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if p != 27016 {
		t.Fatalf("expected 27016 (27015 reserved), got %d", p)
	}
}

func TestConcurrentAcquireNoDuplicates(t *testing.T) {
	const n = 100
	a := New(20000, 20000+n-1)

	var (
		mu   sync.Mutex
		seen = map[int]bool{}
		wg   sync.WaitGroup
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := a.Acquire()
			if err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			mu.Lock()
			if seen[p] {
				t.Errorf("duplicate port %d", p)
			}
			seen[p] = true
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(seen) != n {
		t.Fatalf("expected %d unique ports, got %d", n, len(seen))
	}
}
