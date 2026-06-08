package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/brandonli/cs2-server/internal/apiclient"
	"github.com/brandonli/cs2-server/internal/model"
)

func (b *Bot) handleCreate(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	b.defer_(s, i)

	opts := optionMap(i)
	req := apiclient.CreateRequest{
		OwnerID:    userID(i),
		Name:       opts.str("name", ""),
		Map:        opts.str("map", ""),
		Mode:       opts.str("mode", ""),
		MaxPlayers: int(opts.intv("maxplayers", 0)),
		BotQuota:   int(opts.intv("bots", 0)),
		Public:     opts.boolv("public", false),
		Password:   opts.str("password", ""),
	}

	inst, err := b.api.Create(ctx, req)
	if err != nil {
		b.followupErr(s, i, "create", err)
		return
	}

	visibility := "private (LAN)"
	if inst.Public {
		visibility = "public"
	}
	msg := fmt.Sprintf(
		"**Server created** `%s`\nName: %s\nMap: `%s`\nMode: `%s`\nVisibility: %s\nConnect: `connect %s`",
		inst.ID, orDash(inst.Name), inst.Map, orDash(inst.Mode), visibility, inst.Connect,
	)
	if inst.Public && req.GSLT == "" {
		msg += "\n_Note: public server started without a GSLT may not appear in the server browser._"
	}
	b.followup(s, i, msg)
}

func (b *Bot) handleList(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	b.defer_(s, i)

	owner := ""
	if b.ownerScoped {
		owner = userID(i)
	}
	list, err := b.api.List(ctx, owner)
	if err != nil {
		b.followupErr(s, i, "list", err)
		return
	}
	if len(list) == 0 {
		b.followup(s, i, "No servers running. Use `/create` to start one.")
		return
	}

	var sb strings.Builder
	sb.WriteString("**Your CS2 servers:**\n")
	for _, in := range list {
		sb.WriteString(fmt.Sprintf("- `%s` — %s — `%s` (%s) — `connect %s` — %s\n",
			in.ID, orDash(in.Name), in.Map, orDash(in.Mode), in.Connect, in.Status))
	}
	b.followup(s, i, sb.String())
}

func (b *Bot) handleStatus(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	b.defer_(s, i)
	id := optionMap(i).str("id", "")

	st, err := b.api.Status(ctx, id)
	if err != nil {
		b.followupErr(s, i, "status", err)
		return
	}
	online := "offline"
	if st.Online {
		online = "online"
	}
	b.followup(s, i, fmt.Sprintf(
		"**Server `%s`**\nState: %s\nMap: `%s`\nPlayers: %d/%d",
		id, online, orDash(st.Map), st.PlayerCount, st.MaxPlayers,
	))
}

func (b *Bot) handleRestart(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	b.defer_(s, i)
	id := optionMap(i).str("id", "")

	if err := b.guardOwnership(ctx, i, id); err != nil {
		b.followupErr(s, i, "restart", err)
		return
	}
	if err := b.api.Restart(ctx, id); err != nil {
		b.followupErr(s, i, "restart", err)
		return
	}
	b.followup(s, i, fmt.Sprintf("Server `%s` is restarting.", id))
}

func (b *Bot) handleStop(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	b.defer_(s, i)
	id := optionMap(i).str("id", "")

	if err := b.guardOwnership(ctx, i, id); err != nil {
		b.followupErr(s, i, "stop", err)
		return
	}
	if err := b.api.Stop(ctx, id); err != nil {
		b.followupErr(s, i, "stop", err)
		return
	}
	b.followup(s, i, fmt.Sprintf("Server `%s` stopped and removed.", id))
}

// guardOwnership ensures a user can only mutate their own servers when the bot
// is owner-scoped.
func (b *Bot) guardOwnership(ctx context.Context, i *discordgo.InteractionCreate, id string) error {
	if !b.ownerScoped {
		return nil
	}
	mine, err := b.api.List(ctx, userID(i))
	if err != nil {
		return err
	}
	for _, in := range mine {
		if in.ID == id {
			return nil
		}
	}
	return model.ErrNotFound
}

// --- option parsing ------------------------------------------------------

type options map[string]*discordgo.ApplicationCommandInteractionDataOption

func optionMap(i *discordgo.InteractionCreate) options {
	m := options{}
	for _, o := range i.ApplicationCommandData().Options {
		m[o.Name] = o
	}
	return m
}

func (o options) str(name, def string) string {
	if v, ok := o[name]; ok {
		return v.StringValue()
	}
	return def
}

func (o options) intv(name string, def int64) int64 {
	if v, ok := o[name]; ok {
		return v.IntValue()
	}
	return def
}

func (o options) boolv(name string, def bool) bool {
	if v, ok := o[name]; ok {
		return v.BoolValue()
	}
	return def
}

// --- reply helpers (all ephemeral) ---------------------------------------

func (b *Bot) defer_(s *discordgo.Session, i *discordgo.InteractionCreate) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})
}

func (b *Bot) respond(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: msg, Flags: discordgo.MessageFlagsEphemeral},
	})
}

func (b *Bot) followup(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	_, err := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
		Content: msg,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
	if err != nil {
		b.log.Error("followup failed", "err", err)
	}
}

func (b *Bot) followupErr(s *discordgo.Session, i *discordgo.InteractionCreate, action string, err error) {
	var apiErr *apiclient.APIError
	msg := fmt.Sprintf("Failed to %s: %v", action, err)
	if errors.As(err, &apiErr) {
		msg = fmt.Sprintf("Failed to %s: %s", action, apiErr.Message)
	} else if errors.Is(err, model.ErrNotFound) {
		msg = fmt.Sprintf("Failed to %s: server not found or not yours.", action)
	}
	b.log.Warn("command error", "action", action, "err", err)
	b.followup(s, i, msg)
}

// --- misc ----------------------------------------------------------------

func userID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
