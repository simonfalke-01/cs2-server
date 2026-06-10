package gamemode

import (
	"reflect"
	"testing"
)

func TestLookupKnown(t *testing.T) {
	p, ok := Lookup("1v1")
	if !ok {
		t.Fatal("expected 1v1 preset to exist")
	}
	if p.Name != "1v1" || p.MaxPlayers != 8 {
		t.Fatalf("unexpected 1v1 preset: %+v", p)
	}
	if !p.NoBots {
		t.Fatal("1v1 preset should be human-only (NoBots)")
	}
}

func TestLookupCaseInsensitiveAndTrim(t *testing.T) {
	for _, in := range []string{"1V1", " 1v1 ", "1v1"} {
		if _, ok := Lookup(in); !ok {
			t.Fatalf("expected lookup to succeed for %q", in)
		}
	}
	if _, ok := Lookup("Competitive"); !ok {
		t.Fatal("expected case-insensitive lookup for Competitive")
	}
}

func TestLookupUnknown(t *testing.T) {
	if _, ok := Lookup("surf"); ok {
		t.Fatal("did not expect unknown preset to resolve")
	}
	if _, ok := Lookup(""); ok {
		t.Fatal("did not expect empty name to resolve")
	}
	if IsValid("nope") {
		t.Fatal("IsValid should be false for unknown preset")
	}
}

func TestDefaultIsValid(t *testing.T) {
	if !IsValid(Default) {
		t.Fatalf("Default %q must be a valid preset", Default)
	}
}

func TestNamesSortedAndComplete(t *testing.T) {
	got := Names()
	want := []string{"1v1", "competitive", "deathmatch", "wingman"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
}

func TestAllMatchesRegistry(t *testing.T) {
	all := All()
	if len(all) != len(Names()) {
		t.Fatalf("All() length %d != Names() length %d", len(all), len(Names()))
	}
	for _, p := range all {
		if p.Name == "" {
			t.Fatal("preset with empty name")
		}
		got, ok := Lookup(p.Name)
		if !ok || got != p {
			t.Fatalf("All() preset %+v not consistent with Lookup", p)
		}
	}
}
