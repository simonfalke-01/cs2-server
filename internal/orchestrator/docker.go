package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	dnetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"

	"github.com/brandonli/cs2-server/internal/gamemode"
	"github.com/brandonli/cs2-server/internal/ports"
	"github.com/brandonli/cs2-server/internal/rcon"
	"github.com/brandonli/cs2-server/internal/store"
)

// Label keys applied to managed containers so we can find/reconcile them.
const (
	labelManaged  = "cs2-server.managed"
	labelInstance = "cs2-server.instance-id"
	labelOwner    = "cs2-server.owner-id"
)

// DockerConfig configures the Docker-backed ServerManager.
type DockerConfig struct {
	Image             string
	PublicIP          string // advertised connect IP
	GamePortMin       int
	GamePortMax       int
	DefaultGSLT       string
	DefaultMap        string
	DefaultMode       string
	DefaultMaxPlayers int

	// Shared game files (OverlayFS/fuse-overlayfs) mode: all instances share one
	// read-only seeded game copy (SharedVolume) plus a thin per-instance layer.
	SharedGameFiles bool

	// Network is the Docker network that game containers join so the
	// orchestrator can reach their RCON port by container name. When empty,
	// RCON is dialed via 127.0.0.1 (only correct under host networking).
	Network string

	// SharedVolume is the named Docker volume holding the seeded shared game
	// copy, mounted at /shared in shared-files mode.
	SharedVolume string
}

// DockerManager implements ServerManager on top of the local Docker engine.
type DockerManager struct {
	cli   *client.Client
	store *store.Store
	ports *ports.Allocator
	cfg   DockerConfig

	seedMu sync.Mutex // serializes the one-off shared-game-files seeding
}

var _ ServerManager = (*DockerManager)(nil)

// NewDockerManager builds a manager, reconciling port reservations from any
// instances already recorded in the store.
func NewDockerManager(ctx context.Context, cfg DockerConfig, st *store.Store) (*DockerManager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker: new client: %w", err)
	}

	alloc := ports.New(cfg.GamePortMin, cfg.GamePortMax)
	existing, err := st.List(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, in := range existing {
		alloc.Reserve(in.GamePort)
		alloc.Reserve(in.RCONPort)
	}

	return &DockerManager{cli: cli, store: st, ports: alloc, cfg: cfg}, nil
}

// Close releases the Docker client.
func (m *DockerManager) Close() error { return m.cli.Close() }

// Create provisions and starts a new CS2 server container.
func (m *DockerManager) Create(ctx context.Context, opts CreateOptions) (*Instance, error) {
	m.applyDefaults(&opts)

	gamePort, err := m.ports.Acquire()
	if err != nil {
		return nil, ErrNoPorts
	}
	rconPort, err := m.ports.Acquire()
	if err != nil {
		m.ports.Release(gamePort)
		return nil, ErrNoPorts
	}

	id := newID()
	rconPass := newSecret()

	gslt := opts.GSLT
	if opts.Public && gslt == "" {
		gslt = m.cfg.DefaultGSLT
	}

	inst := &Instance{
		ID:         id,
		OwnerID:    opts.OwnerID,
		Name:       opts.Name,
		Map:        opts.Map,
		Mode:       opts.Mode,
		Status:     StatusStarting,
		Public:     opts.Public,
		Host:       m.cfg.PublicIP,
		GamePort:   gamePort,
		RCONPort:   rconPort,
		RCONPass:   rconPass,
		MaxPlayers: opts.MaxPlayers,
		CreatedAt:  time.Now(),
	}

	// Persist before starting so a crash mid-create is recoverable/cleanable.
	if err := m.store.Put(ctx, inst); err != nil {
		m.releasePorts(inst)
		return nil, err
	}

	// Provisioning can be very slow (first-time game-files seeding downloads
	// ~40GB; even a warm start runs SteamCMD + loads CS2). Do it in the
	// background with a detached context so the API responds immediately and a
	// client disconnect never cancels provisioning. The instance is returned in
	// the "starting" state; clients poll status to learn when it's running.
	go m.provision(opts, inst, gslt)

	return inst, nil
}

