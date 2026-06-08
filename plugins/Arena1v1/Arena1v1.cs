using SwiftlyS2.Shared;
using SwiftlyS2.Shared.Commands;
using SwiftlyS2.Shared.GameEvents;
using SwiftlyS2.Shared.GameEventDefinitions;
using SwiftlyS2.Shared.Misc;
using SwiftlyS2.Shared.Players;
using SwiftlyS2.Shared.Plugins;

namespace Arena1v1;

/// <summary>
/// Arena1v1 — a winner-stays 1v1 arena game mode for CS2 built on SwiftlyS2.
///
/// The plugin is gated by the CS2_MODE environment variable (set by the
/// orchestrator from the requested game-mode preset): it only activates when
/// CS2_MODE=1v1, so the same game image can serve every mode. The supporting
/// round flow (no buying, short freeze time, friendly-fire off, …) is applied
/// by the bundled 1v1.cfg; this plugin owns the orchestration:
///
///   - tracks connected human players in a FIFO queue,
///   - pairs two players into the arena each round (T vs CT), benching the rest
///     in spectator,
///   - when a combatant dies the survivor becomes the "champion" and the dead
///     player is sent to the back of the queue,
///   - the champion stays and the next queued player rotates in for the next
///     round (classic winner-stays 1v1).
/// </summary>
[PluginMetadata(
    Id = "Arena1v1",
    Version = "0.1.0",
    Name = "Arena1v1",
    Author = "cs2-server",
    Description = "Winner-stays 1v1 arena game mode (activates when CS2_MODE=1v1).")]
public partial class Arena1v1 : BasePlugin
{
    // Whether this server is running the 1v1 mode. When false the plugin loads
    // but does nothing, so the same image can serve all game modes.
    private bool _active;

    // SteamID of the current champion (winner who stays), or 0 when none.
    private ulong _champion;

    // SteamIDs of the two players currently fighting in the arena.
    private readonly HashSet<ulong> _combatants = new();

    // FIFO of waiting players (SteamIDs) who will rotate into the arena.
    private readonly Queue<ulong> _queue = new();

    public Arena1v1(ISwiftlyCore core) : base(core) { }

    public override void ConfigureSharedInterface(IInterfaceManager interfaceManager) { }

    public override void UseSharedInterface(IInterfaceManager interfaceManager) { }

    public override void Load(bool hotReload)
    {
        var mode = Environment.GetEnvironmentVariable("CS2_MODE") ?? "";
        _active = string.Equals(mode.Trim(), "1v1", StringComparison.OrdinalIgnoreCase);

        if (!_active)
        {
            Console.WriteLine($"[Arena1v1] Inactive (CS2_MODE='{mode}'); not 1v1, staying dormant.");
            return;
        }

        Console.WriteLine("[Arena1v1] active (1v1) — winner-stays arena enabled.");

        Core.GameEvent.HookPre<EventPlayerConnectFull>(OnPlayerConnectFull);
        Core.GameEvent.HookPre<EventPlayerDisconnect>(OnPlayerDisconnect);
        Core.GameEvent.HookPre<EventPlayerDeath>(OnPlayerDeath);
        Core.GameEvent.HookPre<EventRoundStart>(OnRoundStart);
    }

    public override void Unload() { }

    // --- Commands ---------------------------------------------------------

    /// <summary>!arena / sw_arena — show the current matchup and queue.</summary>
    [Command("arena")]
    public void ArenaCommand(ICommandContext context)
    {
        if (!_active)
        {
            context.Reply("Arena1v1 is not active on this server.");
            return;
        }
        context.Reply($"Arena1v1: {_combatants.Count} fighting, {_queue.Count} in queue.");
    }

    /// <summary>!queue / sw_queue — show how many players are waiting.</summary>
    [Command("queue")]
    public void QueueCommand(ICommandContext context)
    {
        if (!_active)
        {
            context.Reply("Arena1v1 is not active on this server.");
            return;
        }
        context.Reply($"Players in queue: {_queue.Count}.");
    }

    // --- Game events ------------------------------------------------------

    private HookResult OnPlayerConnectFull(EventPlayerConnectFull @event)
    {
        return HookResult.Continue;
    }

    private HookResult OnPlayerDisconnect(EventPlayerDisconnect @event)
    {
        // Rebuild membership lazily on the next round; just drop stale refs of
        // anyone no longer connected so the champion/queue don't go stale.
        PruneDisconnected();
        return HookResult.Continue;
    }

