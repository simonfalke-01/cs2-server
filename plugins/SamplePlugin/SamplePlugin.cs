using CounterStrikeSharp.API;
using CounterStrikeSharp.API.Core;
using CounterStrikeSharp.API.Core.Attributes.Registration;
using CounterStrikeSharp.API.Modules.Commands;
using CounterStrikeSharp.API.Modules.Utils;
using Microsoft.Extensions.Logging;

namespace SamplePlugin;

/// <summary>
/// Minimal CounterStrikeSharp plugin proving that custom C# game logic loads
/// and runs inside the dedicated server. It:
///   - logs a banner when loaded (look for it in the server console),
///   - registers a "css_hello" chat/console command,
///   - hooks the round-start game event.
/// Use this as the template for real gameplay plugins.
/// </summary>
public class SamplePlugin : BasePlugin
{
    public override string ModuleName => "SamplePlugin";
    public override string ModuleVersion => "0.1.0";
    public override string ModuleAuthor => "cs2-server";
    public override string ModuleDescription => "Sample plugin proving custom C# logic loads.";

    public override void Load(bool hotReload)
    {
        Logger.LogInformation("SamplePlugin loaded successfully (hotReload={HotReload}).", hotReload);

        // Fires at the start of each round.
        RegisterEventHandler<EventRoundStart>(OnRoundStart);
    }

    /// <summary>
    /// Chat command: players type "!hello" / "/hello", or "css_hello" in console.
    /// </summary>
    [ConsoleCommand("css_hello", "Greets the caller. Proof the plugin is alive.")]
    [CommandHelper(whoCanExecute: CommandUsage.CLIENT_AND_SERVER)]
    public void OnHelloCommand(CCSPlayerController? player, CommandInfo command)
    {
        if (player is null)
        {
            command.ReplyToCommand("Hello from SamplePlugin (server console)!");
            return;
        }

        command.ReplyToCommand($"{ChatColors.Green}Hello {ChatColors.Gold}{player.PlayerName}{ChatColors.Green}, SamplePlugin is running!");
    }

    private HookResult OnRoundStart(EventRoundStart @event, GameEventInfo info)
    {
        Logger.LogInformation("Round started. SamplePlugin is handling game events.");
        Server.PrintToChatAll($"{ChatColors.Green}[SamplePlugin] {ChatColors.Default}New round, good luck!");
        return HookResult.Continue;
    }
}
