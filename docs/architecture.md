# Architecture

spotify-garden is a thin pipeline: **fetch → model → render → write**. Each
stage is a separate package with no circular dependencies.

## Package Map

```
main.go                         CLI entry, runtime path resolution, subcommand dispatch
internal/
  auth/auth.go                  OAuth2 flow, token save/load/refresh
  client/client.go              Authenticated HTTP GET, 429 retry/backoff
  fetch/fetch.go                Spotify + setlist.fm API calls → model structs
  models/models.go              Play, TopTrack, TopArtist, Setlist, SetlistSet structs
  plays/plays.go                plays load/save/merge/dedup + sharded storage
  render/render.go              Weekly note, artist stubs, persona rendering
templates/
  persona.md.tmpl               Go template for Music Taste context pack
  weekly.md.tmpl                Structure reference (rendering is in Go code)
data/
  plays/                        Sharded play history — YYYY/YYYY-WNN.json (git-committed via Actions)
  plays.json.bak                Legacy file renamed on first post-upgrade collect (can be deleted)
  genres.json                   Artist genre cache
```

## Data Flow

### `collect` command

```
main.runCollect()
  │
  ├─ auth.RefreshIfNeeded()
  │    └─ loads effective tokens path, refreshes access token if expiring within 5 min
  │
  ├─ client.NewClient(token)
  │
  ├─ fetch.GetRecentlyPlayed(c)
  │    └─ GET /me/player/recently-played?limit=50
  │         filters podcast episodes (no track key)
  │         maps to []models.Play (primary artist only)
  │
  ├─ plays.MigrateToSharded(legacyPlaysPath, playsDir)   ← one-shot; no-op after first run
  │    └─ reads data/plays.json, writes to data/plays/YYYY/YYYY-WNN.json,
  │       renames plays.json → plays.json.bak
  │
  ├─ plays.SaveSharded(playsDir, incoming)
  │    └─ routes each play to its ISO week file, merge+dedup per file
  │
  └─ if SPOTIFY_AUTO_DAILY_ON_COLLECT=1:
       generateDailyNote(allPlays, now, overwrite=true)
```

### `weekly` command

```
main.runWeekly()  /  main.generateWeeklyNote(date)
  │
  ├─ plays.LoadSharded(playsDir)
  │
  └─ render.RenderWeekly(allPlays, date, vaultPath)
       │
       ├─ render.PlaysForWeek()     filter plays to ISO week (local time)
       ├─ compute stats             unique tracks/artists/albums, duration
       ├─ compute repeated tracks   ≥2 plays
       ├─ compute albums            sorted by play count
       ├─ render.EnsureArtistStub() for each artist (skip if exists)
       └─ build summary note        → os.WriteFile
```

### `daily` command

```
main.runDaily()
  │
  ├─ plays.LoadSharded(playsDir)
  │
  └─ main.generateDailyNote(allPlays, date, overwrite=false)
       │
       ├─ render.PlaysForDay()      filter plays to local day
       ├─ render.EnsureArtistStub() for each artist in day plays (skip if exists)
       └─ render.RenderDaily(...)
            ├─ compute stats             unique tracks/artists/albums, duration
            ├─ build full play log       every play event in order
            ├─ build song/artist/album lists with counts
            └─ build note string         → os.WriteFile (if missing)
```

### `catch-up` command

```
main.runCatchUp()
  │
  ├─ weekly pass:
  │    for each of last N weeks:
  │      check {vault}/music/listening/spotify-YYYY-Www.md exists
  │    generate missing weeks (oldest first)
  │
  └─ daily pass:
       plays.LoadSharded(playsDir) once
       for each of last N*7 days:
         check {vault}/music/listening/spotify-YYYY-MM-DD.md exists
         generate missing daily notes (skips no-play days)
```

### `persona` command

```
main.runPersona()
  │
  ├─ auth.RefreshIfNeeded()
  ├─ client.NewClient(token)
  │
  ├─ fetch.GetTopArtists(c, "short_term")
  ├─ fetch.GetTopArtists(c, "medium_term")
  ├─ fetch.GetTopArtists(c, "long_term")
  │
  ├─ plays.LoadSharded(playsDir)
  ├─ render.PlaysForWeek(allPlays, now)   ← this week's plays for Recent Rotation
  │
  └─ render.RenderPersona(...)
       └─ text/template execution against templates/persona.md.tmpl
            → os.WriteFile({vault}/01-ai-brain/context-packs/Music Taste.md)
```

### `setlist` command

```
main.runSetlist(args)
  │
  ├─ parse --date flag (default: today)
  │
  └─ fetch.GetSetlist(artistName, date)
       │
       ├─ setlistGet("/search/setlists", params)
       │    └─ GET https://api.setlist.fm/rest/1.0/search/setlists
       │         x-api-key: $SETLISTFM_API_KEY
       │         params: artistName, date (DD-MM-YYYY), p=1
       │
       └─ map first result → models.Setlist
            → print to stdout
```

No vault writes. No Spotify auth required.

## Play Data Storage

