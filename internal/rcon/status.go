package rcon

import (
	"regexp"
	"strconv"
	"strings"
)

// StatusInfo is a parsed subset of the CS2 "status" command output.
type StatusInfo struct {
	Map         string
	PlayerCount int
	MaxPlayers  int
}

var (
	// Matches lines like: "players : 3 humans, 0 bots (10/0 max)" or
	// "players : 1 humans, 4 bots (10 max)" across CS2 builds.
	rePlayers = regexp.MustCompile(`(?i)players\s*:\s*(\d+)\s+humans?,\s*(\d+)\s+bots?\s*\((\d+)`)
	// Matches: "map     : de_inferno" (optionally with extra columns).
	reMap = regexp.MustCompile(`(?i)^\s*map\s*:\s*([^\s]+)`)
)

// ParseStatus extracts map and player counts from "status" output. It is
// best-effort: fields it cannot find are left zero/empty.
func ParseStatus(raw string) StatusInfo {
	var info StatusInfo
	for _, line := range strings.Split(raw, "\n") {
		if m := reMap.FindStringSubmatch(line); m != nil && info.Map == "" {
			info.Map = m[1]
		}
		if m := rePlayers.FindStringSubmatch(line); m != nil {
			humans, _ := strconv.Atoi(m[1])
			bots, _ := strconv.Atoi(m[2])
			maxp, _ := strconv.Atoi(m[3])
			info.PlayerCount = humans + bots
			info.MaxPlayers = maxp
		}
	}
	return info
}
