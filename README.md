# AI Translate (Mattermost plugin)

[![Build Status](https://github.com/approveninja/mattermost-plugin-ai-translate/actions/workflows/ci.yml/badge.svg)](https://github.com/approveninja/mattermost-plugin-ai-translate/actions/workflows/ci.yml)

AI-powered translation of Mattermost messages.

> **Status: skeleton.** This repository currently contains only the
> [Mattermost plugin starter template](https://github.com/mattermost/mattermost-plugin-starter-template)
> scaffolding with project identity applied. No translation functionality is
> implemented yet — that will be designed and built in a follow-up.

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

## License

Apache-2.0. See [LICENSE](LICENSE).