Plays are stored in a sharded annual/weekly structure. Each ISO week gets its
own JSON file; no file is created for weeks with no plays.

```
data/plays/
  2025/
    2025-W01.json
    2025-W52.json
  2026/
    2026-W10.json
    2026-W11.json
```

Each file is a JSON array of play objects, sorted descending by `played_at`:

```json
[
  {
    "played_at": "2026-02-21T14:30:00.000Z",
    "track_id": "...",
    "track_name": "Track Name",
    "artist_id": "...",
    "artist_name": "Artist Name",
    "artist_spotify_url": "https://open.spotify.com/artist/...",
    "album_name": "Album Name",
    "duration_ms": 210000,
    "track_spotify_url": "https://open.spotify.com/track/..."
  }
]
```

Only the primary artist is recorded (index 0 of the `artists` array).

**Shard key timezone:** UTC — Spotify timestamps are UTC, so routing is
deterministic regardless of where the binary runs. Display and filtering for
notes continues to use local time (existing behaviour).

**Legacy migration:** On the first `collect` run after upgrading from a version
that stored a flat `data/plays.json`, `plays.MigrateToSharded` automatically
reads the legacy file, writes it into the sharded layout, and renames the
original to `data/plays.json.bak`. Subsequent runs skip migration because
`.bak` already exists. The `.bak` file is kept as a safety net and can be
deleted once migration is confirmed.

## ISO Week Handling

All week calculations use `time.ISOWeek()` — not `time.Year()`. This ensures
correct behaviour near year boundaries (e.g. Dec 31 may belong to week 1 of
the following year).

Week boundaries are computed in **local time**: Monday 00:00:00 → next Monday
00:00:00 (exclusive). Plays are filtered using local timestamps so the play
log displays in the user's timezone.

## Runtime Path Resolution

Runtime file locations are resolved with this precedence:
1. CLI flags (where applicable)
2. Environment variables
3. `SPOTIFY_STATE_DIR` files (`.env`, `tokens.json`, `data/plays/`, `data/genres.json`)
4. CWD fallback with warning

`playsDir` (`data/plays/`) is derived from `filepath.Dir(playsPath)` — it inherits
the same `SPOTIFY_STATE_DIR` override logic automatically without a separate env var.

`spotify-garden doctor` prints all effective runtime paths and launchd-derived diagnostics.

## Template Resolution

At startup, `templatesDir()` resolves in order:

1. `$SPOTIFY_TEMPLATES_DIR` env var
2. `./templates/` relative to cwd (development)
3. `<binary_dir>/templates/` next to the compiled binary

The weekly note template is the exception — rendering is done in Go code
(`render.RenderWeekly`) using `strings.Builder` for full control over
whitespace and conditional sections. `templates/weekly.md.tmpl` documents the
output structure but is not executed.

## Rate Limiting

`client.Get` retries up to 3 times on HTTP 429 with exponential backoff:
1 s → 2 s → 4 s. After 4 attempts it returns an error.

## Key Design Decisions

**Zero external dependencies** — pure stdlib. No module cache issues, no
supply chain risk, no version drift.

**Sharded play storage (`data/plays/YYYY/YYYY-WNN.json`)** — The old flat
`data/plays.json` grows without bound. Plays are now written directly into
annual/weekly shard files at collection time. Each ISO week has its own file;
empty weeks create no file. Shard keys use UTC so routing is deterministic.
On the first `collect` run after upgrade, `plays.MigrateToSharded` moves the
legacy file to `data/plays.json.bak` automatically. See
[`docs/plans/2026-03-09-sharded-plays-storage.md`](plans/2026-03-09-sharded-plays-storage.md)
for the full design rationale.

**Sharded play cache** — Spotify's recently-played endpoint returns only the
last 50 tracks and has no historical pagination. Running `collect` 5× daily
ensures no plays are lost to the 50-track API cap. With sharding, each
`collect` run only appends to the current week's file rather than rewriting
the full history.

**Weekly note rendered in Go, persona via template** — The weekly note has
many conditional sections with complex formatting logic. Building it with
`strings.Builder` in Go code (mirroring the original Python approach) is more
maintainable than a template with heavy whitespace trimming. The persona note
has simple structure well-suited to `text/template`.

**Artist stubs never overwritten** — Once created, a stub at
`music/artists/{Name}.md` is left alone. Users can freely add notes, links,
and metadata to stubs without risking them being clobbered on the next run.

**catch-up minimizes API calls** — Weekly generation may need Spotify API calls
(top tracks/artists), but daily generation is local-only from `data/plays/`.

**setlist uses a standalone HTTP helper, not the Spotify client** — setlist.fm
has a different base URL, auth scheme (header-based API key vs. Bearer token),
and no rate-limit retry needs. A thin `setlistGet()` function in `fetch.go`
handles it without complicating the `client.Client` struct.

**Concert notes are manual, not automatic** — Concert data has no single
reliable API source (ticketing APIs require approval, email parsing is brittle).
The `setlist` command provides lookup assistance, but note creation is done
via the Obsidian Templater template. This keeps the note a personal writing
artifact rather than a synthetic document.
