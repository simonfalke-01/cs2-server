using SwiftlyS2.Shared.GameEventDefinitions;
using SwiftlyS2.Shared.Misc;
using SwiftlyS2.Shared.Players;

namespace Duel1v1;

public partial class Duel1v1
{
    private enum Phase { Warmup, Live, MatchEnd }

    // --- configuration (changeable in warmup) ----------------------------
    private int _mr = 12;            // MRn: first to n+1, halftime at n
    private bool _nadesEnabled;       // server-wide nade opt-in
    private bool _armorEnabled = true; // server-wide armor toggle (default on)

    private const int OvertimeMr = 3; // OT: first to 4, swap at 3

    // --- runtime state ---------------------------------------------------
    private Phase _phase = Phase.Warmup;

    // The two players, by SteamID. p1 starts CT, p2 starts T.
    private ulong _p1, _p2;
    private readonly HashSet<ulong> _ready = new();

    // Scores keyed by SteamID.
    private int _score1, _score2;
    private int _roundsPlayed;
    private bool _sidesSwapped;       // halftime swap applied?
    private int _otNumber;            // 0 = regulation, 1+ = overtime period

    // Per-player loadout picks (SteamID -> designer name).
    private readonly Dictionary<ulong, string> _primary = new();
    private readonly Dictionary<ulong, string> _secondary = new();

    // Rematch votes after a match ends.
    private readonly HashSet<ulong> _rematch = new();

    // --- phase transitions -----------------------------------------------

    private void EnterWarmup()
    {
        _phase = Phase.Warmup;
        _ready.Clear();
        _rematch.Clear();
        _score1 = _score2 = 0;
        _roundsPlayed = 0;
        _sidesSwapped = false;
        _otNumber = 0;

        AssignPlayers();

        // Warmup: infinite, free practice (handled via respawn on death).
        Server("mp_warmup_start");
        Server("mp_warmup_pausetimer 1");
        Server("mp_respawn_immunitytime 0");

        if (CountPlayers() < 2)
            Broadcast("Waiting for 2 players… (spectators welcome)");
        else
            Broadcast($"Warmup — both players type \x04!ready\x01 to start. Mode: MR{_mr}, nades {(_nadesEnabled ? "on" : "off")}, armor {(_armorEnabled ? "on" : "off")}.");
    }

    private void StartMatch()
    {
        _phase = Phase.Live;
        _score1 = _score2 = 0;
        _roundsPlayed = 0;
        _sidesSwapped = false;
        _otNumber = 0;
        _ready.Clear();

        Server("mp_warmup_end");
        Server("mp_restartgame 1");

        Broadcast($"\x04Match LIVE!\x01 MR{_mr} — first to {WinTarget()}. Good luck!");
    }

    private void EndMatch(ulong winner)
    {
        _phase = Phase.MatchEnd;
        _rematch.Clear();
        var name = NameOf(winner);
        Broadcast($"\x04{name} wins the match {WinnerScore()}-{LoserScore()}!\x01 Type \x04!rematch\x01 to play again.");
    }

    // --- player assignment ----------------------------------------------

    /// <summary>
    /// Ensures _p1/_p2 hold the two active players (first two to connect, by
    /// connected time). Extra humans remain spectators. Returns true if the
    /// active pair changed.
    /// </summary>
    private bool AssignPlayers()
    {
        // Longest-connected first (largest ConnectedTime = earliest joiner), so
        // the first two to connect are the active players.
        var humans = Humans().OrderByDescending(p => p.ConnectedTime).ToList();

        ulong n1 = 0, n2 = 0;
        if (humans.Count > 0) n1 = humans[0].SteamID;
        if (humans.Count > 1) n2 = humans[1].SteamID;

        bool changed = n1 != _p1 || n2 != _p2;
        _p1 = n1;
        _p2 = n2;

        // Put active players on their sides; everyone else to spectator.
        foreach (var p in humans)
        {
            if (p.SteamID == _p1) EnsureTeam(p, SideOf(_p1));
            else if (p.SteamID == _p2) EnsureTeam(p, SideOf(_p2));
            else EnsureTeam(p, Team.Spectator);
        }
        return changed;
    }

