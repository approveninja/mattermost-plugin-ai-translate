package command

import (
	"context"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAdmin struct {
	connected bool
	loggedOut bool
}

func (f *fakeAdmin) Status() bool  { return f.connected }
func (f *fakeAdmin) Logout() error { f.loggedOut = true; return nil }
func (f *fakeAdmin) Login(context.Context) (string, string, func() error, error) {
	return "ABCD-1234", "https://auth.openai.com/codex/device", func() error { return nil }, nil
}

func newTestHandler(admin *fakeAdmin, admins map[string]bool) *Handler {
	return &Handler{
		admin:      admin,
		isSysAdmin: func(uid string) bool { return admins[uid] },
	}
}

func TestStatusCommand(t *testing.T) {
	h := newTestHandler(&fakeAdmin{connected: true}, map[string]bool{"a": true})
	resp, err := h.Handle(&model.CommandArgs{UserId: "a", Command: "/aitranslate status"})
	require.NoError(t, err)
	assert.Contains(t, resp.Text, "connected")
}

func TestLoginRequiresAdmin(t *testing.T) {
	h := newTestHandler(&fakeAdmin{}, map[string]bool{"a": true})
	resp, err := h.Handle(&model.CommandArgs{UserId: "u", Command: "/aitranslate login"})
	require.NoError(t, err)
	assert.Contains(t, resp.Text, "administrator")
}

func TestLoginShowsCode(t *testing.T) {
	h := newTestHandler(&fakeAdmin{}, map[string]bool{"a": true})
	resp, err := h.Handle(&model.CommandArgs{UserId: "a", Command: "/aitranslate login"})
	require.NoError(t, err)
	assert.Contains(t, resp.Text, "ABCD-1234")
	assert.Contains(t, resp.Text, "codex/device")
}

func TestLogout(t *testing.T) {
	admin := &fakeAdmin{connected: true}
	h := newTestHandler(admin, map[string]bool{"a": true})
	_, err := h.Handle(&model.CommandArgs{UserId: "a", Command: "/aitranslate logout"})
	require.NoError(t, err)
	assert.True(t, admin.loggedOut)
}
