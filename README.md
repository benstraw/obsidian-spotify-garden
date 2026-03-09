# spotify-garden

Pulls listening data from the [Spotify Web API](https://developer.spotify.com/documentation/web-api)
and renders structured Obsidian markdown notes — weekly summaries, daily play logs, artist stubs, and a
rolling Music Taste context pack for AI prompting. Also looks up concert setlists via
the [setlist.fm API](https://api.setlist.fm/docs/1.0/index.html).

Replaces the Python `collect_music.py` / `weekly_music_note.py` scripts with a proper
Go CLI that has real auth management, catch-up logic, and a 5x-daily collection schedule.

No external dependencies. stdlib only.

---

## Quick Start

### 1. Register a Spotify app

Go to [developer.spotify.com/dashboard](https://developer.spotify.com/dashboard), create
an app, and add `http://localhost:8888/callback` as a redirect URI.

### 2. Configure environment

```bash
cp .env.example .env
```

```
SPOTIFY_CLIENT_ID=your_client_id
SPOTIFY_CLIENT_SECRET=your_client_secret
SPOTIFY_REDIRECT_URI=http://localhost:8888/callback
OBSIDIAN_VAULT_PATH=/path/to/your/vault
SPOTIFY_TEMPLATES_DIR=/absolute/path/to/templates
SETLISTFM_API_KEY=your_setlistfm_api_key    # optional — only needed for setlist command
```

Get a setlist.fm API key at [setlist.fm/settings/apps](https://www.setlist.fm/settings/apps).

### 3. Build

```bash
go build -o spotify-garden .
```

### 4. Authenticate

```bash
./spotify-garden auth
```

Opens a browser to Spotify's OAuth page. Tokens are saved to the effective
`tokens.json` path (state dir when configured) and
auto-refresh — you should only need to do this once.

### 5. Collect and generate

```bash
./spotify-garden collect                                   # fetch last 50 recently-played
./spotify-garden weekly                                    # this week's note
./spotify-garden weekly --date 2026-02-10                  # specific week
./spotify-garden daily                                     # today's daily note
./spotify-garden daily --date 2026-02-21                   # specific day
./spotify-garden catch-up --weeks 8                        # backfill missing notes
./spotify-garden persona                                   # regenerate Music Taste context pack
./spotify-garden setlist "Jason Isbell"                    # look up today's setlist
./spotify-garden setlist "Jason Isbell" --date 2026-02-21  # specific date
./spotify-garden doctor                                    # print runtime config + diagnostics
```

---

## Build

```bash
go build -o spotify-garden .    # compile binary
go vet ./...                    # static analysis
```

---

## Output

Runtime paths resolve with precedence: flags > env vars > `SPOTIFY_STATE_DIR` > CWD fallback.
Files are written to `$OBSIDIAN_VAULT_PATH/music/` when the vault path is set.

| Command | Output path |
|---|---|
| `collect` | `{state}/data/plays/YYYY/YYYY-WNN.json` (sharded weekly files) |
| `weekly` | `{vault}/music/listening/spotify-YYYY-Www.md` |
| `daily` | `{vault}/music/listening/spotify-YYYY-MM-DD.md` |
| `daily`/`weekly` (artist stubs) | `{vault}/music/artists/{Artist Name}.md` |
| `persona` | `{vault}/01-ai-brain/context-packs/Music Taste.md` |
| `setlist` | stdout only — no vault writes |

---

## Automation (launchd)

Recommended (stable local install, avoids symlinked/external-drive path issues):

```bash
./scripts/install_launchd_local.sh
```

This installs/updates:
- binary: `~/.local/bin/spotify-garden`
- state: `~/Library/Application Support/spotify-garden/state` (`.env`, `tokens.json`, `data/plays/`, `data/genres.json`)
- templates: `~/Library/Application Support/spotify-garden/templates`
- logs: `~/Library/Application Support/spotify-garden/logs`
- launch agents: `~/Library/LaunchAgents/com.$USER.spotify-collect.plist` and `...spotify-weekly.plist`
- collect wrapper exports `SPOTIFY_AUTO_DAILY_ON_COLLECT=1` so today's daily note auto-refreshes on each collect run

Upgrade path (after code changes or `git pull`): re-run:

```bash
./scripts/install_launchd_local.sh
```

Legacy/manual method:

Copy the example plists, edit the path and label, then install:

```bash
# 1. Copy examples
cp spotify-collect.plist.example com.yourname.spotify-collect.plist
cp spotify-weekly.plist.example com.yourname.spotify-weekly.plist

# 2. Edit each plist: set Label and the path to the shell script
#    Label:            com.yourname.spotify-collect
#    ProgramArguments: /absolute/path/to/obsidian-spotify-garden/run_collect_spotify.sh

# 3. Install and load
cp com.yourname.spotify-collect.plist ~/Library/LaunchAgents/
cp com.yourname.spotify-weekly.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.yourname.spotify-collect.plist
launchctl load ~/Library/LaunchAgents/com.yourname.spotify-weekly.plist
```

| Job | Schedule |
|---|---|
| `spotify-collect` | 5× daily at 7, 11, 15, 19, 23h |
| `spotify-weekly` | Sundays at 23:00 (catch-up + weekly + persona) |

Logs go to `/tmp/spotify-collect.log` and `/tmp/spotify-weekly.log`.

Run `./spotify-garden doctor` to confirm effective paths, launchd labels, and log locations.

---

## Cloud Collection (GitHub Actions)

A workflow in `.github/workflows/collect.yml` runs `collect` 5×/day on GitHub's
servers, so collection keeps working even when the Mac is off.

### Setup

1. Run `auth` locally once to get `tokens.json` (interactive OAuth — only needed once).

2. Add three secrets in **Settings → Secrets and variables → Actions**:

   | Secret | Value |
   |---|---|
   | `SPOTIFY_CLIENT_ID` | from your `.env` |
   | `SPOTIFY_CLIENT_SECRET` | from your `.env` |
   | `SPOTIFY_TOKENS_JSON` | `base64 < tokens.json` (copy the output) |

3. The workflow commits `data/plays/` (sharded weekly files) and `data/genres.json` directly to `main` after each run.

### How it works

- On the first run, `tokens.json` is decoded from the `SPOTIFY_TOKENS_JSON` secret.
- Subsequent runs restore `tokens.json` from the GitHub Actions cache (the token refreshes automatically).
- If the Spotify refresh token expires (rare), re-run `./spotify-garden auth` locally and update the `SPOTIFY_TOKENS_JSON` secret with a fresh `base64 < tokens.json`.

> **TODO — plays sync strategy.**
> Cloud collect commits `data/plays/` to the repo; local launchd writes to
> `~/Library/Application Support/spotify-garden/state/data/plays/`. These two
> directories will diverge. Options under consideration:
> 1. **Cloud only** — disable launchd collect, single source of truth in the repo.
> 2. **Add a `sync` command** — merge repo and local `data/plays/` on demand.
> 3. **Both with auto-merge** — wrapper does `git pull` / merge / `git push` around collect.

### Manual trigger

Go to **Actions → Collect → Run workflow** in the GitHub UI.

---

## Documentation

| Doc | Contents |
|---|---|
| [docs/commands.md](docs/commands.md) | All commands, flags, and behaviour details |
| [docs/architecture.md](docs/architecture.md) | Package map, data flow, design decisions |
| [docs/auth-flow.md](docs/auth-flow.md) | OAuth flow, token storage, refresh, troubleshooting |

---

## Notes

- `tokens.json` and `.env` are gitignored — never commit them
- `data/plays/` (sharded weekly files) is committed to the repo by the GitHub Actions workflow; `data/plays.json.bak` is created locally on first collect after upgrade (safe to delete)
- if `SPOTIFY_STATE_DIR` is set and files are missing there, the CLI falls back to CWD and prints warnings
- `catch-up` only writes missing notes (weekly + daily); `weekly` always writes (overwrites if exists)
- `daily` only writes when that date has play data and never overwrites an existing daily note
- when `SPOTIFY_AUTO_DAILY_ON_COLLECT=1`, each `collect` run updates today's daily note
- Artist stubs are never overwritten once created; stubs can be created by daily or weekly generation and include a Concerts Dataview section
- Port `8888` must be free when running `auth` with a localhost redirect URI
- `setlist` requires `SETLISTFM_API_KEY` — prints to stdout only, no vault writes
- Concert notes live in `{vault}/music/concerts/` and are created manually via the Templater template
