package bot

import (
	"github.com/bwmarrin/discordgo"

	"github.com/brandonli/cs2-server/internal/gamemode"
)

// optInt is a helper for optional integer min values in command options.
func optFloat(f float64) *float64 { return &f }

// modeChoices builds the Discord choice list for the /create mode option from
// the game-mode preset registry, so the bot and orchestrator stay in sync.
func modeChoices() []*discordgo.ApplicationCommandOptionChoice {
	presets := gamemode.All()
	out := make([]*discordgo.ApplicationCommandOptionChoice, 0, len(presets))
	for _, p := range presets {
		out = append(out, &discordgo.ApplicationCommandOptionChoice{
			Name:  p.Name + " — " + p.Description,
			Value: p.Name,
		})
	}
	return out
}

// commandDefs returns the slash-command schema registered with Discord.
func commandDefs() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "create",
			Description: "Create an on-demand CS2 server",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "map",
					Description: "Start map (e.g. de_inferno)",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "mode",
					Description: "Game mode preset",
					Required:    false,
					Choices:     modeChoices(),
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "name",
					Description: "Server name shown in the browser",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionBoolean,
					Name:        "public",
					Description: "Make the server Internet-joinable (requires GSLT)",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "maxplayers",
					Description: "Max players (default 10)",
					Required:    false,
					MinValue:    optFloat(1),
					MaxValue:    64,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "bots",
					Description: "Number of bots to add",
					Required:    false,
					MinValue:    optFloat(0),
					MaxValue:    32,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "password",
					Description: "Optional join password",
					Required:    false,
				},
			},
		},
		{
			Name:        "list",
			Description: "List your CS2 servers",
		},
		{
			Name:        "status",
			Description: "Show live status for a server",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "id",
					Description: "Server id",
					Required:    true,
				},
			},
		},
		{
			Name:        "restart",
			Description: "Restart one of your servers",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "id",
					Description: "Server id",
					Required:    true,
				},
			},
		},
		{
			Name:        "stop",
			Description: "Stop and remove one of your servers",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "id",
					Description: "Server id",
					Required:    true,
				},
			},
		},
	}
}
