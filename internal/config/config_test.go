package config

import (
	"strings"
	"testing"
)

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

func TestNonIntegerPortErrors(t *testing.T) {
	t.Setenv("CS2C_GAME_PORT_MIN", "not-a-number")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for non-integer CS2C_GAME_PORT_MIN")
	}
	if !strings.Contains(err.Error(), "CS2C_GAME_PORT_MIN") {
		t.Fatalf("error should name the offending var, got: %v", err)
	}
}

func TestPortMinGreaterThanMaxErrors(t *testing.T) {
	t.Setenv("CS2C_GAME_PORT_MIN", "30000")
	t.Setenv("CS2C_GAME_PORT_MAX", "20000")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when CS2C_GAME_PORT_MIN > CS2C_GAME_PORT_MAX")
	}
	if !strings.Contains(err.Error(), "CS2C_GAME_PORT_MIN") || !strings.Contains(err.Error(), "CS2C_GAME_PORT_MAX") {
		t.Fatalf("error should name both port vars, got: %v", err)
	}
}

func TestGetEnvBool(t *testing.T) {
	const key = "CS2C_TEST_BOOL"
	truthy := []string{"1", "true", "yes", "on"}
	for _, v := range truthy {
		t.Setenv(key, v)
		if !getEnvBool(key, false) {
			t.Fatalf("getEnvBool(%q) = false, want true", v)
		}
	}
	falsy := []string{"0", "false"}
	for _, v := range falsy {
		t.Setenv(key, v)
		if getEnvBool(key, true) {
			t.Fatalf("getEnvBool(%q) = true, want false", v)
		}
	}
	// Garbage falls back to the default.
	t.Setenv(key, "garbage")
	if !getEnvBool(key, true) {
		t.Fatal("getEnvBool(garbage) should fall back to default true")
	}
	if getEnvBool(key, false) {
		t.Fatal("getEnvBool(garbage) should fall back to default false")
	}
	// Empty value also falls back to the default.
	t.Setenv(key, "")
	if !getEnvBool(key, true) {
		t.Fatal("getEnvBool(empty) should fall back to default true")
	}
	if getEnvBool(key, false) {
		t.Fatal("getEnvBool(empty) should fall back to default false")
	}
}

func TestRequireBotNamesMissing(t *testing.T) {
	// Both missing.
	t.Setenv("DISCORD_TOKEN", "")
	t.Setenv("DISCORD_APP_ID", "")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	err = c.RequireBot()
	if err == nil {
		t.Fatal("expected RequireBot error when token and app-id missing")
	}
	if !strings.Contains(err.Error(), "DISCORD_TOKEN") || !strings.Contains(err.Error(), "DISCORD_APP_ID") {
		t.Fatalf("error should name both missing vars, got: %v", err)
	}

	// Only token missing.
	t.Setenv("DISCORD_TOKEN", "")
	t.Setenv("DISCORD_APP_ID", "app-123")
	c, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	err = c.RequireBot()
	if err == nil {
		t.Fatal("expected RequireBot error when token missing")
	}
	if !strings.Contains(err.Error(), "DISCORD_TOKEN") {
		t.Fatalf("error should name DISCORD_TOKEN, got: %v", err)
	}
	if strings.Contains(err.Error(), "DISCORD_APP_ID") {
		t.Fatalf("error should not name the present DISCORD_APP_ID, got: %v", err)
	}

	// Only app-id missing.
	t.Setenv("DISCORD_TOKEN", "tok-123")
	t.Setenv("DISCORD_APP_ID", "")
	c, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	err = c.RequireBot()
	if err == nil {
		t.Fatal("expected RequireBot error when app-id missing")
	}
	if !strings.Contains(err.Error(), "DISCORD_APP_ID") {
		t.Fatalf("error should name DISCORD_APP_ID, got: %v", err)
	}

	// Both present -> no error.
	t.Setenv("DISCORD_TOKEN", "tok-123")
	t.Setenv("DISCORD_APP_ID", "app-123")
	c, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := c.RequireBot(); err != nil {
		t.Fatalf("RequireBot should pass when both set, got: %v", err)
	}
}

func TestAPIToken(t *testing.T) {
	// Default is empty (auth disabled).
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.APIToken != "" {
		t.Fatalf("APIToken default = %q, want empty", c.APIToken)
	}

	// Set value is read verbatim.
	t.Setenv("CS2C_API_TOKEN", "s3cr3t-token")
	c, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.APIToken != "s3cr3t-token" {
		t.Fatalf("APIToken = %q, want s3cr3t-token", c.APIToken)
	}
}
