# Plan: Build obsidian-music-garden Go CLI

## Context

The existing Spotify integration is a loose Python project with two scripts (`collect_music.py`, `weekly_music_note.py`), no proper CLI structure, no catch-up logic, and a once-daily collection schedule that can miss plays when the 50-track API cap is hit or the laptop is asleep.

The goal is to build a proper Go CLI — `obsidian-music-garden` — that mirrors the architecture of the WHOOP CLI (`obsidian-whoop-garden`). It replaces the Python scripts entirely, gains real auth management, catch-up logic, and gets put on a 5x-daily collection schedule.

## Project Location

`/path/to/obsidian-music-garden/`

## Architecture (mirrors obsidian-whoop-garden)

```
obsidian-music-garden/
├── go.mod                                  # module github.com/yourname/music-garden
├── main.go                                 # command dispatch
├── internal/
│   ├── auth/auth.go                        # OAuth2 flow + token refresh
│   ├── client/client.go                    # HTTP client with retry/backoff
│   ├── fetch/fetch.go                      # GetRecentlyPlayed, GetTopTracks, GetTopArtists
│   ├── models/models.go                    # Play, TopTrack, TopArtist structs
│   ├── plays/plays.go                      # plays.json read/write/dedup
│   └── render/render.go                    # weekly note + artist stub + context pack
├── templates/
│   ├── weekly.md.tmpl
│   └── persona.md.tmpl
├── data/
│   └── plays.json                          # git-ignored
├── tokens.json                             # git-ignored, 0600 perms
├── .env                                    # git-ignored (CLIENT_ID, CLIENT_SECRET, VAULT_PATH)
├── .env.example
├── .gitignore
├── run_music_collect_spotify.sh                  # launchd wrapper
└── run_music_weekly_spotify.sh                   # launchd wrapper
```

## Commands

| Command | What it does |
|---|---|
| `music-garden auth` | Full OAuth2 browser flow → saves tokens.json |
| `music-garden collect` | Fetch last 50 recently-played, dedup, append to plays.json |
| `music-garden weekly [--date YYYY-MM-DD]` | Generate weekly note for date's ISO week (default: current) |
| `music-garden catch-up [--weeks N]` | Scan music/listening/ for missing weekly notes, generate each (default: 8 weeks) |
| `music-garden persona` | Regenerate Music Taste context pack in 01-ai-brain/context-packs/ |

## Implementation Detail

### internal/auth/auth.go

Mirror the WHOOP auth pattern exactly:
- `StartAuthFlow()` — open browser to Spotify authorize URL, local HTTP server on `:8888` captures callback, exchange code for tokens, save to `tokens.json`
- `RefreshIfNeeded()` → string — load tokens, refresh if within 5 min of expiry, return access token

**Spotify OAuth endpoints:**
- Auth: `https://accounts.spotify.com/authorize`
- Token: `https://accounts.spotify.com/api/token`
- Auth header for token exchange: `Basic base64(CLIENT_ID:CLIENT_SECRET)`
- Redirect URI: `http://localhost:8888/callback` *(must be added to Spotify app settings)*

**Scopes:**
- `user-read-recently-played` — for `collect`
- `user-top-read` — for `weekly` and `persona`

**Token storage (tokens.json):**
```json
{
  "access_token": "...",
  "refresh_token": "...",
  "expires_at": "2026-02-21T14:00:00Z"
}
```

### internal/client/client.go

Copy WHOOP pattern:
- `Client{accessToken, baseURL}` where `baseURL = "https://api.spotify.com/v1"`
- `Get(path, params)` with exponential backoff on HTTP 429 (1s, 2s, 4s, give up after 4 tries)

### internal/models/models.go

```go
type Play struct {
    PlayedAt         string `json:"played_at"`
    TrackID          string `json:"track_id"`
    TrackName        string `json:"track_name"`
    ArtistID         string `json:"artist_id"`
    ArtistName       string `json:"artist_name"`
    ArtistSpotifyURL string `json:"artist_spotify_url"`
    AlbumName        string `json:"album_name"`
    DurationMS       int    `json:"duration_ms"`
    TrackSpotifyURL  string `json:"track_spotify_url"`
}

type TopTrack struct {
    ID         string
    Name       string
    ArtistName string
}

type TopArtist struct {
    ID         string
    Name       string
    Genres     []string
    SpotifyURL string
}
```

### internal/fetch/fetch.go

- `GetRecentlyPlayed(c)` → []Play — GET `/me/player/recently-played?limit=50`, filter items with no `track` key (podcasts), map to Play structs (primary artist only)
- `GetTopTracks(c, timeRange)` → []TopTrack — GET `/me/top/tracks?limit=50&time_range={short_term|medium_term|long_term}`
- `GetTopArtists(c, timeRange)` → []TopArtist — GET `/me/top/artists?limit=50&time_range={short_term|medium_term|long_term}`

### internal/plays/plays.go

