# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

Go CLI that collects Spotify listening history and generates Obsidian markdown notes (weekly summaries, daily notes, artist stubs, and a "Music Taste" persona context pack). Zero external Go dependencies — stdlib only.

## Build & Test

```sh
go build -o spotify-garden .           # build binary
go vet ./...                           # static checks (used in CI)
go test ./...                          # all tests
go test ./internal/render/ -run TestWeekStr  # single test
```

Pre-commit gate (CI mirrors this): `go vet ./... && go test ./... && go build -o spotify-garden .`

Version injection on release: `go build -ldflags "-X main.version=vX.Y.Z" -o spotify-garden .`

## Architecture

**main.go** — CLI dispatch, `.env` loading, runtime path resolution. Commands: `auth`, `collect`, `daily`, `weekly`, `catch-up`, `persona`, `setlist`, `doctor`, `version`.

**internal/** packages (each small, single-responsibility):
- `auth` — OAuth2 authorization code flow, token save/load/refresh. Local HTTP callback server on port 8888 or manual paste for external redirect URIs.
- `client` — Authenticated HTTP GET with exponential backoff on 429 (1s → 2s → 4s → fail). Bearer token, 30s timeout.
- `fetch` — Spotify API → internal models. Silently filters podcast episodes.
- `models` — Data structs: `Play`, `TopTrack`, `TopArtist`, `Setlist`.
- `plays` — `plays.json` load/save/merge. Deduplicates by `played_at` key, sorts descending.
- `render` — Weekly/daily note generation, artist stub creation (never overwrites existing), persona template rendering. ISO week math lives here (`WeekBounds`, `WeekStr`).

**templates/** — Go `text/template` files for persona and weekly reference. Template dir resolved: `SPOTIFY_TEMPLATES_DIR` env → `./templates` → relative to executable.

## Runtime Path Resolution

Precedence: CLI flags → env vars → `SPOTIFY_STATE_DIR` subdirectories → CWD fallback (with warning). This applies to `.env`, `tokens.json`, and `data/plays.json`. See `resolveRuntimePaths()` in main.go.

## Testing Patterns

- Stdlib `testing` only. Name pattern: `Test<Function>_<scenario>`.
- Date/time tests use `localNoon(year, month, day)` helper to avoid UTC→local day-shift bugs.
- File I/O tests use `t.TempDir()` + `t.Setenv()` for isolated vault simulation.
- Always test ISO week edge cases (Mon/Sun boundaries) when touching date logic.

## Conventions

- Errors: always wrap with context — `fmt.Errorf("context: %w", err)`.
- Business logic belongs in `internal/`, not `main.go`.
- `gofmt` before committing.
- Commits: concise, imperative, scoped.
- Major features: write a plan under `docs/plans/` with a dated filename before or alongside implementation. Bump minor version on completion.

## Environment Variables

Defined in `.env` (git-ignored); see `.env.example` for required keys:
- `SPOTIFY_CLIENT_ID`, `SPOTIFY_CLIENT_SECRET`, `SPOTIFY_REDIRECT_URI`
- `OBSIDIAN_VAULT_PATH` — target vault root
- `SPOTIFY_TEMPLATES_DIR` — override template location
- `SPOTIFY_STATE_DIR` — preferred location for tokens/data
- `SPOTIFY_AUTO_DAILY_ON_COLLECT` — when truthy, `collect` also regenerates today's daily note
- `SETLISTFM_API_KEY` — for `setlist` command
