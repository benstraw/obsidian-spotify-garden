# Commands

All commands auto-load `.env` from the current working directory and
auto-refresh OAuth tokens if they are expiring within 5 minutes.

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
6. Saves tokens to `tokens.json` in the current directory (mode `0600`)

If the browser does not open automatically, the full auth URL is printed to
stdout — copy and paste it manually.

Tokens auto-refresh on subsequent commands. You should only need to run `auth`
once, unless `tokens.json` is deleted or the refresh token expires.

**Requires:** `SPOTIFY_CLIENT_ID`, `SPOTIFY_CLIENT_SECRET`, `SPOTIFY_REDIRECT_URI` in `.env`.

---

## collect

```bash
./spotify-garden collect
```

Fetches the last 50 recently-played tracks from the Spotify API and merges
them into `data/plays.json`.

**Behaviour:**
1. Calls `GET /me/player/recently-played?limit=50`
2. Filters out podcast episodes (items with no `track` key)
3. Loads existing `data/plays.json` (empty slice if the file doesn't exist)
4. Merges using `played_at` as the dedup key — existing plays are never duplicated
5. Saves sorted descending by `played_at`

**Output:** `data/plays.json` (local to the project, git-ignored)

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
2. Fetches top tracks and top artists (`short_term`) from the API
3. Filters `data/plays.json` for plays that fall within the week
4. Creates artist stubs for new artists (see below)
5. Writes the weekly note (always overwrites if it already exists)

**Output:** `{vault}/music/listening/spotify-YYYY-Www.md`

**Weekly note sections:**
- YAML frontmatter (`type: note`, `tags: [music, weekly-music]`, `created`, `week`)
- Stats block: play count, unique tracks/artists/albums, total listening time
- Play Log grouped by local date, with time, track, artist wikilink, album
- Repeated Tracks (≥2 plays in the week)
- Albums This Week (sorted by play count)
- Artists in Rotation (wikilinks, sorted alphabetically)
- New Artists (first appearance — no stub existed before this run)
- Top Tracks — Last ~4 Weeks (short_term top 50)
- Top Artists — Last ~4 Weeks (short_term top 50, with genres)
- Notes (empty section)

**Artist stubs** — created at `{vault}/music/artists/{Name}.md` for every
artist in the week's plays and in the short_term top artists list. Never
overwrites an existing stub. Each stub includes frontmatter (`type: resource`,
`tags: [music/artist]`, `spotify_url`, `genres`) and a dataview query that
lists all weekly notes linking to the artist.

---

## catch-up

```bash
./spotify-garden catch-up [--weeks N]
```

Scans the vault's listening directory for missing weekly notes and generates
only those. Existing notes are never overwritten.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--weeks` | 8 | Number of weeks back to scan |

**Behaviour:**
1. Checks for `spotify-YYYY-Www.md` in `{vault}/music/listening/` for each
   of the last N weeks
2. If all files exist, exits immediately with "All caught up"
3. Generates missing weeks in chronological order (oldest first)
4. Each generation follows the same process as `weekly`

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
- This week's plays from `data/plays.json` for the Recent Rotation section

**Context pack sections:**
- Current Top Artists (last ~4 weeks)
- Top Artists (last ~6 months)
- All-Time Top Artists
- Top Genres (derived from short_term artist genres, deduplicated, up to 15)
- Recent Rotation (unique artists heard this week, sorted)
- Notes (empty)

Intended to be read by AI assistants when creating playlists, recommending
music, or discussing musical taste.