- `Load(path)` → []Play — read plays.json, return empty slice if not exists
- `Save(path, plays)` — write sorted descending by played_at, 0644 perms
- `Merge(existing, incoming)` → []Play — union by played_at key, sort descending

### internal/render/render.go

**Weekly note** (replicates Python weekly_music_note.py output exactly):
- YAML frontmatter (type: note, tags: [music, weekly-music], created, week)
- Stats block (plays, unique tracks/artists/albums, listening time)
- Play Log grouped by local date (day header, time + track + artist wikilink + album)
- Repeated Tracks (≥2 plays)
- Albums This Week
- Artists in Rotation (wikilinks)
- New Artists (first appearance — check if `music/artists/<Name>.md` exists)
- Top Tracks section (short_term top 50)
- Top Artists section (short_term top 50, with genres)
- Notes (empty)

**Artist stubs** — called during weekly note generation:
- Check if `music/artists/<Artist Name>.md` exists — skip if yes (never overwrite)
- Write stub with frontmatter (type: resource, tags: [music/artist], spotify_url, genres), dataview query block

**Music Taste context pack:**
- Written to `01-ai-brain/context-packs/Music Taste.md` (overwritten each run)
- Frontmatter (type: context, tags: [ai-brain/context, music])
- Current Top Artists (short_term), medium_term, long_term
- Top Genres (derived from short_term artist genres)
- Recent Rotation (from this week's plays)

**Timezone handling:** Parse `played_at` as UTC, convert to local with `.Local()` for display.

**ISO week year:** Always use `date.ISOWeek()` for week numbers (not `date.Year()`).

### main.go

Same dispatch pattern as whoop-garden:
- `loadDotEnv(".env")`
- `vaultPath()` → `$OBSIDIAN_VAULT_PATH` env var
- `templatesDir()` → `templates/` relative to binary or cwd
- `runAuth()`, `runCollect()`, `runWeekly()`, `runCatchUp()`, `runPersona()`

### Shell wrappers

**run_music_collect_spotify.sh:**
```bash
#!/bin/zsh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$HOME/.zprofile" 2>/dev/null || true
source "$HOME/.zshrc" 2>/dev/null || true
cd "$SCRIPT_DIR"
exec ./music-garden collect
```

**run_music_weekly_spotify.sh:**
```bash
#!/bin/zsh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$HOME/.zprofile" 2>/dev/null || true
source "$HOME/.zshrc" 2>/dev/null || true
cd "$SCRIPT_DIR"
./music-garden catch-up --weeks 8
./music-garden weekly
./music-garden persona
```

### launchd plists

Distribute as `*.plist.example`. Users copy, rename, edit the path and label:

**music-collect-spotify.plist.example** — 5x daily (7, 11, 15, 19, 23):
```xml
<key>StartCalendarInterval</key>
<array>
    <dict><key>Hour</key><integer>7</integer><key>Minute</key><integer>0</integer></dict>
    <dict><key>Hour</key><integer>11</integer><key>Minute</key><integer>0</integer></dict>
    <dict><key>Hour</key><integer>15</integer><key>Minute</key><integer>0</integer></dict>
    <dict><key>Hour</key><integer>19</integer><key>Minute</key><integer>0</integer></dict>
    <dict><key>Hour</key><integer>23</integer><key>Minute</key><integer>0</integer></dict>
</array>
```

**music-weekly-spotify.plist.example** — Sunday at 11 PM.

## Data Migration

If migrating from an existing plays.json:

```bash
cp /path/to/old/plays.json /path/to/obsidian-music-garden/data/plays.json
```

First `collect` run will merge + deduplicate with any new plays.

## One Setup Requirement

The Spotify developer dashboard app needs `http://localhost:8888/callback` added as a valid redirect URI. After updating, run `music-garden auth` for initial token exchange.

## Build Order

1. `go mod init` + directory scaffold
2. `internal/models/models.go`
3. `internal/auth/auth.go`
4. `internal/client/client.go`
5. `internal/fetch/fetch.go`
6. `internal/plays/plays.go`
7. `templates/weekly.md.tmpl` + `templates/persona.md.tmpl`
8. `internal/render/render.go`
9. `main.go`
10. Shell wrappers + `.env` + `.gitignore`
11. Build + smoke test
12. launchd plists + load

## Verification Checklist

- [ ] `go build -o music-garden .` — builds clean
- [ ] `./music-garden auth` — browser opens, tokens.json written
- [ ] `./music-garden collect` — plays.json grows, no duplicates on re-run
- [ ] `./music-garden weekly` — `music/listening/spotify-YYYY-Www.md` written, artist stubs created
- [ ] Delete a weekly note, run `./music-garden catch-up --weeks 4` — regenerates missing note
- [ ] `./music-garden persona` — `01-ai-brain/context-packs/Music Taste.md` overwritten
- [ ] Load plists, `launchctl list | grep spotify` — both jobs visible
