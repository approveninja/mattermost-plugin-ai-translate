package codexauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/approveninja/mattermost-plugin-ai-translate/server/translate"
)

func TestRefreshRotatesToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "refresh_token", r.Form.Get("grant_type"))
		assert.Equal(t, "old-refresh", r.Form.Get("refresh_token"))
		assert.Equal(t, clientID, r.Form.Get("client_id"))
		_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh"}`))
	}))
	defer srv.Close()

	got, err := refreshTokens(context.Background(), srv.Client(), srv.URL, "old-refresh")
	require.NoError(t, err)
	assert.Equal(t, "new-access", got.AccessToken)
	assert.Equal(t, "new-refresh", got.RefreshToken)
	assert.NotEmpty(t, got.LastRefresh)
}

func TestRefreshKeepsOldRefreshWhenNotRotated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"new-access"}`))
	}))
	defer srv.Close()
	got, err := refreshTokens(context.Background(), srv.Client(), srv.URL, "keep-me")
	require.NoError(t, err)
	assert.Equal(t, "keep-me", got.RefreshToken)
}

func TestRefreshInvalidGrantIsRelogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()
	_, err := refreshTokens(context.Background(), srv.Client(), srv.URL, "x")
	assert.True(t, errors.Is(err, translate.ErrReloginRequired))
}

func TestRefresh429IsQuota(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	_, err := refreshTokens(context.Background(), srv.Client(), srv.URL, "x")
	assert.True(t, errors.Is(err, translate.ErrQuota))
}
