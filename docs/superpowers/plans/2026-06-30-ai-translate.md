# AI Translate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-message AI translation to Mattermost — an inline `[ → EN ▾ ]` control on each post that translates it via OpenAI Codex (ChatGPT OAuth) and toggles the displayed text.

**Architecture:** The Go server part of the plugin (running inside the Mattermost server) exposes HTTP endpoints under `/plugins/com.approveninja.ai-translate/api/v1/`. The webapp bundle renders the control and calls those endpoints. A `codexauth` package manages OAuth tokens (device-code login + single-use-refresh rotation, cluster-locked) in the KV store; a thin `Translator` interface with one `codex` implementation does the actual translation. All outbound calls use injectable base URLs and `*http.Client`, so every unit test runs offline against `httptest`.

**Tech Stack:** Go 1.25, `gorilla/mux`, `mattermost/server/public/pluginapi` (+ `pluginapi/cluster`), `stretchr/testify`; webapp: React/TypeScript, Jest.

## Global Constraints

- All code, comments, docs, commit messages in **English**.
- Go module: `github.com/approveninja/mattermost-plugin-ai-translate`.
- Plugin ID: `com.approveninja.ai-translate`. Min server version: 6.2.1.
- Codex constants (verbatim): client_id `app_EMoamEEZ73f0CkXaXp7hrann`; OAuth token URL `https://auth.openai.com/oauth/token`; device usercode `https://auth.openai.com/api/accounts/deviceauth/usercode`; device token `https://auth.openai.com/api/accounts/deviceauth/token`; exchange redirect_uri `https://auth.openai.com/deviceauth/callback`; verification URL shown to admin `https://auth.openai.com/codex/device`; API base `https://chatgpt.com/backend-api/codex`; default model `gpt-5.5`.
- Refresh tokens are **single-use**: always persist the rotated `refresh_token` returned by a refresh.
- Tokens live in the KV store only, never returned to clients.
- Supported languages (code → English name), default `EN`: EN→English, UA→Ukrainian, RU→Russian, PL→Polish, ES→Spanish, FR→French, DE→German, ZH→Chinese, IT→Italian, PT→Portuguese, JA→Japanese, TR→Turkish.
- Tests use `testify`; HTTP-dependent code is tested against `httptest.Server` via injectable base URLs.
- Run server tests with `go test ./server/...`; a single test with `go test ./server/... -run TestName -v`.

---

## Task 0: Live spikes (interactive — requires admin ChatGPT login)

These validate reality before building on assumptions (spec §11). They need a human to complete the browser login with the admin's ChatGPT account; an agent cannot do this step alone. Run them first and record findings in this plan.

- [ ] **Step 1: Write a throwaway Go spike for device-code + refresh**

Create `server/codexauth/spike/main.go` (excluded from build via `//go:build ignore`):

```go
//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const clientID = "app_EMoamEEZ73f0CkXaXp7hrann"

func main() {
	// 1. Request user code.
	body, _ := json.Marshal(map[string]string{"client_id": clientID})
	resp, err := http.Post("https://auth.openai.com/api/accounts/deviceauth/usercode",
		"application/json", bytes.NewReader(body))
	must(err)
	var uc struct {
		UserCode     string `json:"user_code"`
		DeviceAuthID string `json:"device_auth_id"`
		Interval     int    `json:"interval"`
	}
	json.NewDecoder(resp.Body).Decode(&uc)
	resp.Body.Close()
	fmt.Printf("Open https://auth.openai.com/codex/device and enter code: %s\n", uc.UserCode)

	// 2. Poll for authorization_code.
	var code, verifier string
	for {
		time.Sleep(time.Duration(max(3, uc.Interval)) * time.Second)
		pb, _ := json.Marshal(map[string]string{"device_auth_id": uc.DeviceAuthID, "user_code": uc.UserCode})
		pr, err := http.Post("https://auth.openai.com/api/accounts/deviceauth/token",
			"application/json", bytes.NewReader(pb))
		must(err)
		if pr.StatusCode == 200 {
			var d struct{ AuthorizationCode, CodeVerifier string }
			json.NewDecoder(pr.Body).Decode(&d)
			pr.Body.Close()
			code, verifier = d.AuthorizationCode, d.CodeVerifier
			break
		}
		pr.Body.Close()
		fmt.Print(".")
	}

	// 3. Exchange code for tokens.
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code},
		"redirect_uri": {"https://auth.openai.com/deviceauth/callback"},
		"client_id":    {clientID}, "code_verifier": {verifier}}
	tr, err := http.Post("https://auth.openai.com/oauth/token",
		"application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	must(err)
	var tok struct{ AccessToken, RefreshToken string }
	json.NewDecoder(tr.Body).Decode(&tok)
	tr.Body.Close()
	fmt.Printf("access_token len=%d refresh_token len=%d\n", len(tok.AccessToken), len(tok.RefreshToken))

	// 4. Refresh, confirm rotation.
	rf := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tok.RefreshToken}, "client_id": {clientID}}
	rr, err := http.Post("https://auth.openai.com/oauth/token",
		"application/x-www-form-urlencoded", strings.NewReader(rf.Encode()))
	must(err)
	var ref struct{ AccessToken, RefreshToken string }
	json.NewDecoder(rr.Body).Decode(&ref)
	rr.Body.Close()
	fmt.Printf("refreshed: new access len=%d, refresh rotated=%v\n",
		len(ref.AccessToken), ref.RefreshToken != "" && ref.RefreshToken != tok.RefreshToken)

	// 5. Live /responses call (translation).
	rb, _ := json.Marshal(map[string]any{
		"model":        "gpt-5.5",
		"instructions": "You are a translation engine. Translate into English. Output only the translation.",
		"input":        "Hola, ¿cómo estás?",
		"store":        false,
		"stream":       false,
	})
	req, _ := http.NewRequest("POST", "https://chatgpt.com/backend-api/codex/responses", bytes.NewReader(rb))
	req.Header.Set("Authorization", "Bearer "+ref.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	cr, err := http.DefaultClient.Do(req)
	must(err)
	out, _ := io.ReadAll(cr.Body)
	cr.Body.Close()
	fmt.Printf("/responses status=%d body=%s\n", cr.StatusCode, string(out))
}

func must(err error) { if err != nil { panic(err) } }
func max(a, b int) int { if a > b { return a }; return b }
```

(Add `"io"` to imports when running.)

- [ ] **Step 2: Run the spike**

Run: `go run server/codexauth/spike/main.go`
Complete the browser login when prompted.
Record in this plan, under this task: (a) does refresh return a **rotated** refresh_token? (b) `/responses` status and the JSON shape of the translated output (which field holds the text); (c) whether extra headers (`originator`, `User-Agent`, `chatgpt-account-id`) were required for a 200. **The §"Codex response parsing" in Task 3 must match the real shape observed here.**

- [ ] **Step 3: Webapp hook spike**

In a scratch branch of the running MM webapp (or via the dev server), confirm `registry.registerMessageWillFormatHook((post, message) => …)` is available and can return modified text, and identify a hook to render a per-post control (e.g. `registerPostDropdownMenuAction` as the guaranteed fallback). Record which mechanism Task 15 will use.

- [ ] **Step 4: Remove the spike file**

```bash
git rm server/codexauth/spike/main.go 2>/dev/null; rmdir server/codexauth/spike 2>/dev/null; true
```

---

## Task 1: Language registry

**Files:**
- Create: `server/translate/languages.go`
- Test: `server/translate/languages_test.go`

**Interfaces:**
- Produces: `translate.Languages` (`[]translate.Language` where `Language{Code, Name string}`), `translate.LanguageName(code string) (string, bool)`, `translate.IsSupported(code string) bool`, `translate.DefaultLanguage = "EN"`.

