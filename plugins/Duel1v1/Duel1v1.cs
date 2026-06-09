using SwiftlyS2.Shared;
using SwiftlyS2.Shared.Commands;
using SwiftlyS2.Shared.GameEvents;
using SwiftlyS2.Shared.GameEventDefinitions;
using SwiftlyS2.Shared.Misc;
using SwiftlyS2.Shared.Players;
using SwiftlyS2.Shared.Plugins;

namespace Duel1v1;

/// <summary>
/// Duel1v1 — a strict two-player 1v1 duel game mode for CS2 on SwiftlyS2.
///
/// Gated on CS2_MODE=1v1 (inert on any other mode, so one image serves all
/// modes). Flow:
///   - Infinite warmup with free practice (infinite respawn) until BOTH of the
///     two players type !ready.
///   - Match runs MR rounds (default mr12 = first to 13, halftime side swap),
///     overtime (MR3, first to 4, repeats) on a tie.
///   - Per-player weapon picks via chat (full arsenal), applied immediately.
///     Buying is disabled (cfg gives $0); pressing B shows a hint.
///   - Disconnect aborts the match back to warmup and resets the score.
///   - Match end -> !rematch (both) restarts, otherwise back to warmup.
///
/// See PluginMetadata below; sub-logic lives in partial files:
///   Commands.cs (chat commands), Match.cs (round/score/loadout logic).
/// </summary>
[PluginMetadata(
    Id = "Duel1v1",
    Version = "1.0.0",
    Name = "Duel1v1",
    Author = "cs2-server",
    Description = "Two-player 1v1 duel mode (activates when CS2_MODE=1v1).")]
public partial class Duel1v1 : BasePlugin
{
    // --- lifecycle / activation ------------------------------------------

    private bool _active;

    public Duel1v1(ISwiftlyCore core) : base(core) { }

    public override void ConfigureSharedInterface(IInterfaceManager interfaceManager) { }

    public override void UseSharedInterface(IInterfaceManager interfaceManager) { }

    public override void Load(bool hotReload)
    {
        var mode = Environment.GetEnvironmentVariable("CS2_MODE") ?? "";
        _active = string.Equals(mode.Trim(), "1v1", StringComparison.OrdinalIgnoreCase);

        if (!_active)
        {
            Console.WriteLine($"[Duel1v1] Inactive (CS2_MODE='{mode}'); not 1v1, staying dormant.");
            return;
        }

        Console.WriteLine("[Duel1v1] active (1v1) — duel mode enabled.");

        RegisterCommands();

        Core.GameEvent.HookPre<EventPlayerConnectFull>(OnPlayerConnectFull);
        Core.GameEvent.HookPre<EventPlayerDisconnect>(OnPlayerDisconnect);
        Core.GameEvent.HookPre<EventPlayerSpawn>(OnPlayerSpawn);
        Core.GameEvent.HookPost<EventPlayerDeath>(OnPlayerDeath);
        Core.GameEvent.HookPre<EventRoundStart>(OnRoundStart);
        Core.GameEvent.HookPre<EventBuymenuOpen>(OnBuyMenuOpen);

        EnterWarmup();
    }

    public override void Unload() { }

    // --- helpers ---------------------------------------------------------

    /// <summary>All connected, valid, human players (no bots/HLTV).</summary>
    private List<IPlayer> Humans()
        => Core.PlayerManager.GetAllValidPlayers().Where(p => !p.IsFakeClient).ToList();

    private void Broadcast(string msg)
        => Core.PlayerManager.SendChat($" \x04[1v1]\x01 {msg}");

    private void Tell(IPlayer p, string msg)
        => p.SendChat($" \x04[1v1]\x01 {msg}");

    /// <summary>Run a server console command (cfg/convar/changelevel).</summary>
    private void Server(string cmd)
        => Core.Engine.ExecuteCommand(cmd);
}
