# Commands

All commands use this runtime path precedence:
1. CLI flags (where applicable)
2. Environment variables
3. `SPOTIFY_STATE_DIR` files (`.env`, `tokens.json`, `data/plays/`, `data/genres.json`)
4. Current working directory fallback (with warning)

OAuth tokens auto-refresh if they are expiring within 5 minutes.

---

## auth

```bash
./spotify-garden auth
```

Runs the full OAuth 2.0 authorization code flow:

1. Builds a Spotify authorization URL with the required scopes
2. Opens it in your default browser (macOS `open`)
3. For **localhost redirect URIs**: starts a local HTTP server on `:8888` to
   capture the callback automatically
4. For **external redirect URIs**: prompts you to paste the full redirect URL
   from your browser's address bar
5. Exchanges the authorization code for access and refresh tokens
6. Saves tokens to the effective `tokens.json` path (mode `0600`)

If the browser does not open automatically, the full auth URL is printed to
stdout — copy and paste it manually.

Tokens auto-refresh on subsequent commands. You should only need to run `auth`
once, unless `tokens.json` is deleted or the refresh token expires.

**Requires:** `SPOTIFY_CLIENT_ID`, `SPOTIFY_CLIENT_SECRET`, `SPOTIFY_REDIRECT_URI` in `.env` or environment.

---

## collect

```bash
./spotify-garden collect
```

Fetches the last 50 recently-played tracks from the Spotify API and merges
them into the weekly shard file for the current ISO week under the effective
plays directory (`SPOTIFY_STATE_DIR/data/plays/` when configured).

**Behaviour:**
1. Calls `GET /me/player/recently-played?limit=50`
2. Filters out podcast episodes (items with no `track` key)
3. On first run after upgrade: migrates `data/plays.json` → sharded layout and renames the legacy file to `data/plays.json.bak`
4. Routes each new play to its ISO week file (`data/plays/YYYY/YYYY-WNN.json`), merging with the existing file
5. Deduplicates by `played_at` — existing plays are never duplicated
6. If `SPOTIFY_AUTO_DAILY_ON_COLLECT=1`, regenerates today's daily note
   (`spotify-YYYY-MM-DD.md`) so it stays up to date as new plays arrive

**Output:** `{playsDir}/YYYY/YYYY-WNN.json` (e.g. `data/plays/2026/2026-W11.json`)

Since Spotify only returns the last 50 plays, running `collect` 5× daily
ensures no plays are lost to the 50-track API cap.

---

## weekly

```bash
./spotify-garden weekly [--date YYYY-MM-DD]
```

Generates a weekly markdown note for the ISO week (Mon–Sun) containing the
given date (default: the current week).

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--date` | today | Any date within the target week |

**What it does:**
1. Determines the ISO week (Monday 00:00 → Sunday 23:59 local time)
2. Filters the effective plays file for plays that fall within the week
3. Creates artist stubs for new artists (see below)
4. Writes the weekly note (always overwrites if it already exists)

**Output:** `{vault}/music/listening/spotify-YYYY-Www.md`

**Weekly note sections:**
- YAML frontmatter (`type: note`, `tags: [music, weekly-music]`, `created`, `week`)
- Stats block: play count, unique tracks/artists/albums, total listening time
- Repeated Tracks (≥2 plays in the week)
- Albums This Week (sorted by play count)
- Artists in Rotation (wikilinks, sorted alphabetically)
- New Artists (first appearance — no stub existed before this run)
- Notes (empty section)

**Artist stubs** — created at `{vault}/music/artists/{Name}.md` for every
artist in the week's plays. Never overwrites an existing stub. Each stub
includes frontmatter (`type: resource`, `tags: [music/artist]`, `spotify_url`,
`genres`) and a dataview query that lists all weekly notes linking to the artist.

---

## daily

```bash
./spotify-garden daily [--date YYYY-MM-DD]
```

Generates a daily markdown note for the given calendar date (default: today).

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--date` | today | Date in `YYYY-MM-DD` (interpreted in local timezone) |