// provision performs the slow container bring-up off the request path.
func (m *DockerManager) provision(opts CreateOptions, inst *Instance, gslt string) {
	ctx := context.Background()
	containerID, err := m.startContainer(ctx, inst, opts, gslt)
	if err != nil {
		m.log("provision failed", "id", inst.ID, "err", err)
		inst.Status = StatusError
		_ = m.store.Put(ctx, inst)
		return
	}
	inst.BackendID = containerID
	inst.Status = StatusRunning
	if err := m.store.Put(ctx, inst); err != nil {
		m.log("provision persist failed", "id", inst.ID, "err", err)
	}
}

// log is a tiny stderr logger so the orchestrator records background failures
// without pulling a logger dependency into the manager.
func (m *DockerManager) log(msg string, kv ...any) {
	fmt.Fprintf(os.Stderr, "orchestrator manager: %s %v\n", msg, kv)
}

func (m *DockerManager) startContainer(ctx context.Context, inst *Instance, opts CreateOptions, gslt string) (string, error) {
	if err := m.ensureImage(ctx); err != nil {
		return "", err
	}
	if m.cfg.SharedGameFiles {
		if err := m.ensureSeeded(ctx); err != nil {
			return "", err
		}
	}

	lan := "1"
	if inst.Public {
		lan = "0"
	}

	env := []string{
		"CS2_SERVERNAME=" + opts.Name,
		"CS2_PORT=" + strconv.Itoa(inst.GamePort),
		"CS2_RCON_PORT=" + strconv.Itoa(inst.RCONPort),
		"CS2_RCONPW=" + inst.RCONPass,
		"CS2_MAXPLAYERS=" + strconv.Itoa(opts.MaxPlayers),
		"CS2_STARTMAP=" + opts.Map,
		"CS2_GAMETYPE=" + strconv.Itoa(opts.GameType),
		"CS2_GAMEMODE=" + strconv.Itoa(opts.GameMode),
		"CS2_MODE=" + opts.Mode,
		"CS2_LAN=" + lan,
		"SRCDS_TOKEN=" + gslt,
	}
	if opts.Password != "" {
		env = append(env, "CS2_PW="+opts.Password)
	}
	if opts.BotQuota > 0 {
		env = append(env, "CS2_BOT_QUOTA="+strconv.Itoa(opts.BotQuota))
	}
	if m.cfg.SharedGameFiles {
		env = append(env,
			"CS2_SHARED_MODE=1",
			"CS2_SHARED_LOWER=/shared/cs2",
		)
	}

	gamePortStr := strconv.Itoa(inst.GamePort)
	rconPortStr := strconv.Itoa(inst.RCONPort)

	// CS2 listens on the same port for tcp+udp; RCON uses the simpleproxy TCP
	// port from the base image when CS2_RCON_PORT is set.
	udpGame := nat.Port(gamePortStr + "/udp")
	tcpGame := nat.Port(gamePortStr + "/tcp")
	tcpRCON := nat.Port(rconPortStr + "/tcp")

	exposed := nat.PortSet{udpGame: {}, tcpGame: {}, tcpRCON: {}}
	bindings := nat.PortMap{
		udpGame: []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: gamePortStr}},
		tcpGame: []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: gamePortStr}},
		tcpRCON: []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: rconPortStr}},
	}

	// Per-instance writable data lives in its own Docker-managed named volume,
	// so there are no host paths to coordinate and storage is portable.
	instVolume := "cs2-data-" + inst.ID
	if err := m.ensureVolume(ctx, instVolume); err != nil {
		return "", err
	}

	mounts := []string{}
	if m.cfg.SharedGameFiles {
		// Shared mode: mount the seeded read-only game copy (shared volume) as
		// the overlay lowerdir, plus the per-instance volume for the writable
		// upper/work layers.
		mounts = append(mounts,
			m.cfg.SharedVolume+":/shared:ro",
			instVolume+":/instance",
		)
	} else {
		// Per-instance full game copy (SteamCMD download reused across restarts).
		mounts = append(mounts, instVolume+":/home/steam/cs2-dedicated")
	}

	cfg := &container.Config{
		Image:        m.cfg.Image,
		Env:          env,
		ExposedPorts: exposed,
		Labels: map[string]string{
			labelManaged:  "true",
			labelInstance: inst.ID,
			labelOwner:    inst.OwnerID,
		},
	}
	if m.cfg.SharedGameFiles {
		// The image defaults to the unprivileged "steam" user, but shared mode
		// must mount the overlay as root first (the entrypoint then drops back
		// to steam to run the server).
		cfg.User = "0:0"
	}
	hostCfg := &container.HostConfig{
		PortBindings: bindings,
		Binds:        mounts,
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyUnlessStopped,
		},
	}
	if m.cfg.SharedGameFiles {
		// Mounting the overlay inside the container requires CAP_SYS_ADMIN, and
		// the default AppArmor profile blocks mount(2); disable it. The
		// entrypoint mounts the overlay as root, then drops to the steam user.
		//
		// On rootless/userns Docker the kernel refuses overlay mounts, so the
		// entrypoint falls back to fuse-overlayfs, which needs /dev/fuse.
		hostCfg.CapAdd = []string{"SYS_ADMIN"}
		hostCfg.SecurityOpt = []string{"apparmor=unconfined"}
		hostCfg.Devices = []container.DeviceMapping{{
			PathOnHost:        "/dev/fuse",
			PathInContainer:   "/dev/fuse",
			CgroupPermissions: "rwm",
		}}
	}

	name := "cs2-" + inst.ID

	// Attach to the shared control-plane network so the orchestrator can reach
	// this container's RCON port by name (works regardless of host networking).
	netCfg := &dnetwork.NetworkingConfig{}
	if m.cfg.Network != "" {
		netCfg.EndpointsConfig = map[string]*dnetwork.EndpointSettings{
			m.cfg.Network: {},
		}
	}

	created, err := m.cli.ContainerCreate(ctx, cfg, hostCfg, netCfg, nil, name)
	if err != nil {
		return "", fmt.Errorf("docker: create container: %w", err)
	}
	if err := m.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		// Best-effort cleanup of the created-but-unstarted container.
		_ = m.cli.ContainerRemove(ctx, created.ID, container.RemoveOptions{Force: true})
		return "", fmt.Errorf("docker: start container: %w", err)
	}
	return created.ID, nil
}

