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
	// Created servers are announced publicly (non-ephemeral) so the channel can
	// see and use the connect string.
	b.deferPublic(s, i)

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
		b.editErr(s, i, "create", err)
		return
	}

	visibility := "🔒 private (LAN)"
	if inst.Public {
		visibility = "🌐 public"
	}

	embed := &discordgo.MessageEmbed{
		Title:       "✅ Server created",
		Color:       0x57F287, // Discord green
		Description: fmt.Sprintf("Connect: `connect %s`", inst.Connect),
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Name", Value: orDash(inst.Name), Inline: true},
			{Name: "Map", Value: codeOrDash(inst.Map), Inline: true},
			{Name: "Mode", Value: codeOrDash(inst.Mode), Inline: true},
			{Name: "Visibility", Value: visibility, Inline: true},
			{Name: "ID", Value: "`" + inst.ID + "`", Inline: true},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: "cs2-server"},
	}
	if inst.Public && req.GSLT == "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:  "⚠️ Note",
			Value: "Public server started without a GSLT may not appear in the server browser.",
		})
	}
	b.editEmbed(s, i, embed)
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

func (b *Bot) handleKillAll(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) {
	b.defer_(s, i)

	// Guild admins (Administrator / Manage Server) can stop every server in the
	// guild; everyone else is limited to their own servers. An empty owner
	// targets all servers; a non-empty owner scopes the bulk stop to that user.
	owner := userID(i)
	scope := "your"
	if isAdmin(i) || !b.ownerScoped {
		owner = ""
		scope = "all"
	}

	res, err := b.api.StopAll(ctx, owner)
	if err != nil {
		b.followupErr(s, i, "kill all servers", err)
		return
	}
	if res.Stopped == 0 && len(res.Failed) == 0 {
		b.followup(s, i, "No servers to stop.")
		return
	}

	msg := fmt.Sprintf("Stopped and removed %d %s server(s).", res.Stopped, scope)
	if len(res.Failed) > 0 {
		msg += fmt.Sprintf("\n⚠️ Failed to stop %d: `%s`", len(res.Failed), strings.Join(res.Failed, "`, `"))
	}
	b.followup(s, i, msg)
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

// deferPublic acknowledges the interaction with a non-ephemeral deferred
// response (a public "thinking…" placeholder) that is later filled via
// editResponse/editEmbed. Use this for commands whose result should be visible
// to the whole channel.
func (b *Bot) deferPublic(s *discordgo.Session, i *discordgo.InteractionCreate) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{},
	})
}

// editEmbed fills the original (deferred) interaction response with an embed.
// Editing the original message keeps the deferred placeholder's visibility, so
// after deferPublic this produces exactly one public message.
func (b *Bot) editEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) {
	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
	if err != nil {
		b.log.Error("edit response failed", "err", err)
	}
}

// editErr fills the original (deferred) interaction response with an error
// message, matching the placeholder's visibility set at defer time.
func (b *Bot) editErr(s *discordgo.Session, i *discordgo.InteractionCreate, action string, err error) {
	b.log.Warn("command error", "action", action, "err", err)
	msg := b.errMsg(action, err)
	if _, e := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg}); e != nil {
		b.log.Error("edit error response failed", "err", e)
	}
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
	b.log.Warn("command error", "action", action, "err", err)
	b.followup(s, i, b.errMsg(action, err))
}

// errMsg renders a user-facing failure message for an action, unwrapping API
// and not-found errors into friendlier text.
func (b *Bot) errMsg(action string, err error) string {
	var apiErr *apiclient.APIError
	switch {
	case errors.As(err, &apiErr):
		return fmt.Sprintf("Failed to %s: %s", action, apiErr.Message)
	case errors.Is(err, model.ErrNotFound):
		return fmt.Sprintf("Failed to %s: server not found or not yours.", action)
	default:
		return fmt.Sprintf("Failed to %s: %v", action, err)
	}
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

// isAdmin reports whether the invoking guild member holds Administrator or
// Manage Server. Discord computes Member.Permissions for the interaction
// (including role + channel overrides), so no extra API call is needed.
func isAdmin(i *discordgo.InteractionCreate) bool {
	if i.Member == nil {
		return false
	}
	const adminMask = discordgo.PermissionAdministrator | discordgo.PermissionManageServer
	return i.Member.Permissions&adminMask != 0
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// codeOrDash wraps a non-empty value in inline code, or returns a dash.
func codeOrDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return "`" + s + "`"
}