- [ ] **Step 1: Write the failing test**

```go
package translate

import "testing"

import "github.com/stretchr/testify/assert"

func TestLanguageName(t *testing.T) {
	name, ok := LanguageName("ES")
	assert.True(t, ok)
	assert.Equal(t, "Spanish", name)

	name, ok = LanguageName("es") // case-insensitive
	assert.True(t, ok)
	assert.Equal(t, "Spanish", name)

	_, ok = LanguageName("XX")
	assert.False(t, ok)
}

func TestIsSupportedAndDefault(t *testing.T) {
	assert.True(t, IsSupported("EN"))
	assert.False(t, IsSupported("XX"))
	assert.Equal(t, "EN", DefaultLanguage)
	assert.Len(t, Languages, 12)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/translate/ -run TestLanguage -v`
Expected: FAIL (package/identifiers not defined).

- [ ] **Step 3: Write minimal implementation**

```go
package translate

import "strings"

// Language is a target language offered in the translate dropdown.
type Language struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// DefaultLanguage is the target used until the user picks another.
const DefaultLanguage = "EN"

// Languages is the fixed, ordered list shown in the UI.
var Languages = []Language{
	{"EN", "English"}, {"UA", "Ukrainian"}, {"RU", "Russian"}, {"PL", "Polish"},
	{"ES", "Spanish"}, {"FR", "French"}, {"DE", "German"}, {"ZH", "Chinese"},
	{"IT", "Italian"}, {"PT", "Portuguese"}, {"JA", "Japanese"}, {"TR", "Turkish"},
}

// LanguageName returns the English display name for a language code (case-insensitive).
func LanguageName(code string) (string, bool) {
	up := strings.ToUpper(strings.TrimSpace(code))
	for _, l := range Languages {
		if l.Code == up {
			return l.Name, true
		}
	}
	return "", false
}

// IsSupported reports whether code is in the supported set.
func IsSupported(code string) bool {
	_, ok := LanguageName(code)
	return ok
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/translate/ -run TestLanguage -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/translate/languages.go server/translate/languages_test.go
git commit -m "feat: add supported-language registry for translation"
```

---

## Task 2: Translator interface and typed errors

**Files:**
- Create: `server/translate/translator.go`
- Test: `server/translate/translator_test.go`

**Interfaces:**
- Produces: `translate.Translator` interface (`Translate(ctx context.Context, text, targetLang string) (string, error)`); sentinel errors `translate.ErrNotConfigured`, `translate.ErrReloginRequired`, `translate.ErrQuota`, `translate.ErrUpstream`; helper `translate.UserMessage(err error) string` mapping those to user-facing English text.

- [ ] **Step 1: Write the failing test**

```go
package translate

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserMessage(t *testing.T) {
	assert.Contains(t, UserMessage(ErrNotConfigured), "isn't set up")
	assert.Contains(t, UserMessage(ErrReloginRequired), "expired")
	assert.Contains(t, UserMessage(ErrQuota), "rate-limited")
	assert.Contains(t, UserMessage(ErrUpstream), "failed")
	assert.Contains(t, UserMessage(errors.New("boom")), "failed")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/translate/ -run TestUserMessage -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
package translate

import (
	"context"
	"errors"
)

// Translator turns source text into a target language.
type Translator interface {
	Translate(ctx context.Context, text, targetLang string) (string, error)
}

var (
	// ErrNotConfigured means no Codex credentials are stored yet.
	ErrNotConfigured = errors.New("translation not configured")
	// ErrReloginRequired means the stored OAuth session is invalid/expired.
	ErrReloginRequired = errors.New("codex relogin required")
	// ErrQuota means the provider rate-limited the request (credentials still valid).
	ErrQuota = errors.New("codex quota exhausted")
	// ErrUpstream is any other provider/transport failure.
	ErrUpstream = errors.New("codex upstream error")
)

// UserMessage maps an error to an English, end-user-facing message.
func UserMessage(err error) string {
	switch {
	case errors.Is(err, ErrNotConfigured):
		return "Translation isn't set up yet. An admin must connect Codex with /aitranslate login."
	case errors.Is(err, ErrReloginRequired):
		return "Codex sign-in expired. An admin must reconnect with /aitranslate login."
	case errors.Is(err, ErrQuota):
		return "Translation is rate-limited right now — try again shortly."
	default:
		return "Translation failed, please try again."
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/translate/ -run TestUserMessage -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/translate/translator.go server/translate/translator_test.go
git commit -m "feat: add Translator interface and typed translation errors"
```

---

## Task 3: Codex translation client

**Files:**
- Create: `server/translate/codex/client.go`
- Test: `server/translate/codex/client_test.go`

**Interfaces:**
- Consumes: `translate.ErrReloginRequired/ErrQuota/ErrUpstream`, `translate.LanguageName`.
- Produces: `codex.TokenSource` interface (`AccessToken(ctx context.Context) (string, error)`); `codex.New(tokens TokenSource, model, baseURL string, httpClient *http.Client) *Client`; `(*Client).Translate(ctx, text, targetLang) (string, error)`. `Client` satisfies `translate.Translator`.

> **Note:** the response-parsing in Step 3 must match the JSON shape recorded in Task 0 Step 2. The shape below assumes the OpenAI Responses convention (`output[].content[].text` plus the `output_text` convenience field). Adjust the `extractText` function to the observed shape if it differs.

- [ ] **Step 1: Write the failing test**

```go
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

	c := New(fakeTokens{"tok123"}, "gpt-5.5", srv.URL, srv.Client())
	got, err := c.Translate(context.Background(), "Hola", "EN")
	require.NoError(t, err)
	assert.Equal(t, "Hello", got)
}

func TestTranslateUnauthorizedMapsToRelogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := New(fakeTokens{"x"}, "gpt-5.5", srv.URL, srv.Client())
	_, err := c.Translate(context.Background(), "Hola", "EN")
	assert.True(t, errors.Is(err, translate.ErrReloginRequired))
}

func TestTranslateRateLimitedMapsToQuota(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := New(fakeTokens{"x"}, "gpt-5.5", srv.URL, srv.Client())
	_, err := c.Translate(context.Background(), "Hola", "EN")
	assert.True(t, errors.Is(err, translate.ErrQuota))
}

func TestTranslateUnsupportedLanguage(t *testing.T) {
	c := New(fakeTokens{"x"}, "gpt-5.5", "http://unused", http.DefaultClient)
	_, err := c.Translate(context.Background(), "Hola", "XX")
	assert.ErrorContains(t, err, "unsupported")
	_ = strings.TrimSpace // keep import if unused elsewhere
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/translate/codex/ -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/approveninja/mattermost-plugin-ai-translate/server/translate"
)

// TokenSource yields a currently-valid Codex access token, refreshing as needed.
type TokenSource interface {
	AccessToken(ctx context.Context) (string, error)
}

// Client calls the Codex (ChatGPT-backend) Responses API to translate text.
type Client struct {
	tokens  TokenSource
	model   string
	baseURL string
	http    *http.Client
}

// New builds a Codex translation client. baseURL defaults to the ChatGPT codex
// backend when empty; model defaults to gpt-5.5 when empty.
func New(tokens TokenSource, model, baseURL string, httpClient *http.Client) *Client {
	if model == "" {
		model = "gpt-5.5"
	}
	if baseURL == "" {
		baseURL = "https://chatgpt.com/backend-api/codex"
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{tokens: tokens, model: model, baseURL: strings.TrimRight(baseURL, "/"), http: httpClient}
}

func (c *Client) Translate(ctx context.Context, text, targetLang string) (string, error) {
	name, ok := translate.LanguageName(targetLang)
	if !ok {
		return "", fmt.Errorf("unsupported target language %q", targetLang)
	}
	token, err := c.tokens.AccessToken(ctx)
	if err != nil {
		return "", err
	}

	payload := map[string]any{
		"model": c.model,
		"instructions": "You are a translation engine. Translate the user's message into " + name +
			". Output only the translation, preserving Markdown, mentions, emoji, and code spans.",
		"input":  text,
		"store":  false,
		"stream": false,
	}
	buf, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(buf))
	if err != nil {
		return "", fmt.Errorf("%w: %v", translate.ErrUpstream, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", translate.ErrUpstream, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	switch {
	case resp.StatusCode == http.StatusOK:
		out, err := extractText(body)
		if err != nil {
			return "", fmt.Errorf("%w: %v", translate.ErrUpstream, err)
		}
		return out, nil
	case resp.StatusCode == http.StatusTooManyRequests:
		return "", translate.ErrQuota
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return "", translate.ErrReloginRequired
	default:
		return "", fmt.Errorf("%w: status %d: %s", translate.ErrUpstream, resp.StatusCode, string(body))
	}
}

// extractText pulls the translated string from a Responses API body. Adjust to
// the exact shape recorded in Task 0 if it differs.
func extractText(body []byte) (string, error) {
	var parsed struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	if strings.TrimSpace(parsed.OutputText) != "" {
		return parsed.OutputText, nil
	}
	for _, o := range parsed.Output {
		for _, ct := range o.Content {
			if strings.TrimSpace(ct.Text) != "" {
				return ct.Text, nil
			}
		}
	}
	return "", fmt.Errorf("no text in response: %s", string(body))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/translate/codex/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/translate/codex/
git commit -m "feat: add Codex Responses translation client"
```