    private int CountPlayers() => (_p1 != 0 ? 1 : 0) + (_p2 != 0 ? 1 : 0);

    private bool IsPlayer(ulong sid) => sid != 0 && (sid == _p1 || sid == _p2);

    /// <summary>Current side for a player, accounting for the halftime swap.</summary>
    private Team SideOf(ulong sid)
    {
        // p1 = CT, p2 = T before swap; reversed after.
        bool p1IsCt = !_sidesSwapped;
        if (sid == _p1) return p1IsCt ? Team.CT : Team.T;
        if (sid == _p2) return p1IsCt ? Team.T : Team.CT;
        return Team.Spectator;
    }

    private void EnsureTeam(IPlayer p, Team team)
    {
        if (p.Controller.Team != team)
            p.SwitchTeam(team);
    }

    // --- scoring ---------------------------------------------------------

    // Regulation: first to mr+1 (e.g. MR12 -> 13). Each overtime period adds
    // OvertimeMr wins to the target: from a tie at the start of OT-n both sit at
    // mr + (n-1)*OvertimeMr, and the winner must reach that + (OvertimeMr+1),
    // which simplifies to mr + 1 + n*OvertimeMr (MR12 OT1 -> 16, OT2 -> 19).
    private int WinTarget()
        => _mr + 1 + _otNumber * OvertimeMr;

    private int WinnerScore() => Math.Max(_score1, _score2);
    private int LoserScore() => Math.Min(_score1, _score2);

    private void AddPoint(ulong sid)
    {
        if (sid == _p1) _score1++;
        else if (sid == _p2) _score2++;
    }

    // --- event handlers --------------------------------------------------

    private HookResult OnPlayerConnectFull(EventPlayerConnectFull ev)
    {
        if (!_active) return HookResult.Continue;
        var changed = AssignPlayers();
        if (_phase == Phase.Warmup && changed)
            EnterWarmup();
        return HookResult.Continue;
    }

    private HookResult OnPlayerDisconnect(EventPlayerDisconnect ev)
    {
        if (!_active) return HookResult.Continue;
        var sid = ev.UserIdPlayer?.SteamID ?? 0;
        var wasPlayer = IsPlayer(sid);

        // Drop from ready/rematch sets.
        _ready.Remove(sid);
        _rematch.Remove(sid);

        if ((_phase == Phase.Live || _phase == Phase.MatchEnd) && wasPlayer)
        {
            Broadcast($"{NameOf(sid)} left — match aborted, returning to warmup.");
            // Defer the reshuffle a tick so the player is fully gone.
            Core.Scheduler.NextTick(() => EnterWarmup());
            return HookResult.Continue;
        }

        Core.Scheduler.NextTick(() =>
        {
            if (AssignPlayers() && _phase == Phase.Warmup) EnterWarmup();
        });
        return HookResult.Continue;
    }

    private HookResult OnPlayerSpawn(EventPlayerSpawn ev)
    {
        if (!_active) return HookResult.Continue;
        var p = ev.UserIdPlayer;
        if (p == null || p.IsFakeClient) return HookResult.Continue;

        // Apply loadout shortly after spawn so the pawn/itemservices exist.
        Core.Scheduler.NextTick(() => ApplyLoadout(p));
        return HookResult.Continue;
    }

    private HookResult OnPlayerDeath(EventPlayerDeath ev)
    {
        if (!_active) return HookResult.Continue;

        var victim = ev.UserIdPlayer;
        if (victim == null) return HookResult.Continue;

        if (_phase == Phase.Warmup)
        {
            // Free practice: respawn the dead player quickly.
            var v = victim;
            Core.Scheduler.DelayBySeconds(1.0f, () => { if (v.IsValid) v.Respawn(); });
            return HookResult.Continue;
        }

        if (_phase != Phase.Live) return HookResult.Continue;
        if (!IsPlayer(victim.SteamID)) return HookResult.Continue;

        // Round decided: the OTHER player scores (suicide also awards opponent).
        ulong winner = victim.SteamID == _p1 ? _p2 : _p1;
        AddPoint(winner);
        _roundsPlayed++;

        Broadcast($"{NameOf(winner)} wins the round.  \x04{_score1}\x01-\x04{_score2}\x01  ({NameOf(_p1)} vs {NameOf(_p2)})");

        // Check match/halftime conditions, then the engine starts the next round.
        Core.Scheduler.DelayBySeconds(2.0f, EvaluateAfterRound);
        return HookResult.Continue;
    }

