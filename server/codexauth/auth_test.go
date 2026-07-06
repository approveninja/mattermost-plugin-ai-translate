package codexauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/approveninja/mattermost-plugin-ai-translate/server/translate"
)

type noopLocker struct{}

func (noopLocker) Lock(context.Context) (func(), error) { return func() {}, nil }

func jwtWithExp(t *testing.T, exp int64) string {
	payload, _ := json.Marshal(map[string]any{"exp": exp})
	seg := base64.RawURLEncoding.EncodeToString(payload)
	return "h." + seg + ".s"
}

func TestAccessTokenNotConfigured(t *testing.T) {
	a := NewAuthenticator(NewStore(newFakeKV()), noopLocker{}, http.DefaultClient, "")
	_, err := a.AccessToken(context.Background())
	assert.True(t, errors.Is(err, translate.ErrNotConfigured))
}

func TestAccessTokenReturnsValidWithoutRefresh(t *testing.T) {
	s := NewStore(newFakeKV())
	require.NoError(t, s.Save(Tokens{AccessToken: jwtWithExp(t, time.Now().Add(time.Hour).Unix()), RefreshToken: "r"}))
	a := NewAuthenticator(s, noopLocker{}, http.DefaultClient, "http://should-not-be-called")
	tok, err := a.AccessToken(context.Background())
	require.NoError(t, err)
	assert.Contains(t, tok, ".")
}

func TestAccessTokenRefreshesWhenExpiring(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		fresh := jwtWithExp(t, time.Now().Add(time.Hour).Unix())
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"r2"}`, fresh)
	}))
	defer srv.Close()

	kv := newFakeKV()
	s := NewStore(kv)
	require.NoError(t, s.Save(Tokens{AccessToken: jwtWithExp(t, time.Now().Add(time.Minute).Unix()), RefreshToken: "r1"}))
	a := NewAuthenticator(s, noopLocker{}, srv.Client(), srv.URL)

	tok, err := a.AccessToken(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, tok)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))

	// Rotated refresh token persisted.
	stored, _, _ := s.Load()
	assert.Equal(t, "r2", stored.RefreshToken)
}
