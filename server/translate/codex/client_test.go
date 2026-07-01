package codex

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/approveninja/mattermost-plugin-ai-translate/server/translate"
)

type fakeTokens struct{ tok string }

func (f fakeTokens) AccessToken(context.Context) (string, error) { return f.tok, nil }

func TestTranslateSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/responses", r.URL.Path)
		assert.Equal(t, "Bearer tok123", r.Header.Get("Authorization"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "gpt-5.5", body["model"])
		assert.Equal(t, false, body["store"])
		assert.Contains(t, body["instructions"], "English")
		assert.Equal(t, "Hola", body["input"])
		_, _ = io.WriteString(w, `{"output_text":"Hello"}`)
	}))
	defer srv.Close()

	c := New(fakeTokens{"tok123"}, func() string { return "gpt-5.5" }, srv.URL, srv.Client())
	got, err := c.Translate(context.Background(), "Hola", "EN")
	require.NoError(t, err)
	assert.Equal(t, "Hello", got)
}

func TestTranslateUnauthorizedMapsToRelogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := New(fakeTokens{"x"}, func() string { return "gpt-5.5" }, srv.URL, srv.Client())
	_, err := c.Translate(context.Background(), "Hola", "EN")
	assert.True(t, errors.Is(err, translate.ErrReloginRequired))
}

func TestTranslateRateLimitedMapsToQuota(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := New(fakeTokens{"x"}, func() string { return "gpt-5.5" }, srv.URL, srv.Client())
	_, err := c.Translate(context.Background(), "Hola", "EN")
	assert.True(t, errors.Is(err, translate.ErrQuota))
}

func TestTranslateUnsupportedLanguage(t *testing.T) {
	c := New(fakeTokens{"x"}, func() string { return "gpt-5.5" }, "http://unused", http.DefaultClient)
	_, err := c.Translate(context.Background(), "Hola", "XX")
	assert.ErrorContains(t, err, "unsupported")
	_ = strings.TrimSpace // keep import if unused elsewhere
}

func TestTranslateUsesDynamicModel(t *testing.T) {
	var capturedModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		capturedModel, _ = body["model"].(string)
		_, _ = io.WriteString(w, `{"output_text":"Hello"}`)
	}))
	defer srv.Close()

	model := "gpt-5.5"
	c := New(fakeTokens{"t"}, func() string { return model }, srv.URL, srv.Client())

	// First call: expect the initial model.
	_, err := c.Translate(context.Background(), "Hola", "EN")
	require.NoError(t, err)
	assert.Equal(t, "gpt-5.5", capturedModel)

	// Change the model without rebuilding the client.
	model = "gpt-5.5-mini"

	// Second call: expect the updated model.
	_, err = c.Translate(context.Background(), "Hola", "EN")
	require.NoError(t, err)
	assert.Equal(t, "gpt-5.5-mini", capturedModel)
}
