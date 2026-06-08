using SwiftlyS2.Shared;
using SwiftlyS2.Shared.Commands;
using SwiftlyS2.Shared.GameEvents;
using SwiftlyS2.Shared.GameEventDefinitions;
using SwiftlyS2.Shared.Misc;
using SwiftlyS2.Shared.Plugins;

namespace SamplePlugin;

/// <summary>
/// Minimal SwiftlyS2 plugin proving that custom C# game logic loads and runs
/// inside the dedicated server. It:
///   - logs a banner on load (look for it in the server console),
///   - registers a "sw_hello" command (also "!hello" in chat),
///   - hooks the round-start game event.
/// Use this as the template for real gameplay plugins.
/// </summary>
[PluginMetadata(
    Id = "SamplePlugin",
    Version = "0.1.0",
    Name = "SamplePlugin",
    Author = "cs2-server",
    Description = "Sample plugin proving custom C# logic loads.")]
public partial class SamplePlugin : BasePlugin
{
    public SamplePlugin(ISwiftlyCore core) : base(core) { }

    public override void ConfigureSharedInterface(IInterfaceManager interfaceManager) { }

    public override void UseSharedInterface(IInterfaceManager interfaceManager) { }

    public override void Load(bool hotReload)
    {
        Console.WriteLine($"[SamplePlugin] Loaded successfully (hotReload={hotReload}).");

        // Fires at the start of each round.
        Core.GameEvent.HookPre<EventRoundStart>(OnRoundStart);
    }

    public override void Unload() { }

    /// <summary>
    /// Chat/console command: players type "!hello" / "/hello", or "sw_hello"
    /// in the server console. Proof the plugin is alive.
    /// </summary>
    [Command("hello")]
    public void HelloCommand(ICommandContext context)
    {
        context.Reply("Hello from SamplePlugin — custom C# logic is running!");
    }

    private HookResult OnRoundStart(EventRoundStart @event)
    {
        Console.WriteLine("[SamplePlugin] Round started; handling game events.");
        return HookResult.Continue;
    }
}
