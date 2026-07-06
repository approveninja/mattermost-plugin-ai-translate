# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# mattermost-plugin-ai-translate

Mattermost server+web plugin for **AI-powered translation of messages**.

## Current status: SKELETON ONLY

This repository currently contains **only the scaffolding** from the official
[`mattermost/mattermost-plugin-starter-template`](https://github.com/mattermost/mattermost-plugin-starter-template),
with project identity applied. There is **no translation functionality yet** —
the server/webapp sample code is the template's canonical hello-world.

The translation feature is to be **designed first** (brainstorm → spec → plan)
in a follow-up session before any implementation. See "Planned work" below.

## Working conventions

(Same conventions as the sibling `mattermost-plugin-disallow-dm` project.)

- **All code and project documentation MUST be written in English** (source,
  comments, specs, README, commit messages, this file).
- **Reply to the user in the language they asked in** (e.g. Russian question →
  Russian answer). This applies only to conversational replies, not to
  committed artifacts.
- **ALWAYS ask EVERY question through the poll panel** (the interactive
  question panel / `/poll` — the `AskUserQuestion` tool), never as free-form
  prose. This applies to all questions without exception: clarifications,
  design choices, yes/no confirmations, "which approach" — all of them go
  through the poll, with multiple-choice options. If a question is truly
  open-ended, still pose it via the poll and let the user pick "Other".
- Before substantial creative work, brainstorm the design and get approval
  before writing code (this project follows the superpowers workflow).

## Plugin identity

- **ID:** `com.approveninja.ai-translate`
- **Name:** AI Translate
- **Version:** `0.1.0`
- **Go module:** `github.com/approveninja/mattermost-plugin-ai-translate`
- **GitHub:** https://github.com/approveninja/mattermost-plugin-ai-translate
  (public, default branch `main`)
- **Min server version:** 6.2.1 (template default)

## Workspace layout

This project lives at
`/home/alexeysilver/claudecode/mattermost-plugin-ai-translate`, a sibling of
`mattermost-plugin-disallow-dm` (a separate, already-shipped plugin in the same
`approveninja` org). The two are independent repos; do **not** modify
`disallow-dm` from here.

## Repository structure

Standard Mattermost plugin layout (from the starter template):

- `server/` — Go server code (plugin hooks, slash command sample, KV store
  sample, job sample). Entry: `server/plugin.go`.
- `webapp/` — React/TypeScript web app (webpack build, jest tests).
- `public/` — static assets served by the plugin.
- `assets/` — plugin icon, etc.
- `plugin.json` — plugin manifest (id, settings_schema, bundle paths).
- `Makefile` — build/lint/deploy targets (Mattermost standard).
- `build/` — packaging helpers (`pluginctl`, etc.).

## Architecture (server)

The whole plugin is wired together in `Plugin.OnActivate` (`server/plugin.go`),
which is the place to start reading. The `Plugin` struct holds four
collaborators built there and used everywhere else:

- `client *pluginapi.Client` — the typed wrapper over `p.API`/`p.Driver`; the
  preferred way to call the server (KV, slash commands, logging, etc.).
- `kvstore` (`server/store/kvstore/`) — a small interface over the client's KV
  API. `NewKVStore(client)` is the only constructor; add persistence methods to
  the `KVStore` interface, not by touching `client.KV` directly.
- `commandClient` (`server/command/`) — slash-command registration + dispatch.
  Commands are **registered once** in `NewCommandHandler` and **dispatched** by
  `Handler.Handle`, which the `Plugin.ExecuteCommand` hook delegates to. To add
  a command: register it in `NewCommandHandler`, add a `case` in `Handle`, and
  add it to the `Command` interface (then regenerate mocks — see above).
- `router *mux.Router` (`server/api.go`) — HTTP routes under
  `/plugins/com.approveninja.ai-translate/api/v1/`. All routes pass through the
  `MattermostAuthorizationRequired` middleware, which rejects requests lacking
  the `Mattermost-User-ID` header (set by the server for logged-in users).
  `ServeHTTP` just forwards to this router.

`backgroundJob` is a clustered periodic job (`cluster.Schedule`) — safe to run
in HA because only one node executes each tick; closed in `OnDeactivate`.

**Configuration pattern** (`server/configuration.go`): the active config is a
single `*configuration` guarded by an `RWMutex`. Never mutate it in place —
`OnConfigurationChange` builds a fresh struct via `LoadPluginConfiguration` and
swaps it with `setConfiguration`; readers call `getConfiguration`. Public fields
on the struct are auto-populated from the server config defined in
`plugin.json`'s `settings_schema`. If you add reference-type fields, update
`Clone` to deep-copy them.

The webapp entry (`webapp/src/index.tsx`) is a `Plugin` class whose
`initialize(registry, store)` is currently empty; it self-registers via
`window.registerPlugin(manifest.id, ...)`. `manifest` is code-generated from
`plugin.json` by `build/manifest` (shared by server and webapp), so plugin ID
and version have a single source of truth.

## Development commands

```sh
make check-style   # golangci-lint (server) + eslint (webapp)
make dist          # build the distributable plugin bundle
make deploy        # build and upload to a local Mattermost server
make test          # server + webapp tests
go build ./server/...   # quick server-only build
go test  ./server/...   # quick server-only tests
```

Run a single Go test (no Make/mocks required):

```sh
go test ./server/... -run TestName        # one test by name
go test ./server/command/ -run TestHandle -v
```

Webapp tests are Jest (run from `webapp/`):

```sh
cd webapp && npm test                       # all webapp tests
cd webapp && npx jest src/manifest.test.tsx # a single test file
```

`make test` runs both suites via `gotestsum` (it installs the tool and runs
`webapp` npm install first); for fast iteration use the raw `go test`/`npx jest`
commands above.

Toolchain: Go 1.25; webapp needs node (see `.nvmrc`) — run `npm install` in
`webapp/` before the first webapp build.

Mocks are generated with `mockgen`. After changing the `command.Command`
interface, regenerate `server/command/mocks/mock_commands.go` with the command
recorded at the top of that file:

```sh
mockgen -destination=server/command/mocks/mock_commands.go -package=mocks \
  github.com/approveninja/mattermost-plugin-ai-translate/server/command Command
```

## Verified baseline

As of the initial scaffold commit, `go build`, `go vet`, and
`go test ./server/...` all pass; `plugin.json` is valid. The webapp npm build
has **not** been run yet (no `node_modules` installed).

## Planned work (not yet designed — decide via brainstorm)

Open questions to resolve before implementing the translate feature:

- **Trigger/UX:** translate on a per-post button (post dropdown action) vs.
  automatic inline translation vs. a slash command? Target language: per-user
  preference, channel default, or chosen per action?
- **Provider/model:** which translation backend (e.g. an LLM via API such as
  Claude, or a dedicated translation API)? Where do API keys live (plugin
  settings vs. server config)?
- **Server vs. webapp split:** call the provider from the Go server (keeps keys
  server-side) and render results in the webapp.
- **Settings:** what goes in `plugin.json` `settings_schema` (API key, default
  language, enabled channels, rate limits).
- **Privacy/cost:** message content leaves the server to a third party —
  consider opt-in, redaction, and per-team enablement.

When starting that work: use the brainstorming skill first, write a spec to
`docs/superpowers/specs/`, then a plan, then implement.
