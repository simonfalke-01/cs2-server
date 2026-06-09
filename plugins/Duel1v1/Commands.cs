using SwiftlyS2.Shared.Commands;
using SwiftlyS2.Shared.Players;

namespace Duel1v1;

public partial class Duel1v1
{
    /// <summary>
    /// Registers all chat/console commands. Weapon commands are generated from
    /// the Weapons catalog; config/info commands are explicit. Every command is
    /// available as both !cmd and /cmd (SwiftlyS2 handles both prefixes) and as
    /// a console command.
    /// </summary>
    private void RegisterCommands()
    {
        // Weapon commands (anytime, per-player, applied immediately).
        var seen = new HashSet<string>();
        foreach (var e in Weapons.All)
        {
            if (!seen.Add(e.Alias)) continue;
            var entry = e; // capture
            Core.Command.RegisterCommand(entry.Alias, ctx => WeaponPick(ctx, entry), false, "");
        }

        // Config (warmup only, either player).
        Core.Command.RegisterCommand("mr", CmdMr, false, "");
        Core.Command.RegisterCommand("nades", CmdNades, false, "");
        Core.Command.RegisterCommand("armor", CmdArmor, false, "");
        Core.Command.RegisterCommand("ready", CmdReady, false, "");
        Core.Command.RegisterCommand("unready", CmdUnready, false, "");

        // Map controls (warmup only).
        Core.Command.RegisterCommand("map", CmdMap, false, "");
        Core.Command.RegisterCommand("maps", CmdMaps, false, "");
        Core.Command.RegisterCommand("votemap", CmdVoteMap, false, "");

        // Match.
        Core.Command.RegisterCommand("rematch", CmdRematch, false, "");

        // Info.
        Core.Command.RegisterCommand("guns", CmdGuns, false, "");
        Core.Command.RegisterCommand("help", CmdHelp, false, "");
        Core.Command.RegisterCommand("score", CmdScore, false, "");
    }

    private static bool FromPlayer(ICommandContext ctx, out IPlayer player)
    {
        player = ctx.Sender!;
        return ctx.IsSentByPlayer && player != null;
    }

    // --- weapon pick -----------------------------------------------------

    private void WeaponPick(ICommandContext ctx, Weapons.Entry entry)
    {
        if (!FromPlayer(ctx, out var p)) return;

        if (entry.Slot == Weapons.Slot.Primary) _primary[p.SteamID] = entry.DesignerName;
        else _secondary[p.SteamID] = entry.DesignerName;

        Tell(p, $"{(entry.Slot == Weapons.Slot.Primary ? "Primary" : "Secondary")} set to \x04{entry.Display}\x01.");

        // Apply immediately (even mid-round) for the picker only.
        ReapplyLoadoutNow(p);
    }

    // --- config ----------------------------------------------------------

    private void CmdMr(ICommandContext ctx)
    {
        if (!FromPlayer(ctx, out var p)) return;
        if (_phase != Phase.Warmup) { Tell(p, "MR can only be changed during warmup."); return; }
        if (ctx.Args.Length < 1 || !int.TryParse(ctx.Args[0], out var n) || n < 1 || n > 30)
        {
            Tell(p, $"Usage: !mr <1-30>. Current: MR{_mr} (first to {WinTarget()}).");
            return;
        }
        _mr = n;
        Broadcast($"{p.Name} set the match to \x04MR{_mr}\x01 (first to {_mr + 1}).");
    }

    private void CmdNades(ICommandContext ctx)
    {
        if (!FromPlayer(ctx, out var p)) return;
        if (_phase != Phase.Warmup) { Tell(p, "Nades can only be toggled during warmup."); return; }
        _nadesEnabled = !_nadesEnabled;
        Broadcast($"{p.Name} turned grenades \x04{(_nadesEnabled ? "ON" : "OFF")}\x01.");
    }

    private void CmdArmor(ICommandContext ctx)
    {
        if (!FromPlayer(ctx, out var p)) return;
        if (_phase != Phase.Warmup) { Tell(p, "Armor can only be toggled during warmup."); return; }
        _armorEnabled = !_armorEnabled;
        Broadcast($"{p.Name} turned armor \x04{(_armorEnabled ? "ON" : "OFF")}\x01.");
    }

    private void CmdReady(ICommandContext ctx)
    {
        if (!FromPlayer(ctx, out var p)) return;
        if (_phase != Phase.Warmup) { Tell(p, "Match already running."); return; }
        if (!IsPlayer(p.SteamID)) { Tell(p, "Only the 2 active players can ready up (you're spectating)."); return; }
        if (CountPlayers() < 2) { Tell(p, "Need 2 players before readying up."); return; }

        if (!_ready.Add(p.SteamID)) { Tell(p, "You are already ready."); return; }
        Broadcast($"{p.Name} is \x04ready\x01 ({_ready.Count}/2).");

        if (_ready.Count >= 2 && _ready.Contains(_p1) && _ready.Contains(_p2))
            StartMatch();
    }

