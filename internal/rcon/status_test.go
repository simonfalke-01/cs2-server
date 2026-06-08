package rcon

import "testing"

func TestParseStatus(t *testing.T) {
	raw := `hostname: cs2-server dev box
version : 1.40.5.7/14057 1234 secure
map     : de_inferno
players : 3 humans, 2 bots (10/0 max) (not hibernating)
`
	got := ParseStatus(raw)
	if got.Map != "de_inferno" {
		t.Errorf("map: got %q want de_inferno", got.Map)
	}
	if got.PlayerCount != 5 {
		t.Errorf("players: got %d want 5", got.PlayerCount)
	}
	if got.HumanCount != 3 {
		t.Errorf("humans: got %d want 3", got.HumanCount)
	}
	if got.MaxPlayers != 10 {
		t.Errorf("max: got %d want 10", got.MaxPlayers)
	}
}

func TestParseStatusAltFormat(t *testing.T) {
	raw := `map     : de_dust2
players : 0 humans, 4 bots (12 max)`
	got := ParseStatus(raw)
	if got.Map != "de_dust2" {
		t.Errorf("map: got %q want de_dust2", got.Map)
	}
	if got.PlayerCount != 4 {
		t.Errorf("players: got %d want 4", got.PlayerCount)
	}
	if got.HumanCount != 0 {
		t.Errorf("humans: got %d want 0", got.HumanCount)
	}
	if got.MaxPlayers != 12 {
		t.Errorf("max: got %d want 12", got.MaxPlayers)
	}
}

func TestParseStatusEmpty(t *testing.T) {
	got := ParseStatus("garbage output with no fields")
	if got.Map != "" || got.PlayerCount != 0 || got.MaxPlayers != 0 {
		t.Errorf("expected zero value, got %+v", got)
	}
}
