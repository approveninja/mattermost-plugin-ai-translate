package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/approveninja/mattermost-plugin-ai-translate/server/translate"
)

type stubTranslator struct {
	out string
	err error
}

func (s stubTranslator) Translate(_ context.Context, _, _ string) (string, error) {
	return s.out, s.err
}

func TestHandleTranslateSuccess(t *testing.T) {
	p := &Plugin{translator: stubTranslator{out: "Hello"}}
	p.router = p.initRouter()

	body, _ := json.Marshal(map[string]string{"text": "Hola", "targetLang": "EN"})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/translate", bytes.NewReader(body))
	r.Header.Set("Mattermost-User-ID", "u1")
	p.ServeHTTP(nil, w, r)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Hello", resp["translatedText"])
}

func TestHandleTranslateRequiresAuth(t *testing.T) {
	p := &Plugin{translator: stubTranslator{out: "x"}}
	p.router = p.initRouter()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/translate", bytes.NewReader([]byte(`{"text":"a","targetLang":"EN"}`)))
	p.ServeHTTP(nil, w, r)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandleTranslateMapsErrors(t *testing.T) {
	p := &Plugin{translator: stubTranslator{err: translate.ErrNotConfigured}}
	p.router = p.initRouter()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/translate", bytes.NewReader([]byte(`{"text":"a","targetLang":"EN"}`)))
	r.Header.Set("Mattermost-User-ID", "u1")
	p.ServeHTTP(nil, w, r)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "isn't set up")
}
