# Architecture

spotify-garden is a thin pipeline: **fetch → model → render → write**. Each
stage is a separate package with no circular dependencies.

## Package Map

```
main.go                         CLI entry, .env loading, subcommand dispatch
internal/
  auth/auth.go                  OAuth2 flow, token save/load/refresh
  client/client.go              Authenticated HTTP GET, 429 retry/backoff
  fetch/fetch.go                Spotify API calls → model structs
  models/models.go              Play, TopTrack, TopArtist structs
  plays/plays.go                plays.json load/save/merge/dedup
  render/render.go              Weekly note, artist stubs, persona rendering
templates/
  persona.md.tmpl               Go template for Music Taste context pack
  weekly.md.tmpl                Structure reference (rendering is in Go code)
data/
  plays.json                    Local play history (git-ignored)
```

## Data Flow

### `collect` command

```
main.runCollect()
  │
  ├─ auth.RefreshIfNeeded()
  │    └─ loads tokens.json, refreshes access token if expiring within 5 min
  │
  ├─ client.NewClient(token)
  │
  ├─ fetch.GetRecentlyPlayed(c)
  │    └─ GET /me/player/recently-played?limit=50
  │         filters podcast episodes (no track key)
  │         maps to []models.Play (primary artist only)
  │
  ├─ plays.Load("data/plays.json")
  │
  ├─ plays.Merge(existing, incoming)
  │    └─ union by played_at key, sorted descending
  │
  └─ plays.Save("data/plays.json")
```

### `weekly` command

```
main.runWeekly()  /  main.generateWeeklyNote(date)
  │
  ├─ auth.RefreshIfNeeded()
  ├─ client.NewClient(token)
  │
  ├─ fetch.GetTopTracks(c, "short_term")
  │    └─ GET /me/top/tracks?limit=50&time_range=short_term
  │
  ├─ fetch.GetTopArtists(c, "short_term")
  │    └─ GET /me/top/artists?limit=50&time_range=short_term
  │
  ├─ plays.Load("data/plays.json")
  │
  └─ render.RenderWeekly(allPlays, topTracks, topArtists, date, vaultPath)
       │
       ├─ render.PlaysForWeek()     filter plays to ISO week (local time)
       ├─ compute stats             unique tracks/artists/albums, duration
       ├─ group by local day        for play log
       ├─ compute repeated tracks   ≥2 plays
       ├─ compute albums            sorted by play count
       ├─ render.EnsureArtistStub() for each artist (skip if exists)
       └─ build note string         → os.WriteFile
```

### `catch-up` command

```
main.runCatchUp()
  │
  ├─ for each of last N weeks:
  │    check {vault}/music/listening/spotify-YYYY-Www.md exists
  │
  ├─ if none missing: exit "All caught up"
  │
  └─ for each missing week (oldest first):
       generateWeeklyNote(date)   ← same as weekly command
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
  ├─ plays.Load("data/plays.json")
  ├─ render.PlaysForWeek(allPlays, now)   ← this week's plays for Recent Rotation
  │
  └─ render.RenderPersona(...)
       └─ text/template execution against templates/persona.md.tmpl
            → os.WriteFile({vault}/01-ai-brain/context-packs/Music Taste.md)
```

## plays.json

The central data store. A JSON array of play objects, sorted descending by
`played_at`. Written by `collect`, read by `weekly` and `persona`.

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

## ISO Week Handling

All week calculations use `time.ISOWeek()` — not `time.Year()`. This ensures
correct behaviour near year boundaries (e.g. Dec 31 may belong to week 1 of
the following year).

Week boundaries are computed in **local time**: Monday 00:00:00 → next Monday
00:00:00 (exclusive). Plays are filtered using local timestamps so the play
log displays in the user's timezone.

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

**plays.json as local cache** — Spotify's recently-played endpoint returns
only the last 50 tracks and has no historical pagination. Running `collect`
5× daily ensures the local cache captures all plays before they fall out of
the 50-track window.

**Weekly note rendered in Go, persona via template** — The weekly note has
many conditional sections with complex formatting logic. Building it with
`strings.Builder` in Go code (mirroring the original Python approach) is more
maintainable than a template with heavy whitespace trimming. The persona note
has simple structure well-suited to `text/template`.

**Artist stubs never overwritten** — Once created, a stub at
`music/artists/{Name}.md` is left alone. Users can freely add notes, links,
and metadata to stubs without risking them being clobbered on the next run.

**catch-up checks file existence before auth** — `runCatchUp` scans for
missing files before calling `RefreshIfNeeded()`. If everything is up to date,
it exits without making any API calls.
