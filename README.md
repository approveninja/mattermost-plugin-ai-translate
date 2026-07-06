# AI Translate (Mattermost plugin)

[![Build Status](https://github.com/approveninja/mattermost-plugin-ai-translate/actions/workflows/ci.yml/badge.svg)](https://github.com/approveninja/mattermost-plugin-ai-translate/actions/workflows/ci.yml)

AI-powered translation of Mattermost messages.

## Plugin identity

- **ID:** `com.approveninja.ai-translate`
- **Name:** AI Translate
- **Module:** `github.com/approveninja/mattermost-plugin-ai-translate`

## Development

This plugin follows the standard Mattermost plugin layout (Go `server/` +
React/TypeScript `webapp/`). Common targets:

```sh
make check-style   # lint server and webapp
make dist          # build the distributable plugin bundle
make deploy        # build and upload to a local Mattermost server
```

See the [Mattermost plugin documentation](https://developers.mattermost.com/extend/plugins/)
for details on the toolchain and APIs.

## Usage

### Admin setup

1. **Connect a ChatGPT account (system admin only).**
   Run `/aitranslate login` in any channel. The plugin posts a device-code URL
   and a short code — open the URL in a browser, enter the code, and authorise
   with a ChatGPT account. Tokens are stored server-side.

2. **Verify the connection.**
   Run `/aitranslate status` — the reply shows the connected account and token
   expiry.

3. **Disconnect when needed.**
   Run `/aitranslate logout` to revoke and delete the stored tokens.

4. **Configure model and default language.**
   In **System Console → Plugins → AI Translate** choose the Codex model (e.g.
   `gpt-4o`) and the default target language (e.g. `English`).

### Per-user translation

Every message shows a compact control **`[ → EN ▾ ]`** (the arrow and the
current target language):

- **Click the label** to toggle between the original message and the
  translation.  The translation is fetched on first use and cached for the
  session.
- **Click the caret `▾`** to open a language picker.  Select any language to
  re-translate the post into that language.  Your last-used language is
  remembered across reloads (stored as a per-user preference).

### Caveat

v1 routes all translations through a single admin-owned ChatGPT account via the
Codex OAuth device-code flow.  Using one shared account for all users has
ToS, rate-limit, and privacy implications that are accepted trade-offs for this
version.  See the design spec
[`docs/superpowers/specs/2026-06-30-ai-translate-design.md`](docs/superpowers/specs/2026-06-30-ai-translate-design.md)
§9 for the full discussion.

## License

Apache-2.0. See [LICENSE](LICENSE).