---

## Task 4: Token store (KV)

**Files:**
- Create: `server/codexauth/store.go`
- Test: `server/codexauth/store_test.go`

**Interfaces:**
- Produces: `codexauth.Tokens{AccessToken, RefreshToken, LastRefresh string}`; `codexauth.KV` interface (`Get(key string, out any) error`; `Set(key string, value any) (bool, error)`; `Delete(key string) error`); `codexauth.Store` with `Load() (Tokens, bool, error)`, `Save(Tokens) error`, `Clear() error`; constant `codexauth.tokensKey = "codex_oauth_tokens"`.

> **Note:** `codexauth.KV` mirrors the subset of `pluginapi.Client.KV` we use, so tests inject a fake and production passes `&client.KV`.

- [ ] **Step 1: Write the failing test**

```go
package codexauth

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeKV struct{ data map[string][]byte }

func newFakeKV() *fakeKV { return &fakeKV{data: map[string][]byte{}} }

func (f *fakeKV) Get(key string, out any) error {
	b, ok := f.data[key]
	if !ok {
		return nil // pluginapi leaves out at zero value when key missing
	}
	return json.Unmarshal(b, out)
}
func (f *fakeKV) Set(key string, value any) (bool, error) {
	b, _ := json.Marshal(value)
	f.data[key] = b
	return true, nil
}
func (f *fakeKV) Delete(key string) error { delete(f.data, key); return nil }

func TestStoreSaveLoadClear(t *testing.T) {
	s := NewStore(newFakeKV())

	_, ok, err := s.Load()
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, s.Save(Tokens{AccessToken: "a", RefreshToken: "r", LastRefresh: "t"}))
	got, ok, err := s.Load()
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "a", got.AccessToken)
	assert.Equal(t, "r", got.RefreshToken)

	require.NoError(t, s.Clear())
	_, ok, err = s.Load()
	require.NoError(t, err)
	assert.False(t, ok)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/codexauth/ -run TestStore -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
package codexauth

import "github.com/pkg/errors"

const tokensKey = "codex_oauth_tokens"

// Tokens is the persisted Codex OAuth credential bundle.
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	LastRefresh  string `json:"last_refresh"`
}

// KV is the subset of pluginapi.Client.KV that codexauth needs.
type KV interface {
	Get(key string, out any) error
	Set(key string, value any) (bool, error)
	Delete(key string) error
}

// Store persists Tokens in the plugin KV store.
type Store struct{ kv KV }

func NewStore(kv KV) *Store { return &Store{kv: kv} }

// Load returns the stored tokens; ok is false when none are stored yet.
func (s *Store) Load() (Tokens, bool, error) {
	var t Tokens
	if err := s.kv.Get(tokensKey, &t); err != nil {
		return Tokens{}, false, errors.Wrap(err, "failed to load codex tokens")
	}
	if t.RefreshToken == "" {
		return Tokens{}, false, nil
	}
	return t, true, nil
}

// Save writes the tokens, overwriting any existing record.
func (s *Store) Save(t Tokens) error {
	if _, err := s.kv.Set(tokensKey, t); err != nil {
		return errors.Wrap(err, "failed to save codex tokens")
	}
	return nil
}

// Clear deletes the stored tokens (logout).
func (s *Store) Clear() error {
	if err := s.kv.Delete(tokensKey); err != nil {
		return errors.Wrap(err, "failed to clear codex tokens")
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/codexauth/ -run TestStore -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/codexauth/store.go server/codexauth/store_test.go
git commit -m "feat: add KV-backed Codex token store"
```

---

## Task 5: Token refresh (pure) with rotation and error classification

**Files:**
- Create: `server/codexauth/refresh.go`
- Test: `server/codexauth/refresh_test.go`

**Interfaces:**
- Consumes: `translate.ErrReloginRequired/ErrQuota/ErrUpstream`.
- Produces: `codexauth.refreshTokens(ctx, httpClient *http.Client, tokenURL, refreshToken string) (Tokens, error)` (package-private); constant `codexauth.clientID = "app_EMoamEEZ73f0CkXaXp7hrann"` and `codexauth.defaultTokenURL = "https://auth.openai.com/oauth/token"`.

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/codexauth/ -run TestRefresh -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
package codexauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/approveninja/mattermost-plugin-ai-translate/server/translate"
)

