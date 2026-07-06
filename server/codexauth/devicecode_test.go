package codexauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBeginDeviceLoginParsesStringInterval(t *testing.T) {
	// OpenAI returns `interval` as a JSON string (e.g. "7"), not a number.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"user_code":"AB-12","device_auth_id":"d1","interval":"7"}`))
	}))
	defer srv.Close()

	a := NewAuthenticator(NewStore(newFakeKV()), noopLocker{}, srv.Client(), srv.URL)
	dl, err := a.BeginDeviceLogin(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "AB-12", dl.UserCode)
	assert.Equal(t, 7, dl.Interval)
}

func TestDeviceLoginFlow(t *testing.T) {
	poll := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/api/accounts/deviceauth/usercode", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"user_code":"ABCD-1234","device_auth_id":"dev1","interval":1}`))
	})
	mux.HandleFunc("/api/accounts/deviceauth/token", func(w http.ResponseWriter, r *http.Request) {
		poll++
		if poll < 2 {
			w.WriteHeader(http.StatusNotFound) // pending
			return
		}
		_, _ = w.Write([]byte(`{"authorization_code":"auth1","code_verifier":"ver1"}`))
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "authorization_code", r.Form.Get("grant_type"))
		assert.Equal(t, "auth1", r.Form.Get("code"))
		assert.Equal(t, "ver1", r.Form.Get("code_verifier"))
		_, _ = w.Write([]byte(`{"access_token":"acc","refresh_token":"ref"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	s := NewStore(newFakeKV())
	a := NewAuthenticator(s, noopLocker{}, srv.Client(), srv.URL)

	dl, err := a.BeginDeviceLogin(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ABCD-1234", dl.UserCode)
	assert.Contains(t, dl.VerificationURL, "/codex/device")

	done, err := a.PollDeviceLogin(context.Background(), dl)
	require.NoError(t, err)
	assert.False(t, done) // first poll pending
	done, err = a.PollDeviceLogin(context.Background(), dl)
	require.NoError(t, err)
	assert.True(t, done)

	stored, ok, _ := s.Load()
	require.True(t, ok)
	assert.Equal(t, "acc", stored.AccessToken)
	assert.Equal(t, "ref", stored.RefreshToken)
}
