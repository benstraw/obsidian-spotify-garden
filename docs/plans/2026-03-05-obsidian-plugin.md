# Plan: Obsidian Spotify Garden Plugin

## Context

The current Go CLI collects Spotify listening data and generates Obsidian markdown notes. It works, but the architecture has a fundamental sync problem: a GitHub Actions collector writes to a repo, a local binary writes to a state dir, and notes go to the vault — three locations that never sync. Rather than patching this, we're rebuilding as a native Obsidian plugin where everything lives in the vault.

**Goal:** A single Obsidian plugin that collects listening history, caches it in the vault, and generates weekly/daily/artist/persona notes — all without external tooling.

## Architecture

```
obsidian-music-garden/          (new repo)
├── manifest.json
├── package.json
├── esbuild.config.mjs
├── src/
│   ├── main.ts                   — plugin lifecycle, commands, settings tab
│   ├── spotify-auth.ts           — PKCE OAuth2 flow (no client secret)
│   ├── spotify-api.ts            — API client: recently-played, top-tracks, top-artists, artist details
│   ├── collector.ts              — scheduled collection loop (registerInterval)
│   ├── data-store.ts             — read/write plays.json, genres.json in vault
│   ├── renderer.ts               — generate markdown: weekly, daily, artist stubs, persona
│   └── settings.ts               — plugin settings interface + defaults
├── styles.css
└── tsconfig.json
```

## Data Storage

All data lives in the vault at `{vault}/.music-garden/`:

```
.music-garden/
├── plays.json          — listening history (same format as Go CLI)
├── genres.json         — artist→genres cache (same format)
└── tokens.json         — OAuth tokens (PKCE, no client secret)
```

Using a dotfolder keeps it out of the way but visible and syncable. Tokens in the vault is acceptable because PKCE tokens don't contain a client secret — they're user-scoped and revocable.

## OAuth Flow (PKCE)

1. User enters their Spotify Client ID in plugin settings (they create a free Spotify app at developer.spotify.com)
2. User clicks "Connect Spotify" → opens browser to Spotify auth URL with PKCE challenge
3. Spotify redirects to a static callback page hosted on GitHub Pages (free, we control it)
4. Callback page extracts the auth code and redirects to `obsidian://music-garden?code=...`
5. Plugin receives the code via `registerObsidianProtocolHandler`, exchanges it for tokens
6. Tokens saved to `.music-garden/tokens.json`, auto-refreshed on expiry

Works on both desktop and mobile Obsidian.

## Collection

- `registerInterval` runs every 15 minutes while Obsidian is open
- Calls `/v1/me/player/recently-played?limit=50`
- Merges into plays.json (dedup by `played_at`, same as Go CLI)
- Fetches genres for any new artists, updates genres.json
- Spotify keeps last 50 tracks — 15-min interval is conservative enough to never lose data during active listening

## Note Rendering

Same note types as the Go CLI, triggered via commands and/or on collection:

| Note | Path | Trigger |
|------|------|---------|
| Weekly | `music/listening/spotify-YYYY-Www.md` | Command, or auto on Sunday |
| Daily | `music/listening/spotify-YYYY-MM-DD.md` | Command, or auto on collect |
| Artist stubs | `music/artists/{Name}.md` | Auto on collect (never overwrite) |
| Persona | `01-ai-brain/context-packs/Music Taste.md` | Command |

Note paths configurable in settings. Markdown format identical to current Go output.

## Settings

- Spotify Client ID (required)
- Collection interval (default: 15 min)
- Auto-generate daily notes on collect (toggle)
- Auto-generate weekly notes on Sunday (toggle)
- Note output paths (weekly, daily, artist, persona)
- Genres in weekly notes (toggle)
- Genres in daily notes (toggle)

## Commands (Obsidian command palette)

- `Spotify Garden: Collect now` — manual collect
- `Spotify Garden: Generate weekly note` — current week (or prompt for date)
- `Spotify Garden: Generate daily note` — today (or prompt for date)
- `Spotify Garden: Catch up` — generate missing notes for last N weeks
- `Spotify Garden: Regenerate persona` — rebuild Music Taste context pack
- `Spotify Garden: Genre backfill` — fetch genres for all uncached artists
- `Spotify Garden: Connect Spotify` — start OAuth flow
- `Spotify Garden: Disconnect Spotify` — clear tokens

## GitHub Actions / External Data Import

**TODO: Revisit this decision.** The plugin collects when Obsidian is open, but for 24/7 coverage a GitHub Actions collector (already built in the Go CLI repo) fills the gaps. The plugin needs a way to import/merge externally-collected plays.json and genres.json — either via GitHub API fetch, local file path import, or both. Decide this before or during implementation.

## Migration from Go CLI

The JSON formats (plays.json, genres.json) are identical. Users can copy their existing data files into `{vault}/.music-garden/` and the plugin picks them up immediately.

## Implementation Order

1. **Scaffold** — repo, package.json, esbuild, manifest.json, tsconfig
2. **Settings + data store** — settings tab, read/write JSON in vault
3. **OAuth** — PKCE flow + static callback page + protocol handler
4. **API client** — recently-played, artists batch, top tracks/artists
5. **Collector** — scheduled loop, dedup/merge logic
6. **Renderer** — daily notes first (simplest), then weekly, artist stubs, persona
7. **Commands** — register all palette commands
8. **Polish** — status bar indicator, notices, error handling

## Verification

1. Install plugin in dev mode (`ln -s` into vault's `.obsidian/plugins/`)
2. Configure Spotify Client ID, run OAuth flow
3. Trigger manual collect — verify plays.json appears in `.music-garden/`
4. Let auto-collect run — verify new plays merge correctly
5. Generate daily/weekly notes — verify markdown matches Go CLI output
6. Copy existing plays.json from Go CLI — verify plugin reads it correctly
7. Test on Obsidian mobile (OAuth redirect via obsidian:// protocol)
