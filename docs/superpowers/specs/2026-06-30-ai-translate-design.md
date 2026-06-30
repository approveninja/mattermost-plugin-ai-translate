# AI Translate — Design Spec (v1)

- **Status:** approved design, pre-implementation
- **Date:** 2026-06-30
- **Plugin:** `com.approveninja.ai-translate` (AI Translate)
- **Scope:** first shippable version (v1)

## 1. Summary

Add per-message AI translation to Mattermost. Each post gets an inline control
`[ → EN ▾ ]`. Clicking it translates the post into the user's currently selected
target language and replaces the displayed text in place (toggle back to the
original). A dropdown lets the user pick a different target language; the last
chosen language is remembered per user.

v1 uses a single translation backend: **OpenAI Codex via OAuth** (the ChatGPT
device-code flow used by Codex CLI), with the **admin authenticating once** and
the whole server translating through that one account. The provider layer is a
thin interface so additional providers (OpenAI API key, Anthropic, GLM) can be
added later without rework.

## 2. Decisions (locked)

| Topic | Decision |
| --- | --- |
| Backend (v1) | OpenAI Codex via OAuth, **risks explicitly accepted** (see §9) |
| Provider abstraction | Thin `Translator` interface, one impl (`codex`) in v1 |
| Auth model | Admin logs in once; tokens stored server-side (KV), shared by all users |
| OAuth flow | Device-code (headless), endpoints confirmed from `hermes-agent` |
| Config scope | Global admin config (one provider/model); users pick only language |
| Key/token storage | Server-side only, never sent to clients |
| UX | Inline control under/by each post; toggle replaces text in place |
| Language list | Hardcoded: EN, UA, RU, PL, ES, FR, DE, ZH, IT, PT, JA, TR; default EN |
| Per-user memory | Last target language stored per user in KV (works across devices) |
| Translation cache | None server-side; webapp keeps in-session results for instant toggle |

## 3. Reference: confirmed Codex contract

Extracted from `NousResearch/hermes-agent` (`hermes_cli/auth.py`,
`agent/codex_runtime.py`, `agent/transports/codex.py`). This removes the
"undocumented/reverse-engineered" uncertainty — the flow is known and working.

**Constants**
- `client_id = app_EMoamEEZ73f0CkXaXp7hrann`
- OAuth token URL: `https://auth.openai.com/oauth/token`
- API base URL: `https://chatgpt.com/backend-api/codex`
- Default model: `gpt-5.5`

**Device-code login**
1. `POST https://auth.openai.com/api/accounts/deviceauth/usercode`
   body JSON `{client_id}` → `{user_code, device_auth_id, interval}`
2. User opens `https://auth.openai.com/codex/device`, enters `user_code`.
3. Poll `POST https://auth.openai.com/api/accounts/deviceauth/token`
   body JSON `{device_auth_id, user_code}`:
   - `200` → `{authorization_code, code_verifier}`
   - `403`/`404` → still pending, keep polling at `interval` (min 3s)
4. Exchange `POST https://auth.openai.com/oauth/token` (form-urlencoded):
   `grant_type=authorization_code, code, redirect_uri=https://auth.openai.com/deviceauth/callback, client_id, code_verifier`
   → `{access_token, refresh_token}`

**Refresh** `POST https://auth.openai.com/oauth/token` (form-urlencoded):
`grant_type=refresh_token, refresh_token, client_id`
→ new `access_token`, and usually a **rotated `refresh_token`**.
⚠️ The refresh token is **single-use** — the new one MUST be persisted.

**Error classification on token endpoint**
- `429` → quota exhausted; credentials still valid → "retry later" (do NOT relogin)
- `invalid_grant` / `invalid_token` / `refresh_token_reused` / `401` / `403`
  → relogin required (admin must re-run login)

