package config

import "testing"

func TestDefaultModeValidation(t *testing.T) {
	t.Setenv("CS2C_DEFAULT_MODE", "bogus")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for bogus CS2C_DEFAULT_MODE")
	}
	t.Setenv("CS2C_DEFAULT_MODE", "1v1")
	c, err := Load()
	if err != nil {
		t.Fatalf("unexpected error for valid mode: %v", err)
	}
	if c.DefaultMode != "1v1" {
		t.Fatalf("DefaultMode = %q, want 1v1", c.DefaultMode)
	}
}