    private HookResult OnPlayerDeath(EventPlayerDeath @event)
    {
        if (!_active)
        {
            return HookResult.Continue;
        }

        // Decide the round the moment a combatant dies: the survivor among the
        // two combatants becomes the champion and stays next round; the dead
        // combatant goes to the back of the queue.
        var survivors = Humans().Where(p => _combatants.Contains(p.SteamID) && p.IsAlive).ToList();
        if (survivors.Count == 1)
        {
            var winner = survivors[0];
            _champion = winner.SteamID;
            foreach (var sid in _combatants.Where(s => s != _champion).ToList())
            {
                _queue.Enqueue(sid);
            }
            Core.PlayerManager.SendChat($"[Arena1v1] {winner.Name} wins the duel and stays!");
        }
        return HookResult.Continue;
    }

    private HookResult OnRoundStart(EventRoundStart @event)
    {
        if (!_active)
        {
            return HookResult.Continue;
        }
        SetupArena();
        return HookResult.Continue;
    }

    // --- Arena orchestration ---------------------------------------------

    /// <summary>
    /// Assigns two players to the arena (T vs CT) for this round and benches the
    /// rest in spectator. The champion (previous round's winner) keeps their
    /// place; the second slot is filled from the queue.
    /// </summary>
    private void SetupArena()
    {
        var humans = Humans();
        if (humans.Count < 2)
        {
            // Not enough players: park everyone in spectator and wait.
            foreach (var p in humans)
            {
                Bench(p);
            }
            _combatants.Clear();
            Core.PlayerManager.SendChat("[Arena1v1] Waiting for at least 2 players…");
            return;
        }

        SyncQueue(humans);

        // Pick the two combatants: the champion (if still present) plus the next
        // player from the queue; otherwise the two longest-waiting players.
        var bySteam = humans.ToDictionary(p => p.SteamID);
        var fighters = new List<IPlayer>();

        if (_champion != 0 && bySteam.TryGetValue(_champion, out var champ))
        {
            fighters.Add(champ);
        }
        while (fighters.Count < 2 && _queue.Count > 0)
        {
            var sid = _queue.Dequeue();
            if (bySteam.TryGetValue(sid, out var p) && !fighters.Contains(p))
            {
                fighters.Add(p);
            }
        }
        // Backfill from any remaining humans if the queue ran dry.
        if (fighters.Count < 2)
        {
            foreach (var p in humans)
            {
                if (fighters.Count >= 2) break;
                if (!fighters.Contains(p)) fighters.Add(p);
            }
        }

        _combatants.Clear();
        for (int i = 0; i < fighters.Count; i++)
        {
            var team = i == 0 ? Team.T : Team.CT;
            fighters[i].SwitchTeam(team);
            _combatants.Add(fighters[i].SteamID);
        }

        // Everyone else waits in spectator and (re)joins the back of the queue.
        foreach (var p in humans)
        {
            if (_combatants.Contains(p.SteamID)) continue;
            Bench(p);
            if (!_queue.Contains(p.SteamID))
            {
                _queue.Enqueue(p.SteamID);
            }
        }

        _champion = 0; // consumed; the next death sets the new champion.

        if (fighters.Count == 2)
        {
            Core.PlayerManager.SendChat($"[Arena1v1] {fighters[0].Name} vs {fighters[1].Name} — fight!");
        }
    }

    /// <summary>Moves a player to spectator.</summary>
    private void Bench(IPlayer player)
    {
        player.SwitchTeam(Team.Spectator);
    }

    /// <summary>All connected, valid, human players.</summary>
    private List<IPlayer> Humans()
    {
        return Core.PlayerManager.GetAllValidPlayers()
            .Where(p => !p.IsFakeClient)
            .ToList();
    }

    /// <summary>Ensures every connected human that is not fighting is queued.</summary>
    private void SyncQueue(List<IPlayer> humans)
    {
        var present = humans.Select(p => p.SteamID).ToHashSet();
        // Drop queued SteamIDs that are no longer connected.
        var kept = new Queue<ulong>();
        foreach (var sid in _queue)
        {
            if (present.Contains(sid)) kept.Enqueue(sid);
        }
        _queue.Clear();
        foreach (var sid in kept) _queue.Enqueue(sid);

        // Add any newcomer (connected, not fighting, not already queued).
        foreach (var p in humans)
        {
            if (_combatants.Contains(p.SteamID)) continue;
            if (!_queue.Contains(p.SteamID)) _queue.Enqueue(p.SteamID);
        }
    }

    /// <summary>Drops the champion/combatants/queue entries that disconnected.</summary>
    private void PruneDisconnected()
    {
        var present = Humans().Select(p => p.SteamID).ToHashSet();
        if (_champion != 0 && !present.Contains(_champion)) _champion = 0;
        _combatants.RemoveWhere(sid => !present.Contains(sid));

        var kept = new Queue<ulong>();
        foreach (var sid in _queue)
        {
            if (present.Contains(sid)) kept.Enqueue(sid);
        }
        _queue.Clear();
        foreach (var sid in kept) _queue.Enqueue(sid);
    }
}