// ensureImage pulls the configured image if it is not present locally.
func (m *DockerManager) ensureImage(ctx context.Context) error {
	_, _, err := m.cli.ImageInspectWithRaw(ctx, m.cfg.Image)
	if err == nil {
		return nil
	}
	// Not present: attempt a pull. Locally-built images won't be pullable, so a
	// pull failure here is only fatal if the image is also absent (it is).
	rc, perr := m.cli.ImagePull(ctx, m.cfg.Image, image.PullOptions{})
	if perr != nil {
		return fmt.Errorf("docker: image %q not found locally and pull failed: %w", m.cfg.Image, perr)
	}
	defer rc.Close()
	// Drain the pull progress stream.
	buf := make([]byte, 4096)
	for {
		if _, rerr := rc.Read(buf); rerr != nil {
			break
		}
	}
	return nil
}

// ensureVolume creates a named Docker volume if it does not already exist.
func (m *DockerManager) ensureVolume(ctx context.Context, name string) error {
	_, err := m.cli.VolumeInspect(ctx, name)
	if err == nil {
		return nil
	}
	if _, cerr := m.cli.VolumeCreate(ctx, volume.CreateOptions{
		Name:   name,
		Labels: map[string]string{labelManaged: "true"},
	}); cerr != nil {
		return fmt.Errorf("docker: create volume %q: %w", name, cerr)
	}
	return nil
}