**Model call** OpenAI Responses API shape against `…/codex/responses`:
- Header `Authorization: Bearer <access_token>`
- Body `{model, instructions, input, store: false, stream: false}`
- No `chatgpt-account-id` header needed (hermes stores only access+refresh).
- Spike will confirm whether `originator` / `User-Agent` headers are required.

## 4. Architecture

```
Webapp (React/TS)                         Server (Go plugin)
┌────────────────────────┐                ┌─────────────────────────────────┐
│ Post inline control     │  POST          │ /api/v1/translate               │
│ [ → EN ▾ ] + toggle     │ ───translate──▶│  → Translator.Translate()       │
│ messageWillFormat hook  │                │     → codex.Client (/responses) │
│ language dropdown       │  GET/PUT       │ /api/v1/prefs/lang  (KV per user)│
│ in-session result cache │ ───lang pref──▶│ /api/v1/auth/status             │
└────────────────────────┘                │                                 │
                                           │ codexauth: device-code login,   │
  Admin (slash command)                    │   token store (KV), refresh     │
  /aitranslate login|status|logout  ──────▶│   (cluster-locked, rotation)    │
                                           │ plugin.json settings: model,    │
                                           │   default language              │
                                           └─────────────────────────────────┘
```

## 5. Server components

All paths under `server/`.

### 5.1 `translate/translator.go`
```go
type Translator interface {
    Translate(ctx context.Context, text, targetLang string) (string, error)
}
```
- Plus typed errors so the API/command layer can map to user messages:
  `ErrNotConfigured`, `ErrReloginRequired`, `ErrQuota`, `ErrUpstream`.

