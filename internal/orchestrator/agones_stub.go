package orchestrator

// This file documents the migration seam to Kubernetes + Agones.
//
// ServerManager is intentionally backend-agnostic. To run on Kubernetes with
// Agones (https://agones.dev), add an `agones.go` implementing ServerManager:
//
//   - Create:  allocate a GameServer from a Fleet (GameServerAllocation), or
//              create a GameServer directly; map the allocated UDP port/host
//              from the GameServer status into an Instance.
//   - Stop:    delete the GameServer (or mark it Shutdown via the SDK).
//   - Status:  read player count via the Agones SDK / RCON sidecar.
//   - List/Get: back by a Kubernetes informer/lister over GameServers, or keep
//              using the same SQLite store for ownership metadata.
//
// The CS2 container image (docker/cs2) is reused unchanged; you add an Agones
// SDK sidecar (or the built-in SDK) and a health endpoint. cmd/orchestrator
// would select the backend from config (e.g. CS2C_BACKEND=docker|agones).
//
// Keeping all game-server lifecycle behind ServerManager means the API and the
// Discord bot require no changes when this backend is added.
