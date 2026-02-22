# spotify-garden

Pulls listening data from the [Spotify Web API](https://developer.spotify.com/documentation/web-api)
and renders structured Obsidian markdown notes — weekly play logs, artist stubs, and a
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

Opens a browser to Spotify's OAuth page. Tokens are saved to `tokens.json` and
auto-refresh — you should only need to do this once.

### 5. Collect and generate

```bash
./spotify-garden collect                                   # fetch last 50 recently-played
./spotify-garden weekly                                    # this week's note
./spotify-garden weekly --date 2026-02-10                  # specific week
./spotify-garden catch-up --weeks 8                        # backfill missing notes
./spotify-garden persona                                   # regenerate Music Taste context pack
./spotify-garden setlist "Jason Isbell"                    # look up today's setlist
./spotify-garden setlist "Jason Isbell" --date 2026-02-21  # specific date
```

---

## Build

```bash
go build -o spotify-garden .    # compile binary
go vet ./...                    # static analysis
```

---

## Output

Files are written to `$OBSIDIAN_VAULT_PATH/music/` when the vault path is set.

| Command | Output path |
|---|---|
| `collect` | `data/plays.json` (local, git-ignored) |
| `weekly` | `{vault}/music/listening/spotify-YYYY-Www.md` |
| `weekly` (artist stubs) | `{vault}/music/artists/{Artist Name}.md` |
| `persona` | `{vault}/01-ai-brain/context-packs/Music Taste.md` |
| `setlist` | stdout only — no vault writes |

---

## Automation (launchd)

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

---

## Documentation

| Doc | Contents |
|---|---|
| [docs/commands.md](docs/commands.md) | All commands, flags, and behaviour details |
| [docs/architecture.md](docs/architecture.md) | Package map, data flow, design decisions |
| [docs/auth-flow.md](docs/auth-flow.md) | OAuth flow, token storage, refresh, troubleshooting |

---

## Notes

- `tokens.json`, `.env`, and `data/plays.json` are gitignored — never commit them
- `catch-up` only writes missing notes; `weekly` always writes (overwrites if exists)
- Artist stubs are never overwritten once created; new stubs include a Concerts Dataview section
- Port `8888` must be free when running `auth` with a localhost redirect URI
- `setlist` requires `SETLISTFM_API_KEY` — prints to stdout only, no vault writes
- Concert notes live in `{vault}/music/concerts/` and are created manually via the Templater template