// ensureSeeded makes sure the shared read-only game copy exists in the shared
// volume. If not yet seeded it runs a one-off seeding container (the slow ~40GB
// download) and waits for it to finish. Concurrent Create calls are serialized.
func (m *DockerManager) ensureSeeded(ctx context.Context) error {
	if err := m.ensureVolume(ctx, m.cfg.SharedVolume); err != nil {
		return err
	}

	if seeded, _ := m.isSeeded(ctx); seeded {
		return nil
	}

	m.seedMu.Lock()
	defer m.seedMu.Unlock()

	// Re-check after acquiring the lock (another Create may have seeded it).
	if seeded, _ := m.isSeeded(ctx); seeded {
		return nil
	}

	const name = "cs2-seed"
	// Remove any stale seeder from a previous failed attempt.
	_ = m.cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})

	cfg := &container.Config{
		Image:      m.cfg.Image,
		Entrypoint: []string{"/opt/cs2-hooks/seed.sh", "/shared/cs2"},
		Labels:     map[string]string{labelManaged: "true", "cs2-server.role": "seed"},
	}
	hostCfg := &container.HostConfig{
		Binds: []string{m.cfg.SharedVolume + ":/shared"},
	}

	created, err := m.cli.ContainerCreate(ctx, cfg, hostCfg, &dnetwork.NetworkingConfig{}, nil, name)
	if err != nil {
		return fmt.Errorf("docker: create seeder: %w", err)
	}
	defer m.cli.ContainerRemove(context.Background(), created.ID, container.RemoveOptions{Force: true})

	if err := m.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("docker: start seeder: %w", err)
	}

	// Wait for the seeder to complete (this is the long download).
	statusCh, errCh := m.cli.ContainerWait(ctx, created.ID, container.WaitConditionNotRunning)
	select {
	case werr := <-errCh:
		if werr != nil {
			return fmt.Errorf("docker: wait seeder: %w", werr)
		}
	case st := <-statusCh:
		if st.StatusCode != 0 {
			return fmt.Errorf("docker: seeder exited with code %d", st.StatusCode)
		}
	}

	if seeded, _ := m.isSeeded(ctx); !seeded {
		return fmt.Errorf("docker: seeding finished but marker missing in volume %q", m.cfg.SharedVolume)
	}
	return nil
}

// isSeeded checks for the .cs2-seeded marker inside the shared volume by running
// a tiny throwaway container that tests for the file.
func (m *DockerManager) isSeeded(ctx context.Context) (bool, error) {
	cfg := &container.Config{
		Image:      m.cfg.Image,
		Entrypoint: []string{"sh", "-c", "test -f /shared/cs2/.cs2-seeded"},
		Labels:     map[string]string{labelManaged: "true", "cs2-server.role": "probe"},
	}
	hostCfg := &container.HostConfig{
		Binds:      []string{m.cfg.SharedVolume + ":/shared:ro"},
		AutoRemove: false,
	}
	created, err := m.cli.ContainerCreate(ctx, cfg, hostCfg, &dnetwork.NetworkingConfig{}, nil, "")
	if err != nil {
		return false, err
	}
	defer m.cli.ContainerRemove(context.Background(), created.ID, container.RemoveOptions{Force: true})

	if err := m.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return false, err
	}
	statusCh, errCh := m.cli.ContainerWait(ctx, created.ID, container.WaitConditionNotRunning)
	select {
	case werr := <-errCh:
		if werr != nil {
			return false, werr
		}
	case st := <-statusCh:
		return st.StatusCode == 0, nil
	}
	return false, nil
}

// Stop stops and removes the instance's container and record.
func (m *DockerManager) Stop(ctx context.Context, id string) error {
	inst, err := m.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if inst.BackendID != "" {
		timeout := 30
		_ = m.cli.ContainerStop(ctx, inst.BackendID, container.StopOptions{Timeout: &timeout})
		// Treat an already-removed container as success so the instance record
		// can always be cleaned up (e.g. after a manual docker rm).
		if rerr := m.cli.ContainerRemove(ctx, inst.BackendID, container.RemoveOptions{Force: true}); rerr != nil && !client.IsErrNotFound(rerr) {
			return fmt.Errorf("docker: remove container: %w", rerr)
		}
	}
	// Reclaim the per-instance named volume (overlay upper/work in shared mode,
	// or the full game copy otherwise). Best-effort; don't fail the stop.
	_ = m.cli.VolumeRemove(ctx, "cs2-data-"+inst.ID, true)
	m.releasePorts(inst)
	return m.store.Delete(ctx, id)
}

// Restart restarts the instance's container.
func (m *DockerManager) Restart(ctx context.Context, id string) error {
	inst, err := m.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if inst.BackendID == "" {
		return ErrNotFound
	}
	timeout := 30
	if err := m.cli.ContainerRestart(ctx, inst.BackendID, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("docker: restart: %w", err)
	}
	return m.store.SetStatus(ctx, id, StatusRunning)
}

