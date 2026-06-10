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
    private bool _roundDecided;        // a death already scored this round (1v1: only the FIRST death counts)
    private bool _sidesSwapped;       // halftime swap applied?
    private int _otNumber;            // 0 = regulation, 1+ = overtime period
    private bool _pendingSwapRespawn; // force a clean respawn at the next live round start after a side swap

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
        _roundDecided = false;
        _sidesSwapped = false;
        _pendingSwapRespawn = false;
        _otNumber = 0;

        EnforceModeConvars();
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
        _roundDecided = false;
        _sidesSwapped = false;
        _pendingSwapRespawn = false;
        _otNumber = 0;
        _ready.Clear();

        // Re-assert mode convars: the stock gamemode cfg resets money / intro /
        // vote convars on map load and on mp_restartgame, so without this a
        // rematch funds buys and gets stuck on the team-intro / map-vote screen.
        EnforceModeConvars();
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
        var c = p.Controller;
        if (c == null) return;
        if (c.Team != team)
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

        // Zero money + apply loadout shortly after spawn so the pawn/itemservices exist.
        Core.Scheduler.NextTick(() => { ZeroMoney(p); ApplyLoadout(p); });
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

        // In 1v1 both pawns can die in one round (mutual HE/molotov, simultaneous
        // trade). Only the FIRST death decides the round; later deaths this round
        // must not score or advance _roundsPlayed (which would corrupt first-to-N,
        // the halftime trigger, and OT parity). Cleared in OnRoundStart.
        if (_roundDecided) return HookResult.Continue;
        _roundDecided = true;

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
        if (_phase != Phase.Live) return HookResult.Continue;

        // New round: arm the round-decided latch again so the next death scores.
        _roundDecided = false;

        // Keep players pinned to the right side in case CS shuffled them, and keep
        // money at 0 (the engine re-grants start money on the round reset).
        AssignPlayers();
        ZeroMoneyAll();

        // After a side swap, SwitchTeam alone updates the scoreboard but the pawn
        // keeps the OLD spawn point + player model for one round. Force a clean
        // respawn so the NEW side applies on THIS round (fixes the one-round lag).
        if (_pendingSwapRespawn)
        {
            _pendingSwapRespawn = false;
            foreach (var p in Humans())
            {
                if (!IsPlayer(p.SteamID)) continue;
                var pl = p;
                Core.Scheduler.NextTick(() => { if (pl.IsValid) pl.Respawn(); });
            }
        }
        return HookResult.Continue;
    }

    private HookResult OnBuyMenuOpen(EventBuymenuOpen ev)
    {
        if (!_active) return HookResult.Continue;
        Broadcast("Buying is disabled — use weapon commands (\x04!guns\x01 to list).");
        // Stop on this PRE hook actually cancels the buy menu (Continue only hinted).
        return HookResult.Stop;
    }

    // --- map-change feedback (workshop download edges + new-map live) -----

    private HookResult OnUgcDownloadStart(EventUgcFileDownloadStart ev)
    {
        if (!_active) return HookResult.Continue;
        if (_pendingWorkshopId == 0 || _downloadAnnounced) return HookResult.Continue;
        _downloadAnnounced = true;
        Broadcast($"\x04Downloading\x01 \x04{_pendingMapLabel}\x01 from the Workshop — please wait.");
        Core.PlayerManager.SendCenterHTML($"Downloading workshop map<br><b>{_pendingMapLabel}</b><br>please wait…", 10000);
        return HookResult.Continue;
    }

    private HookResult OnUgcDownloadFinished(EventUgcFileDownloadFinished ev)
    {
        if (!_active) return HookResult.Continue;
        if (_pendingWorkshopId == 0 || !_downloadAnnounced) return HookResult.Continue;
        Broadcast("Workshop download \x04complete\x01 — loading map…");
        Core.PlayerManager.SendCenterHTML("Download complete<br>loading map…", 6000);
        return HookResult.Continue;
    }

    private HookResult OnUgcDownloadError(EventUgcMapDownloadError ev)
    {
        if (!_active) return HookResult.Continue;
        if (_pendingWorkshopId == 0) return HookResult.Continue;
        Broadcast($"\x04Workshop download FAILED\x01 (error {ev.ErrorCode}) — staying on the current map. Try \x04!maps\x01 for the pool.");
        Core.PlayerManager.SendCenterHTML("Workshop download failed", 6000);
        _pendingWorkshopId = 0;
        _pendingMapLabel = "";
        _downloadAnnounced = false;
        return HookResult.Continue;
    }

    private HookResult OnGameNewMap(EventGameNewmap ev)
    {
        if (!_active) return HookResult.Continue;
        // The new map is live; clear any pending-change feedback state.
        _pendingWorkshopId = 0;
        _pendingMapLabel = "";
        _downloadAnnounced = false;
        Broadcast($"Now playing \x04{ev.MapName}\x01.");
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
            SwapSides();
        }

        // Overtime trigger: regulation exhausted and tied.
        int regulationTotal = 2 * _mr;
        if (_otNumber == 0 && _roundsPlayed >= regulationTotal && _score1 == _score2)
        {
            _otNumber = 1;
            _sidesSwapped = false;
            Broadcast($"\x04Overtime!\x01 First to {WinTarget()} (MR{OvertimeMr}).");
            SwapSides();
        }
        else if (_otNumber > 0)
        {
            // OT halftime swap at OvertimeMr rounds into this OT period.
            int otRoundsPlayed = _roundsPlayed - regulationTotal - (_otNumber - 1) * (2 * OvertimeMr);
            if (!_sidesSwapped && otRoundsPlayed >= OvertimeMr)
            {
                _sidesSwapped = true;
                Broadcast("\x04OT halftime — switching sides.\x01");
                SwapSides();
            }
            // Next OT if this one ended tied.
            int otTotal = 2 * OvertimeMr;
            if (otRoundsPlayed >= otTotal && _score1 == _score2)
            {
                _otNumber++;
                _sidesSwapped = false;
                Broadcast($"\x04Another overtime!\x01 First to {WinTarget()}.");
                SwapSides();
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

    // --- side swap / money / convar enforcement --------------------------

    /// <summary>
    /// Apply a side change: flip controller teams now (scoreboard) and flag a
    /// forced respawn at the next live round start so the new side's spawn point
    /// and player model take effect immediately. IPlayer.SwitchTeam alone is a
    /// silent team flip; the pawn keeps the old spawn/model until a fresh spawn,
    /// which is the one-round halftime-swap lag we are fixing.
    /// </summary>
    private void SwapSides()
    {
        AssignPlayers();
        _pendingSwapRespawn = true;
    }

    /// <summary>Force a player's money to 0 (buying is disabled in 1v1).</summary>
    private void ZeroMoney(IPlayer p)
    {
        if (!p.IsValid) return;
        var c = p.Controller;
        if (c == null) return;
        var money = c.InGameMoneyServices;
        if (money == null) return;
        money.Account = 0;
        money.StartAccount = 0;
    }

    private void ZeroMoneyAll()
    {
        foreach (var p in Humans()) ZeroMoney(p);
    }

    /// <summary>
    /// Re-assert the convars the stock gamemode cfg clobbers on map load /
    /// mp_restartgame: keep money at 0, kill the team-intro flyover and the
    /// native end-match map-vote UI (voting is via !votemap), and keep the 1v1
    /// freezetime short. Plugin-side so it always wins regardless of cfg exec order.
    /// </summary>
    private void EnforceModeConvars()
    {
        Server("mp_maxmoney 0");
        Server("mp_startmoney 0");
        Server("mp_afterroundmoney 0");
        Server("mp_buytime 0");
        Server("mp_buy_anywhere 0");
        Server("mp_team_intro_time 0");
        Server("mp_endmatch_votenextmap 0");
        Server("mp_endmatch_votenextleveltime 0");
        Server("mp_match_end_changelevel 0");
        Server("mp_match_end_restart 0");
        Server("mp_freezetime 1");
    }

    private string NameOf(ulong sid)
    {
        var p = Humans().FirstOrDefault(x => x.SteamID == sid);
        return p?.Name ?? "Player";
    }
}