const (
	clientID        = "app_EMoamEEZ73f0CkXaXp7hrann"
	defaultTokenURL = "https://auth.openai.com/oauth/token"
)

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// refreshTokens exchanges a refresh token for a fresh access token. The returned
// Tokens always carries a usable refresh token (the rotated one when present,
// else the input). Errors are classified into translate.Err* sentinels.
func refreshTokens(ctx context.Context, httpClient *http.Client, tokenURL, refreshToken string) (Tokens, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return Tokens{}, translate.ErrReloginRequired
	}
	if tokenURL == "" {
		tokenURL = defaultTokenURL
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, fmt.Errorf("%w: %v", translate.ErrUpstream, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return Tokens{}, fmt.Errorf("%w: %v", translate.ErrUpstream, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusTooManyRequests {
		return Tokens{}, translate.ErrQuota
	}
	if resp.StatusCode != http.StatusOK {
		// OAuth error bodies: {"error":"invalid_grant"} (string) — these mean relogin.
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		switch e.Error {
		case "invalid_grant", "invalid_token", "invalid_request", "refresh_token_reused":
			return Tokens{}, translate.ErrReloginRequired
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return Tokens{}, translate.ErrReloginRequired
		}
		return Tokens{}, fmt.Errorf("%w: refresh status %d: %s", translate.ErrUpstream, resp.StatusCode, string(body))
	}

	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Tokens{}, fmt.Errorf("%w: %v", translate.ErrUpstream, err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return Tokens{}, translate.ErrReloginRequired
	}
	rotated := payload.RefreshToken
	if strings.TrimSpace(rotated) == "" {
		rotated = refreshToken
	}
	return Tokens{AccessToken: payload.AccessToken, RefreshToken: rotated, LastRefresh: nowRFC3339()}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/codexauth/ -run TestRefresh -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/codexauth/refresh.go server/codexauth/refresh_test.go
git commit -m "feat: add Codex token refresh with rotation and error classification"
```

---

## Task 6: Access-token provider (expiry check + cluster-locked refresh)

**Files:**
- Create: `server/codexauth/auth.go`
- Test: `server/codexauth/auth_test.go`

**Interfaces:**
- Consumes: `Store`, `refreshTokens`, `translate.ErrNotConfigured`.
- Produces: `codexauth.Locker` interface (`Lock(ctx context.Context) (func(), error)`); `codexauth.Authenticator` with `New(store *Store, locker Locker, httpClient *http.Client, tokenURL string) *Authenticator` and method `AccessToken(ctx context.Context) (string, error)` (satisfies `codex.TokenSource`); helper `accessTokenExpiringWithin(token string, skew time.Duration) bool` (JWT `exp` decode; treats unparseable tokens as expiring).

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/codexauth/ -run TestAccessToken -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
package codexauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/approveninja/mattermost-plugin-ai-translate/server/translate"
)

// Locker provides mutual exclusion across the cluster around token refresh.
type Locker interface {
	Lock(ctx context.Context) (unlock func(), err error)
}

// Authenticator returns valid access tokens, refreshing under a cluster lock.
type Authenticator struct {
	store    *Store
	locker   Locker
	http     *http.Client
	tokenURL string
}

func NewAuthenticator(store *Store, locker Locker, httpClient *http.Client, tokenURL string) *Authenticator {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Authenticator{store: store, locker: locker, http: httpClient, tokenURL: tokenURL}
}

const refreshSkew = 5 * time.Minute

// AccessToken returns a non-expired access token, refreshing if needed.
func (a *Authenticator) AccessToken(ctx context.Context) (string, error) {
	tokens, ok, err := a.store.Load()
	if err != nil {
		return "", err
	}
	if !ok {
		return "", translate.ErrNotConfigured
	}
	if !accessTokenExpiringWithin(tokens.AccessToken, refreshSkew) {
		return tokens.AccessToken, nil
	}

	unlock, err := a.locker.Lock(ctx)
	if err != nil {
		return "", err
	}
	defer unlock()

	// Re-read under lock: another holder may have refreshed already.
	tokens, ok, err = a.store.Load()
	if err != nil {
		return "", err
	}
	if !ok {
		return "", translate.ErrNotConfigured
	}
	if !accessTokenExpiringWithin(tokens.AccessToken, refreshSkew) {
		return tokens.AccessToken, nil
	}

	refreshed, err := refreshTokens(ctx, a.http, a.tokenURL, tokens.RefreshToken)
	if err != nil {
		return "", err
	}
	if err := a.store.Save(refreshed); err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}

// accessTokenExpiringWithin reports whether the JWT access token expires within
// skew. Unparseable tokens are treated as expiring (force a refresh attempt).
func accessTokenExpiringWithin(token string, skew time.Duration) bool {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return true
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return true
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil || claims.Exp == 0 {
		return true
	}
	return time.Now().Add(skew).After(time.Unix(claims.Exp, 0))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/codexauth/ -run TestAccessToken -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/codexauth/auth.go server/codexauth/auth_test.go
git commit -m "feat: add access-token provider with cluster-locked refresh"
```

---

## Task 7: Device-code login orchestration

**Files:**
- Create: `server/codexauth/devicecode.go`
- Test: `server/codexauth/devicecode_test.go`

**Interfaces:**
- Consumes: `Store`, `clientID`, `nowRFC3339`.
- Produces: `codexauth.DeviceLogin` struct (`UserCode, VerificationURL, DeviceAuthID string`, `Interval int`); `(*Authenticator).BeginDeviceLogin(ctx) (DeviceLogin, error)`; `(*Authenticator).PollDeviceLogin(ctx, DeviceLogin) (bool, error)` (returns done=true when tokens were obtained+saved, false to keep polling); endpoint base configurable via new `Authenticator.authBaseURL` field defaulting to `https://auth.openai.com`.

> **Note:** `New`/`NewAuthenticator` gains an `authBaseURL` param (use `""` for the real default). Update Task 6 callers/tests accordingly when implementing — change the constructor signature to `NewAuthenticator(store, locker, httpClient, authBaseURL string)` and derive `tokenURL = authBaseURL + "/oauth/token"`. Adjust Task 6 tests to pass the httptest URL as `authBaseURL` (the token handler path becomes `/oauth/token`).

- [ ] **Step 1: Write the failing test**

```go
package codexauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/codexauth/ -run TestDeviceLogin -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
package codexauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// DeviceLogin holds the state needed to display and poll a device-code login.
type DeviceLogin struct {
	UserCode        string
	VerificationURL string
	DeviceAuthID    string
	Interval        int
}

// BeginDeviceLogin requests a user code from the OAuth device endpoint.
func (a *Authenticator) BeginDeviceLogin(ctx context.Context) (DeviceLogin, error) {
	body, _ := json.Marshal(map[string]string{"client_id": clientID})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		a.authBase()+"/api/accounts/deviceauth/usercode", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return DeviceLogin{}, fmt.Errorf("device code request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return DeviceLogin{}, fmt.Errorf("device code request status %d: %s", resp.StatusCode, string(b))
	}
	var d struct {
		UserCode     string `json:"user_code"`
		DeviceAuthID string `json:"device_auth_id"`
		Interval     int    `json:"interval"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return DeviceLogin{}, err
	}
	if d.UserCode == "" || d.DeviceAuthID == "" {
		return DeviceLogin{}, fmt.Errorf("incomplete device code response")
	}
	if d.Interval < 3 {
		d.Interval = 3
	}
	return DeviceLogin{
		UserCode:        d.UserCode,
		VerificationURL: a.authBase() + "/codex/device",
		DeviceAuthID:    d.DeviceAuthID,
		Interval:        d.Interval,
	}, nil
}

// PollDeviceLogin polls once. done=true means tokens were obtained and saved.
func (a *Authenticator) PollDeviceLogin(ctx context.Context, dl DeviceLogin) (bool, error) {
	body, _ := json.Marshal(map[string]string{"device_auth_id": dl.DeviceAuthID, "user_code": dl.UserCode})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		a.authBase()+"/api/accounts/deviceauth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("device poll failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusForbidden, http.StatusNotFound:
		return false, nil // still pending
	case http.StatusOK:
		// fallthrough to exchange below
	default:
		b, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("device poll status %d: %s", resp.StatusCode, string(b))
	}

	var d struct {
		AuthorizationCode string `json:"authorization_code"`
		CodeVerifier      string `json:"code_verifier"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return false, err
	}
	if d.AuthorizationCode == "" || d.CodeVerifier == "" {
		return false, fmt.Errorf("incomplete device token response")
	}

	tokens, err := a.exchangeCode(ctx, d.AuthorizationCode, d.CodeVerifier)
	if err != nil {
		return false, err
	}
	if err := a.store.Save(tokens); err != nil {
		return false, err
	}
	return true, nil
}

func (a *Authenticator) exchangeCode(ctx context.Context, code, verifier string) (Tokens, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {a.authBase() + "/deviceauth/callback"},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, a.tokenURL,
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.http.Do(req)
	if err != nil {
		return Tokens{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return Tokens{}, fmt.Errorf("token exchange status %d: %s", resp.StatusCode, string(b))
	}
	var d struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return Tokens{}, err
	}
	if d.AccessToken == "" {
		return Tokens{}, fmt.Errorf("token exchange returned no access_token")
	}
	return Tokens{AccessToken: d.AccessToken, RefreshToken: d.RefreshToken, LastRefresh: nowRFC3339()}, nil
}
```

Add to `auth.go` (Task 6): the `authBaseURL` field and helper, and derive `tokenURL`:

```go
// in Authenticator struct add:  authBaseURL string
// replace NewAuthenticator with:
func NewAuthenticator(store *Store, locker Locker, httpClient *http.Client, authBaseURL string) *Authenticator {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if authBaseURL == "" {
		authBaseURL = "https://auth.openai.com"
	}
	authBaseURL = strings.TrimRight(authBaseURL, "/")
	return &Authenticator{
		store: store, locker: locker, http: httpClient,
		authBaseURL: authBaseURL, tokenURL: authBaseURL + "/oauth/token",
	}
}

func (a *Authenticator) authBase() string { return a.authBaseURL }
```

(Update Task 6 tests: they already pass `srv.URL` as the 4th arg; ensure the refresh handler is registered at path `/oauth/token`, matching the derived `tokenURL`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/codexauth/ -v`
Expected: PASS (all codexauth tests).

- [ ] **Step 5: Commit**

```bash
git add server/codexauth/devicecode.go server/codexauth/devicecode_test.go server/codexauth/auth.go server/codexauth/auth_test.go
git commit -m "feat: add Codex device-code login orchestration"
```

---

## Task 8: Plugin configuration (model, default language)

**Files:**
- Modify: `plugin.json` (settings_schema)
- Modify: `server/configuration.go`
- Test: `server/configuration_test.go` (create)

**Interfaces:**
- Produces: `configuration{Model string; DefaultLanguage string}`; `(*configuration).model()` returns `Model` or `"gpt-5.5"`; `(*configuration).defaultLanguage()` returns a supported code or `"EN"`.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigurationDefaults(t *testing.T) {
	c := &configuration{}
	assert.Equal(t, "gpt-5.5", c.model())
	assert.Equal(t, "EN", c.defaultLanguage())

	c = &configuration{Model: "gpt-5.5-mini", DefaultLanguage: "ru"}
	assert.Equal(t, "gpt-5.5-mini", c.model())
	assert.Equal(t, "RU", c.defaultLanguage())

	c = &configuration{DefaultLanguage: "zz"} // unsupported → EN
	assert.Equal(t, "EN", c.defaultLanguage())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/ -run TestConfigurationDefaults -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Replace `type configuration struct{}` in `server/configuration.go` and add methods + import:

```go
import (
	"reflect"
	"strings"

	"github.com/pkg/errors"

	"github.com/approveninja/mattermost-plugin-ai-translate/server/translate"
)

type configuration struct {
	Model           string `json:"Model"`
	DefaultLanguage string `json:"DefaultLanguage"`
}

func (c *configuration) model() string {
	if strings.TrimSpace(c.Model) == "" {
		return "gpt-5.5"
	}
	return c.Model
}

func (c *configuration) defaultLanguage() string {
	up := strings.ToUpper(strings.TrimSpace(c.DefaultLanguage))
	if translate.IsSupported(up) {
		return up
	}
	return translate.DefaultLanguage
}
```

(Keep `Clone`, `getConfiguration`, `setConfiguration`, `OnConfigurationChange` unchanged. Note `setConfiguration`'s `NumField()==0` guard no longer triggers since the struct now has fields — that is fine.)

Add to `plugin.json` `settings_schema.settings` (array):

```json
{
  "key": "Model",
  "display_name": "Codex model",
  "type": "text",
  "help_text": "The Codex model used for translation.",
  "default": "gpt-5.5"
},
{
  "key": "DefaultLanguage",
  "display_name": "Default target language",
  "type": "dropdown",
  "help_text": "Default language new users translate into.",
  "default": "EN",
  "options": [
    {"display_name": "English", "value": "EN"},
    {"display_name": "Ukrainian", "value": "UA"},
    {"display_name": "Russian", "value": "RU"},
    {"display_name": "Polish", "value": "PL"},
    {"display_name": "Spanish", "value": "ES"},
    {"display_name": "French", "value": "FR"},
    {"display_name": "German", "value": "DE"},
    {"display_name": "Chinese", "value": "ZH"},
    {"display_name": "Italian", "value": "IT"},
    {"display_name": "Portuguese", "value": "PT"},
    {"display_name": "Japanese", "value": "JA"},
    {"display_name": "Turkish", "value": "TR"}
  ]
}
```

- [ ] **Step 4: Run test + validate manifest**

Run: `go test ./server/ -run TestConfigurationDefaults -v` → PASS
Run: `make check-style` or `go vet ./server/...` → no errors. Confirm `plugin.json` is valid JSON.

- [ ] **Step 5: Commit**

```bash
git add plugin.json server/configuration.go server/configuration_test.go
git commit -m "feat: add Model and DefaultLanguage plugin settings"
```

---

## Task 9: Plugin wiring (construct auth + translator, cluster mutex)

**Files:**
- Modify: `server/plugin.go`
- Create: `server/clusterlock.go`

**Interfaces:**
- Produces: `Plugin.authenticator *codexauth.Authenticator`, `Plugin.translator translate.Translator`; a `clusterLocker` adapting `cluster.Mutex` to `codexauth.Locker`.

> **Note:** this task has no standalone unit test (it is wiring verified by `go build`/`go vet` and the API tests in Tasks 10–12). Keep it small.

- [ ] **Step 1: Implement the cluster locker adapter**

`server/clusterlock.go`:

```go
package main

import (
	"context"

	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"
)

// clusterLocker adapts a cluster-wide mutex to codexauth.Locker.
type clusterLocker struct {
	api plugin.API
	key string
}

func (l clusterLocker) Lock(ctx context.Context) (func(), error) {
	m, err := cluster.NewMutex(l.api, l.key)
	if err != nil {
		return nil, err
	}
	if err := m.LockWithContext(ctx); err != nil {
		return nil, err
	}
	return m.Unlock, nil
}
```

(Verify the `cluster.NewMutex` / `LockWithContext` signatures against the vendored `pluginapi/cluster` version; adjust if the API differs.)

- [ ] **Step 2: Wire into OnActivate**

In `server/plugin.go`, add fields to `Plugin`:

```go
authenticator *codexauth.Authenticator
translator    translate.Translator
```

In `OnActivate`, after `p.kvstore = ...`:

```go
store := codexauth.NewStore(&p.client.KV)
locker := clusterLocker{api: p.API, key: "codex_oauth_refresh"}
p.authenticator = codexauth.NewAuthenticator(store, locker, nil, "")
p.translator = codex.New(p.authenticator, p.getConfiguration().model(), "", nil)
```

Add imports for `codexauth`, `codex`, `translate`. (`&p.client.KV` satisfies `codexauth.KV` — verify the method set: `Get(string, any) error`, `Set(string, any, ...KVSetOption) (bool, error)`, `Delete(string) error`. If `Set` has variadic options, define `codexauth.KV.Set` to match, or wrap with a tiny adapter struct in this file.)

- [ ] **Step 3: Build**

Run: `go build ./server/...` → success
Run: `go vet ./server/...` → no errors

- [ ] **Step 4: Commit**

```bash
git add server/plugin.go server/clusterlock.go
git commit -m "feat: wire Codex authenticator and translator into the plugin"
```

---

## Task 10: Translate API endpoint

**Files:**
- Modify: `server/api.go`
- Test: `server/api_test.go` (create)

**Interfaces:**
- Consumes: `Plugin.translator`, `translate.UserMessage`.
- Produces: `POST /api/v1/translate` accepting `{"postId":"","text":"","targetLang":"EN"}` → `200 {"translatedText":"..."}` or an error status with `{"error":"<user message>"}`. When `postId` is set, the server loads the post text via `p.client.Post.GetPost`; otherwise it uses `text`.

> **Note:** to keep the handler unit-testable without a full pluginapi, route translation through a small `Plugin` method `translatePost(ctx, userID, req)` that depends only on `p.translator` and a post-text resolver func field `p.resolvePostText func(postID string) (string, error)` set in `OnActivate` to `func(id string){ post, err := p.client.Post.GetPost(id); ... }`. Tests set `p.translator` to a stub and `p.resolvePostText` to a stub.

- [ ] **Step 1: Write the failing test**

```go
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

func (s stubTranslator) Translate(context.Context, string, string) (string, error) {
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/ -run TestHandleTranslate -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

In `server/api.go`, register the route in `initRouter` and add the handler:

```go
apiRouter.HandleFunc("/translate", p.handleTranslate).Methods(http.MethodPost)
```

```go
type translateRequest struct {
	PostID     string `json:"postId"`
	Text       string `json:"text"`
	TargetLang string `json:"targetLang"`
}

func (p *Plugin) handleTranslate(w http.ResponseWriter, r *http.Request) {
	var req translateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	text := req.Text
	if req.PostID != "" && p.resolvePostText != nil {
		resolved, err := p.resolvePostText(req.PostID)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "Could not load that message.")
			return
		}
		text = resolved
	}
	if strings.TrimSpace(text) == "" {
		writeJSONError(w, http.StatusBadRequest, "Nothing to translate.")
		return
	}

	out, err := p.translator.Translate(r.Context(), text, req.TargetLang)
	if err != nil {
		status := http.StatusBadGateway
		switch {
		case errors.Is(err, translate.ErrNotConfigured), errors.Is(err, translate.ErrReloginRequired), errors.Is(err, translate.ErrQuota):
			status = http.StatusServiceUnavailable
		}
		writeJSONError(w, status, translate.UserMessage(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"translatedText": out})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
```

Add imports `encoding/json`, `errors`, `strings`, and the `translate` package. Add field `resolvePostText func(postID string) (string, error)` to `Plugin` in `plugin.go`, set in `OnActivate`:

```go
p.resolvePostText = func(postID string) (string, error) {
	post, err := p.client.Post.GetPost(postID)
	if err != nil {
		return "", err
	}
	return post.Message, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/ -run TestHandleTranslate -v` → PASS

- [ ] **Step 5: Commit**

```bash
git add server/api.go server/api_test.go server/plugin.go
git commit -m "feat: add POST /api/v1/translate endpoint"
```

---

## Task 11: Language preference endpoints

**Files:**
- Modify: `server/api.go`
- Modify: `server/plugin.go` (set `p.prefStore`)
- Create: `server/prefs.go`
- Test: `server/prefs_test.go`

**Interfaces:**
- Produces: `langPrefStore` with `Get(userID) (string, error)` and `Set(userID, lang string) error` (KV key `lang_pref-<userID>`, validates supported, falls back to config default on empty); routes `GET /api/v1/prefs/lang` → `{"lang":"EN"}` and `PUT /api/v1/prefs/lang` body `{"lang":"RU"}` → `200`.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeKV2 struct{ m map[string]string }

func (f *fakeKV2) Get(key string, out any) error {
	if p, ok := out.(*string); ok {
		*p = f.m[key]
	}
	return nil
}
func (f *fakeKV2) Set(key string, value any) (bool, error) {
	f.m[key] = value.(string)
	return true, nil
}

func TestLangPrefStore(t *testing.T) {
	kv := &fakeKV2{m: map[string]string{}}
	s := &langPrefStore{kv: kv, fallback: func() string { return "EN" }}

	got, err := s.Get("u1")
	require.NoError(t, err)
	assert.Equal(t, "EN", got) // fallback

	require.NoError(t, s.Set("u1", "ru"))
	got, err = s.Get("u1")
	require.NoError(t, err)
	assert.Equal(t, "RU", got)

	assert.Error(t, s.Set("u1", "zz")) // unsupported
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/ -run TestLangPrefStore -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

`server/prefs.go`:

```go
package main

import (
	"strings"

	"github.com/pkg/errors"

	"github.com/approveninja/mattermost-plugin-ai-translate/server/codexauth"
	"github.com/approveninja/mattermost-plugin-ai-translate/server/translate"
)

type langPrefStore struct {
	kv       codexauth.KV
	fallback func() string
}

func (s *langPrefStore) key(userID string) string { return "lang_pref-" + userID }

func (s *langPrefStore) Get(userID string) (string, error) {
	var lang string
	if err := s.kv.Get(s.key(userID), &lang); err != nil {
		return "", errors.Wrap(err, "failed to get language preference")
	}
	if !translate.IsSupported(lang) {
		return s.fallback(), nil
	}
	return strings.ToUpper(lang), nil
}

func (s *langPrefStore) Set(userID, lang string) error {
	up := strings.ToUpper(strings.TrimSpace(lang))
	if !translate.IsSupported(up) {
		return errors.Errorf("unsupported language %q", lang)
	}
	if _, err := s.kv.Set(s.key(userID), up); err != nil {
		return errors.Wrap(err, "failed to set language preference")
	}
	return nil
}
```

(The `langPrefStore.kv` field type `codexauth.KV` requires `Delete` too; `fakeKV2` in the test only implements `Get`/`Set` — add a no-op `Delete` to `fakeKV2`, or define a local 2-method interface in `prefs.go` instead of reusing `codexauth.KV`. Prefer a local interface `type kvGetSet interface { Get(string, any) error; Set(string, any) (bool, error) }`.)

Routes in `initRouter`:

```go
apiRouter.HandleFunc("/prefs/lang", p.handleGetLang).Methods(http.MethodGet)
apiRouter.HandleFunc("/prefs/lang", p.handleSetLang).Methods(http.MethodPut)
```

Handlers in `api.go`:

```go
func (p *Plugin) handleGetLang(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	lang, err := p.prefStore.Get(userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Could not load preference.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"lang": lang})
}

func (p *Plugin) handleSetLang(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	var body struct{ Lang string `json:"lang"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	if err := p.prefStore.Set(userID, body.Lang); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Unsupported language.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"lang": strings.ToUpper(body.Lang)})
}
```

Add `prefStore *langPrefStore` to `Plugin`; in `OnActivate`:

```go
p.prefStore = &langPrefStore{kv: &p.client.KV, fallback: func() string { return p.getConfiguration().defaultLanguage() }}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/ -run TestLangPrefStore -v` → PASS
Run: `go build ./server/...` → success

- [ ] **Step 5: Commit**

```bash
git add server/prefs.go server/prefs_test.go server/api.go server/plugin.go
git commit -m "feat: add per-user language preference endpoints"
```

---

## Task 12: Auth-status endpoint

**Files:**
- Modify: `server/api.go`
- Test: add to `server/api_test.go`

**Interfaces:**
- Produces: `GET /api/v1/auth/status` → `{"connected":true|false}`. Uses a `Plugin.authStatus func() bool` set in `OnActivate` to `func() bool { _, ok, _ := store.Load(); return ok }`. Never returns token material.

- [ ] **Step 1: Write the failing test**

```go
func TestHandleAuthStatus(t *testing.T) {
	p := &Plugin{authStatus: func() bool { return true }}
	p.router = p.initRouter()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	r.Header.Set("Mattermost-User-ID", "u1")
	p.ServeHTTP(nil, w, r)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"connected":true`)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/ -run TestHandleAuthStatus -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Route: `apiRouter.HandleFunc("/auth/status", p.handleAuthStatus).Methods(http.MethodGet)`

```go
func (p *Plugin) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	connected := false
	if p.authStatus != nil {
		connected = p.authStatus()
	}
	writeJSON(w, http.StatusOK, map[string]bool{"connected": connected})
}
```

Add `authStatus func() bool` to `Plugin`; set in `OnActivate` (capture the `store` built in Task 9):

```go
p.authStatus = func() bool { _, ok, _ := store.Load(); return ok }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/ -run TestHandleAuthStatus -v` → PASS

- [ ] **Step 5: Commit**

```bash
git add server/api.go server/api_test.go server/plugin.go
git commit -m "feat: add GET /api/v1/auth/status endpoint"
```

---

## Task 13: `/aitranslate` slash command

**Files:**
- Modify: `server/command/command.go`
- Modify: `server/plugin.go` (pass a login driver into the command handler)
- Test: `server/command/command_test.go`

**Interfaces:**
- Consumes: a `command.CodexAdmin` interface (`Status() bool`, `Logout() error`, `Login(ctx) (userCode, verificationURL string, wait func() error, err error)`); the existing `Command` interface gains nothing public — the handler switches on the `aitranslate` trigger.
- Produces: `/aitranslate login|status|logout` returning ephemeral responses. Admin-gating is enforced by the caller passing `isAdmin bool` into `Handle` via the args (see note).

> **Note:** The existing `command.Handle(args)` signature stays. Add the `aitranslate` command registration and a `case` in `Handle`. Inject `CodexAdmin` and an `isSysAdmin func(userID string) bool` into `NewCommandHandler`. Keep the `hello` sample or remove it — removing it is cleaner; if removed, update `command_test.go` accordingly. The `Login` flow runs `BeginDeviceLogin`, returns the code/URL immediately for the ephemeral reply, and spawns polling in a goroutine that DMs/logs the result (since slash-command responses are synchronous, the login completion is reported via `client.Post` to the admin or a follow-up — for v1, return the code+URL and instruct the admin to re-run `/aitranslate status` after authorizing).

- [ ] **Step 1: Write the failing test**

```go
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

func (f *fakeAdmin) Status() bool   { return f.connected }
func (f *fakeAdmin) Logout() error  { f.loggedOut = true; return nil }
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./server/command/ -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

Rewrite `server/command/command.go`:

```go
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

type Handler struct {
	client     *pluginapi.Client
	admin      CodexAdmin
	isSysAdmin func(userID string) bool
}

type Command interface {
	Handle(args *model.CommandArgs) (*model.CommandResponse, error)
}

const trigger = "aitranslate"

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
```

Implement `CodexAdmin` on the plugin side. Add `server/codexadmin.go`:

```go
package main

import (
	"context"
	"time"

	"github.com/approveninja/mattermost-plugin-ai-translate/server/codexauth"
)

type codexAdmin struct {
	auth  *codexauth.Authenticator
	store *codexauth.Store
}

func (c codexAdmin) Status() bool { _, ok, _ := c.store.Load(); return ok }
func (c codexAdmin) Logout() error { return c.store.Clear() }

func (c codexAdmin) Login(ctx context.Context) (string, string, func() error, error) {
	dl, err := c.auth.BeginDeviceLogin(ctx)
	if err != nil {
		return "", "", nil, err
	}
	wait := func() error {
		deadline := time.Now().Add(15 * time.Minute)
		for time.Now().Before(deadline) {
			time.Sleep(time.Duration(dl.Interval) * time.Second)
			done, err := c.auth.PollDeviceLogin(ctx, dl)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		}
		return context.DeadlineExceeded
	}
	return dl.UserCode, dl.VerificationURL, wait, nil
}
```

Update `NewCommandHandler` call in `OnActivate`:

```go
admin := codexAdmin{auth: p.authenticator, store: store}
isSysAdmin := func(userID string) bool {
	u, err := p.client.User.Get(userID)
	return err == nil && strings.Contains(u.Roles, "system_admin")
}
p.commandClient = command.NewCommandHandler(p.client, admin, isSysAdmin)
```

(Update `server/command/mocks/mock_commands.go` if the `Command` interface changed — it didn't gain methods, but its definition was edited; regenerate with the mockgen command in CLAUDE.md, or delete the unused mock if nothing references it. Verify `command_test.go` no longer references `executeHelloCommand`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./server/command/ -v` → PASS
Run: `go build ./server/...` → success

- [ ] **Step 5: Commit**

```bash
git add server/command/ server/codexadmin.go server/plugin.go
git commit -m "feat: add /aitranslate login|status|logout command"
```

---

## Task 14: Webapp — languages and API client

**Files:**
- Create: `webapp/src/languages.ts`
- Create: `webapp/src/client.ts`
- Test: `webapp/src/client.test.tsx`

**Interfaces:**
- Produces: `LANGUAGES: {code: string; name: string}[]`, `DEFAULT_LANGUAGE = 'EN'`; `Client` with `translate(postId, targetLang)`, `getLang()`, `setLang(lang)` calling the plugin API base `/plugins/com.approveninja.ai-translate/api/v1`.

- [ ] **Step 1: Write the failing test**

```tsx
import {Client} from './client';

describe('Client', () => {
    const base = '/plugins/com.approveninja.ai-translate/api/v1';

    afterEach(() => {
        (global.fetch as jest.Mock | undefined)?.mockReset?.();
    });

    it('posts to translate and returns text', async () => {
        global.fetch = jest.fn().mockResolvedValue({
            ok: true, json: async () => ({translatedText: 'Hello'}),
        }) as jest.Mock;
        const c = new Client();
        const out = await c.translate('post1', 'EN');
        expect(out).toBe('Hello');
        expect((global.fetch as jest.Mock).mock.calls[0][0]).toBe(`${base}/translate`);
    });

    it('throws the server error message', async () => {
        global.fetch = jest.fn().mockResolvedValue({
            ok: false, json: async () => ({error: 'Translation isn\'t set up yet.'}),
        }) as jest.Mock;
        const c = new Client();
        await expect(c.translate('p', 'EN')).rejects.toThrow('set up');
    });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd webapp && npx jest src/client.test.tsx`
Expected: FAIL.

- [ ] **Step 3: Implement**

`webapp/src/languages.ts`:

```ts
export const DEFAULT_LANGUAGE = 'EN';

export const LANGUAGES: Array<{code: string; name: string}> = [
    {code: 'EN', name: 'English'},
    {code: 'UA', name: 'Ukrainian'},
    {code: 'RU', name: 'Russian'},
    {code: 'PL', name: 'Polish'},
    {code: 'ES', name: 'Spanish'},
    {code: 'FR', name: 'French'},
    {code: 'DE', name: 'German'},
    {code: 'ZH', name: 'Chinese'},
    {code: 'IT', name: 'Italian'},
    {code: 'PT', name: 'Portuguese'},
    {code: 'JA', name: 'Japanese'},
    {code: 'TR', name: 'Turkish'},
];
```

`webapp/src/client.ts`:

```ts
const BASE = '/plugins/com.approveninja.ai-translate/api/v1';

export class Client {
    private async parse(res: Response): Promise<any> {
        const data = await res.json().catch(() => ({}));
        if (!res.ok) {
            throw new Error(data.error || 'Request failed');
        }
        return data;
    }

    async translate(postId: string, targetLang: string): Promise<string> {
        const res = await fetch(`${BASE}/translate`, {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({postId, targetLang}),
        });
        const data = await this.parse(res);
        return data.translatedText as string;
    }

    async getLang(): Promise<string> {
        const res = await fetch(`${BASE}/prefs/lang`);
        const data = await this.parse(res);
        return data.lang as string;
    }

    async setLang(lang: string): Promise<void> {
        const res = await fetch(`${BASE}/prefs/lang`, {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({lang}),
        });
        await this.parse(res);
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd webapp && npx jest src/client.test.tsx` → PASS

- [ ] **Step 5: Commit**

```bash
git add webapp/src/languages.ts webapp/src/client.ts webapp/src/client.test.tsx
git commit -m "feat: add webapp language list and API client"
```

---

## Task 15: Webapp — inline translate control + message formatting

**Files:**
- Create: `webapp/src/components/translate_control.tsx`
- Create: `webapp/src/translation_state.ts`
- Modify: `webapp/src/index.tsx`
- Test: `webapp/src/components/translate_control.test.tsx`

**Interfaces:**
- Consumes: `Client`, `LANGUAGES`, `DEFAULT_LANGUAGE`.
- Produces: `translationState` singleton (`get(postId)`, `set(postId, {lang, text})`, `toggle(postId)`, `subscribe(cb)`) holding per-post `{showing: 'original'|'translated', lang, text}` with an in-session cache; `TranslateControl` component rendering `[ → <LANG> ▾ ]`, calling `client.translate` on click, toggling, and persisting the chosen language via `client.setLang`.

> **Note:** Use the webapp mechanism confirmed in Task 0 Step 3. The plan assumes `registerMessageWillFormatHook((post, message) => translationState.displayText(post.id, message))` to swap text and a `TranslateControl` rendered via the chosen per-post mechanism. If only the post dropdown menu is available, register `registry.registerPostDropdownMenuAction` items per language that call the same `translationState`/`Client` logic; the toggle then operates on the formatted text only.

- [ ] **Step 1: Write the failing test**

```tsx
import React from 'react';
import {render, screen, fireEvent, waitFor} from '@testing-library/react';

import {TranslateControl} from './translate_control';
import {translationState} from '../translation_state';

jest.mock('../client', () => ({
    Client: jest.fn().mockImplementation(() => ({
        translate: jest.fn().mockResolvedValue('Hello'),
        setLang: jest.fn().mockResolvedValue(undefined),
        getLang: jest.fn().mockResolvedValue('EN'),
    })),
}));

describe('TranslateControl', () => {
    beforeEach(() => translationState.reset());

    it('translates and toggles on click', async () => {
        render(<TranslateControl postId='p1'/>);
        fireEvent.click(screen.getByText(/→ EN/));
        await waitFor(() => {
            expect(translationState.get('p1')?.text).toBe('Hello');
            expect(translationState.get('p1')?.showing).toBe('translated');
        });
    });
});
```

(Confirm `@testing-library/react` is available; if not, add it as a devDependency in this step's commit, or test `translation_state.ts` logic directly without rendering.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd webapp && npx jest src/components/translate_control.test.tsx`
Expected: FAIL.

- [ ] **Step 3: Implement**

`webapp/src/translation_state.ts`:

```ts
type Entry = {showing: 'original' | 'translated'; lang: string; text?: string};

class TranslationState {
    private entries = new Map<string, Entry>();
    private subs = new Set<() => void>();

    get(postId: string): Entry | undefined {
        return this.entries.get(postId);
    }
    set(postId: string, entry: Entry) {
        this.entries.set(postId, entry);
        this.subs.forEach((cb) => cb());
    }
    toggle(postId: string) {
        const e = this.entries.get(postId);
        if (!e) {
            return;
        }
        e.showing = e.showing === 'translated' ? 'original' : 'translated';
        this.set(postId, e);
    }
    displayText(postId: string, original: string): string {
        const e = this.entries.get(postId);
        if (e && e.showing === 'translated' && e.text) {
            return e.text;
        }
        return original;
    }
    subscribe(cb: () => void): () => void {
        this.subs.add(cb);
        return () => this.subs.delete(cb);
    }
    reset() {
        this.entries.clear();
    }
}

export const translationState = new TranslationState();
```

`webapp/src/components/translate_control.tsx`:

```tsx
import React, {useState} from 'react';

import {Client} from '../client';
import {LANGUAGES, DEFAULT_LANGUAGE} from '../languages';
import {translationState} from '../translation_state';

const client = new Client();

export function TranslateControl({postId}: {postId: string}) {
    const existing = translationState.get(postId);
    const [lang, setLang] = useState(existing?.lang || DEFAULT_LANGUAGE);
    const [busy, setBusy] = useState(false);
    const [menuOpen, setMenuOpen] = useState(false);

    const showing = translationState.get(postId)?.showing || 'original';

    const doTranslate = async (targetLang: string) => {
        setBusy(true);
        try {
            const cached = translationState.get(postId);
            let text = cached?.text;
            if (!text || cached?.lang !== targetLang) {
                text = await client.translate(postId, targetLang);
                await client.setLang(targetLang);
            }
            translationState.set(postId, {showing: 'translated', lang: targetLang, text});
            setLang(targetLang);
        } finally {
            setBusy(false);
        }
    };

    const onMainClick = () => {
        if (showing === 'translated') {
            translationState.toggle(postId);
        } else {
            doTranslate(lang);
        }
    };

    const label = showing === 'translated' ? '↩ original' : `→ ${lang}`;

    return (
        <span className='ai-translate-control'>
            <button disabled={busy} onClick={onMainClick}>{busy ? '…' : label}</button>
            <button onClick={() => setMenuOpen((o) => !o)}>{'▾'}</button>
            {menuOpen && (
                <ul className='ai-translate-menu'>
                    {LANGUAGES.map((l) => (
                        <li key={l.code}>
                            <button onClick={() => {
                                setMenuOpen(false);
                                doTranslate(l.code);
                            }}>{l.name}</button>
                        </li>
                    ))}
                </ul>
            )}
        </span>
    );
}
```

`webapp/src/index.tsx` — register inside `initialize`:

```tsx
import {translationState} from './translation_state';
// ...
registry.registerMessageWillFormatHook((post: any, message: string) =>
    translationState.displayText(post.id, message));
// Render TranslateControl per post via the mechanism confirmed in Task 0 Step 3.
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd webapp && npx jest src/components/translate_control.test.tsx` → PASS

- [ ] **Step 5: Commit**

```bash
git add webapp/src/translation_state.ts webapp/src/components/translate_control.tsx webapp/src/index.tsx webapp/src/components/translate_control.test.tsx webapp/package.json webapp/package-lock.json
git commit -m "feat: add inline translate control and message formatting hook"
```

---

## Task 16: End-to-end build, lint, and manual verification

**Files:** none (verification + docs)

- [ ] **Step 1: Full build and tests**

Run: `go test ./server/...` → all PASS
Run: `cd webapp && npm test` → all PASS
Run: `make check-style` → no errors
Run: `make dist` → bundle builds

- [ ] **Step 2: Manual verification on a dev server**

Run: `make deploy` (local MM). In System Console set the model/default language. Run `/aitranslate login` as a sysadmin, complete the device login, then `/aitranslate status` → connected. Post a non-English message, click `[ → EN ▾ ]`, confirm it toggles to the translation and back, and that picking another language re-translates and is remembered after reload.

- [ ] **Step 3: Update README**

Add a short "Usage" section to `README.md`: admin connects with `/aitranslate login`; users translate via the per-post control; note the single-account/ToS caveat from the spec §9.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: document AI Translate usage and setup"
```

---

## Self-review notes

- **Spec coverage:** provider interface (T2), codex client (T3), device-code login (T7), refresh+rotation+lock (T5,T6), translate endpoint (T10), lang prefs (T11), auth status (T12), command (T13), config (T8), webapp control+toggle+memory (T14–T15), privacy/tests throughout, spikes (T0). All spec §2 decisions and §11 spikes are represented.
- **Open implementation risks flagged inline:** exact `/responses` body shape (T3 note tied to T0), `pluginapi` KV/`cluster.Mutex` signatures (T9 note), webapp injection mechanism (T0 Step 3 → T15 note), `@testing-library/react` availability (T15 note).
- **Type consistency:** `codexauth.KV` (T4) reused by store/auth; `codex.TokenSource` (T3) satisfied by `*codexauth.Authenticator.AccessToken` (T6); `NewAuthenticator` signature finalized in T7 (4th arg `authBaseURL`) — T6 tests pass `srv.URL` accordingly.