    private HookResult OnRoundStart(EventRoundStart ev)
    {
        if (!_active) return HookResult.Continue;
        // Loadout is applied on spawn; nothing required here, but keep players
        // pinned to the right team in case CS shuffled them.
        if (_phase == Phase.Live) AssignPlayers();
        return HookResult.Continue;
    }

    private HookResult OnBuyMenuOpen(EventBuymenuOpen ev)
    {
        if (!_active) return HookResult.Continue;
        Broadcast("Buying is disabled — use weapon commands (\x04!guns\x01 to list).");
        return HookResult.Continue;
    }

    // --- round evaluation ------------------------------------------------

    private void EvaluateAfterRound()
    {
        // Win check.
        int target = WinTarget();
        if (_score1 >= target) { EndMatch(_p1); return; }
        if (_score2 >= target) { EndMatch(_p2); return; }

        // Halftime swap (regulation only, once, at mp_maxrounds/2 == _mr).
        if (_otNumber == 0 && !_sidesSwapped && _roundsPlayed >= _mr)
        {
            _sidesSwapped = true;
            Broadcast("\x04Halftime — switching sides.\x01");
            AssignPlayers();
        }

        // Overtime trigger: regulation exhausted and tied.
        int regulationTotal = 2 * _mr;
        if (_otNumber == 0 && _roundsPlayed >= regulationTotal && _score1 == _score2)
        {
            _otNumber = 1;
            _sidesSwapped = false;
            Broadcast($"\x04Overtime!\x01 First to {WinTarget()} (MR{OvertimeMr}).");
        }
        else if (_otNumber > 0)
        {
            // OT halftime swap at OvertimeMr rounds into this OT period.
            int otRoundsPlayed = _roundsPlayed - regulationTotal - (_otNumber - 1) * (2 * OvertimeMr);
            if (!_sidesSwapped && otRoundsPlayed >= OvertimeMr)
            {
                _sidesSwapped = true;
                Broadcast("\x04OT halftime — switching sides.\x01");
                AssignPlayers();
            }
            // Next OT if this one ended tied.
            int otTotal = 2 * OvertimeMr;
            if (otRoundsPlayed >= otTotal && _score1 == _score2)
            {
                _otNumber++;
                _sidesSwapped = false;
                Broadcast($"\x04Another overtime!\x01 First to {WinTarget()}.");
            }
        }
    }

    // --- loadout ---------------------------------------------------------

    private void ApplyLoadout(IPlayer p)
    {
        if (!p.IsValid || !p.IsAlive) return;
        var pawn = p.PlayerPawn;
        if (pawn == null) return;
        var items = pawn.ItemServices;
        if (items == null) return;

        items.RemoveItems();

        var sid = p.SteamID;
        var primary = _primary.TryGetValue(sid, out var pr) ? pr : Weapons.DefaultPrimary;
        var secondary = _secondary.TryGetValue(sid, out var sc) ? sc : Weapons.DefaultSecondary;

        items.GiveItem(primary);
        items.GiveItem(secondary);
        items.GiveItem("weapon_knife");

        if (_nadesEnabled)
            foreach (var n in Weapons.NadeKit) items.GiveItem(n);

        // Armor.
        if (_armorEnabled)
        {
            pawn.ArmorValue = 100;
            // Helmet via give to ensure the helmet flag is set.
            items.GiveItem("item_assaultsuit");
        }
        else
        {
            pawn.ArmorValue = 0;
        }
    }

    private void ReapplyLoadoutNow(IPlayer p)
    {
        // Immediate weapon swap mid-round: strip and re-give per current picks.
        if (p.IsValid && p.IsAlive) ApplyLoadout(p);
    }

    private string NameOf(ulong sid)
    {
        var p = Humans().FirstOrDefault(x => x.SteamID == sid);
        return p?.Name ?? "Player";
    }
}