// Get returns the recorded instance.
func (m *DockerManager) Get(ctx context.Context, id string) (*Instance, error) {
	return m.store.Get(ctx, id)
}

// List returns recorded instances, optionally filtered by owner.
func (m *DockerManager) List(ctx context.Context, ownerID string) ([]*Instance, error) {
	return m.store.List(ctx, ownerID)
}

// Status pulls live status from the container via RCON.
func (m *DockerManager) Status(ctx context.Context, id string) (*LiveStatus, error) {
	inst, err := m.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	ls := &LiveStatus{MaxPlayers: inst.MaxPlayers, Map: inst.Map}

	// Confirm the container is actually running.
	insp, err := m.cli.ContainerInspect(ctx, inst.BackendID)
	if err != nil || insp.State == nil || !insp.State.Running {
		ls.Online = false
		return ls, nil
	}

	// Reach RCON by container name on the shared network when configured,
	// otherwise fall back to localhost (host-network deployments).
	rconHost := "127.0.0.1"
	if m.cfg.Network != "" {
		rconHost = "cs2-" + inst.ID
	}
	addr := fmt.Sprintf("%s:%d", rconHost, inst.RCONPort)
	raw, err := rcon.Run(ctx, addr, inst.RCONPass, "status", 5*time.Second)
	if err != nil {
		// Container is up but RCON may not be ready yet (still loading).
		ls.Online = true
		return ls, nil
	}

	parsed := rcon.ParseStatus(raw)
	ls.Online = true
	ls.Raw = raw
	if parsed.Map != "" {
		ls.Map = parsed.Map
	}
	ls.PlayerCount = parsed.PlayerCount
	ls.HumanCount = parsed.HumanCount
	if parsed.MaxPlayers > 0 {
		ls.MaxPlayers = parsed.MaxPlayers
	}
	return ls, nil
}

// ListManagedContainers returns IDs of containers we manage (for reconciliation).
func (m *DockerManager) ListManagedContainers(ctx context.Context) ([]string, error) {
	f := filters.NewArgs()
	f.Add("label", labelManaged+"=true")
	list, err := m.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: f})
	if err != nil {
		return nil, fmt.Errorf("docker: list: %w", err)
	}
	ids := make([]string, 0, len(list))
	for _, c := range list {
		ids = append(ids, c.ID)
	}
	return ids, nil
}

func (m *DockerManager) applyDefaults(opts *CreateOptions) {
	if opts.Map == "" {
		opts.Map = m.cfg.DefaultMap
	}
	if opts.Name == "" {
		opts.Name = "cs2-server"
	}

	// Resolve the game-mode preset. An empty/unknown request mode falls back to
	// the control-plane default; the preset seeds GameType/GameMode/MaxPlayers
	// only where the request left them unset, so explicit values still win.
	if opts.Mode == "" {
		opts.Mode = m.cfg.DefaultMode
	}
	if preset, ok := gamemode.Lookup(opts.Mode); ok {
		opts.Mode = preset.Name // canonicalize casing/spacing
		if opts.GameType == 0 && opts.GameMode == 0 {
			opts.GameType = preset.GameType
			opts.GameMode = preset.GameMode
		}
		if opts.MaxPlayers <= 0 {
			opts.MaxPlayers = preset.MaxPlayers
		}
		if preset.NoBots {
			// Human-only mode: ignore any requested bot quota. Bots would also
			// keep the server perpetually "occupied" and defeat idle reaping.
			opts.BotQuota = 0
		}
	}

	if opts.MaxPlayers <= 0 {
		opts.MaxPlayers = m.cfg.DefaultMaxPlayers
	}
	if opts.GameMode == 0 && opts.GameType == 0 {
		// Default to competitive (type 0, mode 1).
		opts.GameMode = 1
	}
}

func (m *DockerManager) releasePorts(inst *Instance) {
	m.ports.Release(inst.GamePort)
	m.ports.Release(inst.RCONPort)
}

// newID returns a short, URL-safe instance id.
func newID() string {
	var b [5]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// newSecret returns a random RCON password.
func newSecret() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