    private void CmdUnready(ICommandContext ctx)
    {
        if (!FromPlayer(ctx, out var p)) return;
        if (_phase != Phase.Warmup) return;
        if (_ready.Remove(p.SteamID))
            Broadcast($"{p.Name} is \x04not ready\x01 ({_ready.Count}/2).");
    }

    // --- map -------------------------------------------------------------

    private void CmdMap(ICommandContext ctx)
    {
        if (!FromPlayer(ctx, out var p)) return;
        if (_phase != Phase.Warmup) { Tell(p, "Map can only be changed during warmup."); return; }
        if (ctx.Args.Length < 1) { Tell(p, $"Usage: !map <name>.  Pool: {MapPool.Listing()}"); return; }

        var name = ctx.Args[0];
        Broadcast($"{p.Name} is changing the map to \x04{name}\x01…");
        var cmd = MapPool.ChangeCommandFor(name);
        Core.Scheduler.DelayBySeconds(2.0f, () => Server(cmd));
    }

    private void CmdMaps(ICommandContext ctx)
    {
        if (!FromPlayer(ctx, out var p)) return;
        Tell(p, $"Map pool: \x04{MapPool.Listing()}\x01.  Use !map <name> or !votemap <name>.");
    }

    // Simple one-shot map vote: either player proposing the same map twice, or
    // both players proposing it, switches. Kept lightweight: a proposal by one
    // player asks the other to confirm by repeating !votemap <name>.
    private string _pendingMapVote = "";
    private ulong _pendingMapVoter;

    private void CmdVoteMap(ICommandContext ctx)
    {
        if (!FromPlayer(ctx, out var p)) return;
        if (_phase != Phase.Warmup) { Tell(p, "Map can only be changed during warmup."); return; }
        if (ctx.Args.Length < 1) { Tell(p, $"Usage: !votemap <name>.  Pool: {MapPool.Listing()}"); return; }
        if (!IsPlayer(p.SteamID)) { Tell(p, "Only active players can vote maps."); return; }

        var name = ctx.Args[0].ToLowerInvariant();

        if (_pendingMapVote == name && _pendingMapVoter != p.SteamID)
        {
            Broadcast($"Both players voted \x04{name}\x01 — changing map…");
            var cmd = MapPool.ChangeCommandFor(name);
            _pendingMapVote = "";
            _pendingMapVoter = 0;
            Core.Scheduler.DelayBySeconds(2.0f, () => Server(cmd));
            return;
        }

        _pendingMapVote = name;
        _pendingMapVoter = p.SteamID;
        Broadcast($"{p.Name} wants to play \x04{name}\x01 — other player type \x04!votemap {name}\x01 to confirm.");
    }

    // --- match -----------------------------------------------------------

    private void CmdRematch(ICommandContext ctx)
    {
        if (!FromPlayer(ctx, out var p)) return;
        if (_phase != Phase.MatchEnd) { Tell(p, "Rematch is only available after a match ends."); return; }
        if (!IsPlayer(p.SteamID)) { Tell(p, "Only the 2 players can request a rematch."); return; }

        if (!_rematch.Add(p.SteamID)) { Tell(p, "You already voted to rematch."); return; }
        Broadcast($"{p.Name} wants a \x04rematch\x01 ({_rematch.Count}/2).");

        if (_rematch.Count >= 2 && _rematch.Contains(_p1) && _rematch.Contains(_p2))
        {
            Broadcast("\x04Rematch!\x01 Resetting…");
            StartMatch();
        }
    }

    // --- info ------------------------------------------------------------

    private void CmdGuns(ICommandContext ctx)
    {
        if (!FromPlayer(ctx, out var p)) return;
        var (primaries, secondaries) = Weapons.Listing();
        Tell(p, $"Primaries: \x04{primaries}\x01");
        Tell(p, $"Pistols: \x04{secondaries}\x01");
    }

    private void CmdHelp(ICommandContext ctx)
    {
        if (!FromPlayer(ctx, out var p)) return;
        Tell(p, "Config (warmup): !mr <n>, !nades, !armor, !map <name>, !maps, !votemap <name>, !ready, !unready");
        Tell(p, "Weapons (anytime): !ak !m4 !awp !deagle … (\x04!guns\x01 for full list)");
        Tell(p, "Info: !score, !rematch, !help");
    }

    private void CmdScore(ICommandContext ctx)
    {
        if (!FromPlayer(ctx, out var p)) return;
        string phase = _phase switch
        {
            Phase.Warmup => "warmup",
            Phase.Live => _otNumber > 0 ? $"overtime {_otNumber}" : "live",
            _ => "match over",
        };
        Tell(p, $"[{phase}] \x04{NameOf(_p1)}\x01 {_score1} - {_score2} \x04{NameOf(_p2)}\x01  (MR{_mr}, first to {WinTarget()})");
    }
}
