package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

// CodexAdmin drives the admin OAuth lifecycle from the slash command.
type CodexAdmin interface {
	Status() bool
	Logout() error
	Login(ctx context.Context) (userCode, verificationURL string, wait func() error, err error)
}

// Handler implements the /aitranslate slash command.
type Handler struct {
	client     *pluginapi.Client
	admin      CodexAdmin
	isSysAdmin func(userID string) bool
}

// Command is the interface consumed by plugin.go.
type Command interface {
	Handle(args *model.CommandArgs) (*model.CommandResponse, error)
}

const trigger = "aitranslate"

// NewCommandHandler registers the /aitranslate command and returns a Command.
func NewCommandHandler(client *pluginapi.Client, admin CodexAdmin, isSysAdmin func(string) bool) Command {
	err := client.SlashCommand.Register(&model.Command{
		Trigger:          trigger,
		AutoComplete:     true,
		AutoCompleteDesc: "Manage AI Translate",
		AutoCompleteHint: "[login|status|logout]",
		AutocompleteData: model.NewAutocompleteData(trigger, "[login|status|logout]", "Manage AI Translate Codex connection"),
	})
	if err != nil {
		client.Log.Error("Failed to register command", "error", err)
	}
	return &Handler{client: client, admin: admin, isSysAdmin: isSysAdmin}
}

func ephemeral(text string) *model.CommandResponse {
	return &model.CommandResponse{ResponseType: model.CommandResponseTypeEphemeral, Text: text}
}

// Handle dispatches /aitranslate [login|status|logout].
func (h *Handler) Handle(args *model.CommandArgs) (*model.CommandResponse, error) {
	fields := strings.Fields(args.Command)
	sub := ""
	if len(fields) >= 2 {
		sub = strings.ToLower(fields[1])
	}
	switch sub {
	case "status":
		if h.admin.Status() {
			return ephemeral("AI Translate is connected to Codex."), nil
		}
		return ephemeral("AI Translate is not connected. An admin can run `/aitranslate login`."), nil
	case "logout":
		if !h.isSysAdmin(args.UserId) {
			return ephemeral("Only a system administrator can do that."), nil
		}
		if err := h.admin.Logout(); err != nil {
			return ephemeral("Logout failed: " + err.Error()), nil
		}
		return ephemeral("Disconnected from Codex."), nil
	case "login":
		if !h.isSysAdmin(args.UserId) {
			return ephemeral("Only a system administrator can connect Codex."), nil
		}
		code, url, wait, err := h.admin.Login(context.Background())
		if err != nil {
			return ephemeral("Could not start login: " + err.Error()), nil
		}
		go func() {
			if err := wait(); err != nil {
				h.client.Log.Error("Codex device login failed", "err", err)
			}
		}()
		return ephemeral(fmt.Sprintf(
			"To connect Codex:\n1. Open %s\n2. Enter code: **%s**\n\nThen run `/aitranslate status` to confirm.",
			url, code)), nil
	default:
		return ephemeral("Usage: `/aitranslate [login|status|logout]`"), nil
	}
}