**Behaviour:**
1. Loads the effective plays file
2. Filters plays for the local calendar day
3. Creates missing artist stubs for artists heard that day
4. If no plays exist for that day, exits without writing a file
5. If a note already exists, skips (never overwrites)
6. Otherwise writes the daily note

**Output:** `{vault}/music/listening/spotify-YYYY-MM-DD.md`

**Daily note sections:**
- YAML frontmatter (`type: note`, `tags: [music, daily-music]`, `created`, `date`)
- Stats block: play count, unique tracks/artists/albums, total listening time
- Play Log with local times, track, artist wikilink, album
- Songs Played (all song+artist+album combinations with play counts)
- Artists Played (all artists with play counts)
- Albums Played (all album+artist combinations with play counts)
- Notes (empty section)

**Artist stubs:** `daily` also creates missing artist stubs at
`{vault}/music/artists/{Name}.md` for artists heard on that day.

---

## catch-up

```bash
./spotify-garden catch-up [--weeks N]
```

Scans the vault's listening directory for missing weekly and daily notes and
generates only what is missing. Existing notes are never overwritten.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--weeks` | 8 | Number of weeks back to scan |

**Behaviour:**
1. Checks for `spotify-YYYY-Www.md` in `{vault}/music/listening/` for each
   of the last N weeks
2. Generates missing weekly notes in chronological order (oldest first)
3. Loads the effective plays file once, then checks the last `N*7` days for missing
   `spotify-YYYY-MM-DD.md` files
4. Generates missing daily notes (skips days with no plays)

This is the preferred command for the scheduled Sunday run — it fills any
gaps from missed `collect` windows without overwriting notes you have already
edited.

---

## persona

```bash
./spotify-garden persona
```

Regenerates the Music Taste context pack at
`{vault}/01-ai-brain/context-packs/Music Taste.md` (always overwrites).

**What it fetches:**
- Top 50 artists for `short_term` (~4 weeks), `medium_term` (~6 months), `long_term` (all time)
- This week's plays from the effective plays file for the Recent Rotation section

**Context pack sections:**
- Current Top Artists (last ~4 weeks)
- Top Artists (last ~6 months)
- All-Time Top Artists
- Top Genres (derived from short_term artist genres, deduplicated, up to 15)
- Recent Rotation (unique artists heard this week, sorted)
- Notes (empty)

Intended to be read by AI assistants when creating playlists, recommending
music, or discussing musical taste.

---

## setlist

```bash
./spotify-garden setlist <artist> [--date YYYY-MM-DD]
```

Looks up a setlist on setlist.fm and prints it to stdout. No vault files are written.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--date` | today | Date of the concert in YYYY-MM-DD format |

**Requires:** `SETLISTFM_API_KEY` in `.env`.
Get a key at [setlist.fm/settings/apps](https://www.setlist.fm/settings/apps).

**Output format:**

```
Artist Name — Venue Name — City, ST
2026-02-21

Set 1:
1. Song Title
2. Song Title
...

Encore:
1. Song Title

Setlist.fm: https://www.setlist.fm/setlist/...
```

**Concert note workflow:**
1. During or after a show, open the Templater template `Concert Note` in Obsidian — it prompts for artist and venue, then renames the file to `YYYY-MM-DD - Artist - Venue.md` and places it in `music/concerts/`
2. Run `spotify-garden setlist "<Artist>" --date YYYY-MM-DD`, copy the output, and paste it into the Set List section of the note
3. The artist stub's Concerts Dataview block will automatically pick up the new note via the `music/live-artist/<Artist Name>` tag

---

## doctor

```bash
./spotify-garden doctor
```

Prints effective runtime configuration and diagnostics in one place:

1. Working directory and executable path
2. Effective `.env`, `tokens.json`, `data/plays/` (plays dir), `data/plays.json` (legacy, if present), templates, vault/listening paths
3. State-dir fallback warnings
4. Launchd labels and expected log paths
5. Best-effort loaded/not-loaded launchd job status

Exit code is `0` when no issues are found and nonzero when warnings/errors are detected.