### 5.2 `translate/codex/`
- HTTP client to `https://chatgpt.com/backend-api/codex/responses`.
- Prompt: an `instructions` string ("You are a translation engine. Translate the
  user's message into {languageName}. Output only the translation, preserving
  Markdown, mentions, emoji, and code spans.") and `input` = the post text.
- Model from config (default `gpt-5.5`), `store: false`.
- Obtains a valid access token from `codexauth` (refreshing if needed).
- Maps upstream `401`/quota/relogin responses to the typed errors above.

### 5.3 `codexauth/`
- `Login` orchestration of the device-code flow (driven by the slash command);
  returns `{user_code, verification_url}` to display, then polls to completion.
- Token store in KV (single global record; key e.g. `codex_oauth_tokens`).
- `AccessToken(ctx)` returns a fresh access token, refreshing when it expires
  within a skew window (~5 min), persisting the rotated refresh token.
- **Refresh under a cluster-wide lock** (`pluginapi/cluster` mutex) with
  re-read from KV after acquiring the lock, to avoid two nodes/requests
  consuming the single-use refresh token concurrently (see §7).
- Error classification per §3.

### 5.4 `api.go` (extends the existing router)
- `POST /api/v1/translate` — body `{postId?, text, targetLang}` →
  `{translatedText}`. Requires `Mattermost-User-ID` (existing middleware).
  Resolves text from `postId` server-side when provided (avoids trusting client
  text and respects channel access); falls back to `text` otherwise.
- `GET /api/v1/prefs/lang` / `PUT /api/v1/prefs/lang` — per-user last language
  (KV key per userID).
- `GET /api/v1/auth/status` — whether Codex is connected (for UI affordance /
  admin feedback); never returns token material.

### 5.5 `command/`
- `/aitranslate login` (admin only) — starts device-code login, posts an
  ephemeral message with the code + `auth.openai.com/codex/device` URL, polls,
  reports success/failure.
- `/aitranslate status` — connected? token age? (no secrets).
- `/aitranslate logout` — clears stored tokens.
- Admin gating via `client.User`/system-role check.

### 5.6 Configuration (`plugin.json` `settings_schema` + `configuration.go`)
- `Model` (text, default `gpt-5.5`).
- `DefaultLanguage` (dropdown of the supported list, default `EN`).
- (Tokens are NOT plugin settings — they live in KV, set via the login flow.)

## 6. Webapp components

All under `webapp/src/`.

- **Inline post control**: render `[ → EN ▾ ]` for each post. Mechanism to be
  confirmed by spike (§8c): `registerMessageWillFormatHook` to swap the rendered
  message text when a post is toggled, plus a small control to trigger translate
  and pick language. Fallback if the hook can't host the control: a post
  dropdown menu action ("Translate ▸ language") that flips the same state.
- **Toggle state**: per-post, in component/Redux state — `original` vs
  `translated`; clicking toggles. In-session cache keyed by `postId+lang` so
  re-toggling is instant and doesn't re-call the API.
- **Language dropdown**: hardcoded list (EN, UA, RU, PL, ES, FR, DE, ZH, IT, PT,
  JA, TR). Selecting a language sets it as current and persists via
  `PUT /api/v1/prefs/lang`; the label reflects the current target (`→ EN`).
- **API client**: thin wrapper over the three endpoints; handles error → toast.

## 7. Data & control flow

**Translate click**
1. Webapp `POST /api/v1/translate {postId, targetLang}`.
2. Server resolves post text, gets access token (refresh if expiring), calls
   Codex `/responses`.
3. Returns `{translatedText}`; webapp replaces displayed text and caches it.

**Refresh-token race (critical)**
- Single-use refresh token + concurrent translate requests ⇒ risk of one request
  invalidating the token mid-flight.
- Mitigation: `codexauth.AccessToken` refreshes inside a cluster mutex; after
  acquiring the lock it re-reads the KV record (another holder may have already
  refreshed) and only calls the token endpoint if still expiring. The rotated
  refresh token is written back under the same lock.

**Admin login**
- `/aitranslate login` → device-code → ephemeral code+URL → poll → store tokens.

## 8. Error handling (user-facing, ephemeral)

- Not configured / never logged in → "Translation isn't set up yet. An admin
  must connect Codex with `/aitranslate login`."
- Relogin required → "Codex sign-in expired. An admin must reconnect with
  `/aitranslate login`."
- Quota (`429`) → "Translation is rate-limited right now — try again shortly."
- Upstream/unknown → "Translation failed, please try again."

## 9. Privacy, security, risk

- **Accepted risk (Codex-via-OAuth):** the whole team translates through one
  admin ChatGPT account against the ChatGPT backend. This carries ToS/account-
  flag risk, shared rate limits, and dependence on a non-public endpoint. The
  user explicitly accepted these for v1. The thin provider interface keeps a
  documented API-key path open as a future fallback.
- **Token storage:** Mattermost KV is not encrypted at rest. Tokens are stored
  server-side only and never sent to clients. Consider encrypting the token
  record with a plugin-held secret (at minimum, document the exposure).
- **Content egress:** message text leaves the server to ChatGPT **only on an
  explicit user click**, never automatically.

## 10. Testing

- **Server unit tests:** prompt construction; Responses parsing; refresh +
  rotation + lock behavior; error classification (429 vs invalid_grant vs 401);
  command handler (admin gating, login/status/logout); `/api/v1/translate`
  resolves post text and enforces auth. HTTP calls mocked.
- **Webapp (jest):** toggle original↔translated; language selection + persistence
  call; in-session cache prevents re-calls; error → toast.

## 11. Spikes to run before/early in implementation

1. **Headless refresh** with the public `client_id` returns a new access token
   and a rotated refresh token (linchpin of the whole auth model).
2. **Live `/responses` call** with a Bearer access token returns a translation;
   confirm required headers (`originator` / `User-Agent`) and the model id.
3. **Webapp injection**: `registerMessageWillFormatHook` (current MM webapp) can
   render the inline control and swap displayed text; else use the post dropdown
   menu fallback.

## 12. Out of scope (v1)

- Additional providers (OpenAI API key, Anthropic, GLM) — interface left open.
- Per-user OAuth accounts; per-channel/team enablement; automatic translation.
- Server-side translation cache; usage metering/quotas; language auto-detection
  beyond what the model infers.
