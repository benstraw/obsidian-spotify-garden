# Plan: Concert Notes Notetaker App

## Summary

Design a streamlined concert-notes input tool — separate from Obsidian itself —
that captures show details, setlists, and personal observations at or after a
live event and exports a clean Obsidian-ready markdown file into the vault.

The existing `spotify-garden setlist` command already automates setlist retrieval
from setlist.fm. This plan extends the concert-note workflow to make the
pre-note capture step just as frictionless, removing the need for a browser,
Obsidian Templater prompts, and manual copy-paste during or after a show.

---

## Current Workflow Pain Points

1. The existing workflow requires Obsidian + Templater to be open during/after
   the show to create and name the note.
2. Setlist data must be fetched separately from the terminal (`spotify-garden
   setlist`), then manually pasted into the note.
3. There is no place to capture real-time observations (set notes, crowd vibe,
   openers, rating, photos) while the show is in progress.
4. The flow is laptop-only — phone use during shows is more natural.

---

## Competitive Analysis

### Apps with Obsidian Integration

| App | Platform | Obsidian Integration | Relevant Strengths | Gaps |
|-----|----------|---------------------|--------------------|------|
| **Obsidian + Templater** (current) | Desktop/Mobile | Native | Fully in-vault, wikilinks, tags | Manual, no setlist.fm auto-fill, friction mid-show |
| **Obsidian Sync + Mobile app** | iOS/Android | Native | Sync notes straight into vault | Same templating friction on mobile |
| **Bear** | macOS/iOS | Export to md | Fast capture, good mobile UX | Separate silo — requires manual vault copy/rename |
| **Notion** | Web/iOS/Android | Markdown export | Rich templates, sharing | Heavy; export is multi-step; no wikilink format |
| **Craft** | macOS/iOS | Markdown export | Clean UX, blocks, sharing | Same export friction; no concert-specific template |
| **Capacities** | Web/Desktop | No direct md export | Notion-like with types | No Obsidian path; export not folder-addressable |
| **Logseq** | Desktop/Mobile | Compatible md format | Outlines, daily notes, open-source | Opinionated block format may conflict with vault style |
| **Setlist.fm app** | iOS/Android | None | Best data source, community edits | No notes, no Obsidian output |
| **ConcertVault / Songkick** | Web/iOS | None | Event discovery, history | No notetaking or export |
| **Last.fm** | Web/iOS | None | Scrobbling, listen history | No live-show notes |

### Dedicated Concert Tracking Apps

| App | Notes |
|-----|-------|
| **Setlist.fm** | Best community setlist DB; has public API (used by `spotify-garden setlist`). No per-user note field on individual shows. |
| **LiveHere** (iOS) | Concert diary; photo + rating. No markdown export, no API, no Obsidian path. |
| **Concert Diary** (iOS/Android) | Simple logbook. No export, no integration. |
| **ConcertWall** | Community feed; no personal notes, no export. |

### Key Competitive Gaps

None of the above tools combine:
- Mobile-friendly real-time capture during a show
- Auto-populated setlist from setlist.fm (with fallback for unlogged setlists)
- Formatted, vault-ready markdown output (correct filename, frontmatter,
  wikilinks, folder path)
- Zero post-show copy-paste work

**This is the product gap the MVP should fill.**

---

## MVP Scope

### Guiding Principles

- **Minimal friction**: one-screen capture. Should be usable in a dark venue
  on a phone with one hand.
- **No new cloud infra required for v1**: write directly to the vault folder
  on disk (macOS/iOS via Files app, or Obsidian Sync folder path).
- **Extend, don't duplicate**: reuse the existing `setlist` command output and
  `plays.json` data; the concert note is a first-class citizen in the existing
  vault schema.

### Output Format

The generated note must be drop-compatible with the existing concert-note schema
described in `docs/commands.md` (setlist command section):

```
music/concerts/YYYY-MM-DD - Artist - Venue.md
```

**Frontmatter:**

```yaml
---
type: note
tags:
  - music/concert
  - music/live-artist/<Artist Name>
artist: "[[<Artist Name>]]"
venue: "[[<Venue Name>]]"
date: YYYY-MM-DD
city: "City, ST"
support: ""
rating: ""
created: YYYY-MM-DDTHH:MM:SS
---
```

**Body sections:**

```markdown
## Setlist

*(auto-populated from setlist.fm, or blank template if not yet logged)*

## Notes

*(freeform, captured during/after show)*

## Opener(s)

## Vibe / Highlights

## Rating

/10
```

### MVP Delivery Options

Three approaches are viable for v1; they are not mutually exclusive.

#### Option A — CLI command in `spotify-garden` (lowest effort, desktop-only)

Add a `concert` command alongside `setlist`:

```bash
spotify-garden concert "<Artist>" "<Venue>" [--date YYYY-MM-DD] [--notes "..."]
```

- Fetches setlist from setlist.fm (same as `setlist` command, reuses
  `fetch.GetSetlist`)
- Writes the markdown note to `{vault}/music/concerts/YYYY-MM-DD - Artist - Venue.md`
- Creates an artist stub if one does not exist
- Prints the note path on success

**Effort:** ~1 day. No new dependencies. Fits naturally in existing Go codebase.
**Limitation:** Desktop/terminal only; not useful mid-show.

#### Option B — Web micro-app (mobile-friendly, cross-platform)

A single-page HTML/JS app (no build step, single `.html` file) that:

1. Prompts for Artist, Venue, Date, City (pre-filled to today/current location)
2. Calls setlist.fm API (client-side, proxied through a lightweight Go handler
   or via a CORS-enabled serverless function) to auto-fill the setlist
3. Provides a freeform Notes textarea
4. On submit: generates the markdown file and either:
   - Downloads it as a `.md` file (user drags it into vault)
   - Or writes it directly to the vault via Obsidian URI scheme
     (`obsidian://new?file=...&content=...`)

**Effort:** ~2–3 days for the web UI + Obsidian URI integration.
**Limitation:** Obsidian URI length limit (~8 KB) may truncate long setlists;
download fallback handles this.

#### Option C — Native iOS Shortcut (no-code, mobile-only)

An Apple Shortcut that:

1. Asks for Artist, Venue, Date via prompts
2. Calls the setlist.fm API (Shortcut HTTP action)
3. Builds the markdown template text in-Shortcut
4. Writes the file to the Obsidian vault folder via Files/iCloud Drive

**Effort:** ~4 hours. Zero code changes to this repository.
**Limitation:** iOS/macOS only; brittle to API changes; no git history.

### Recommended MVP Path

**Phase 1 (v0.1 — this sprint):** Option A — CLI `concert` command.

- Immediately useful for post-show note creation.
- Reuses all existing infrastructure (`fetch.GetSetlist`, artist stub creation,
  vault path resolution).
- Ships as part of `spotify-garden`, already installed and configured.

**Phase 2 (v0.2):** Option B — Web micro-app.

- Adds mobile mid-show capture.
- Hosted as a single static file; no server required for the Obsidian URI
  variant.
- Reuses the same output schema as Phase 1.

**Phase 3 (future):** Option C shortcut for pure iOS users as a companion.

---

## Phase 1 Implementation Detail (`concert` command)

### New command signature

```bash
spotify-garden concert "<Artist>" [--venue "<Venue>"] [--date YYYY-MM-DD] \
    [--city "City, ST"] [--support "<Opener>"] [--notes "..."] [--rating N]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--venue` | `""` | Venue name |
| `--date` | today | Date in YYYY-MM-DD |
| `--city` | `""` | City and state/country |
| `--support` | `""` | Opening act(s) |
| `--notes` | `""` | Freeform notes (or `"-"` to read stdin) |
| `--rating` | `""` | Numeric rating out of 10 |

### Logic

1. Parse flags; artist is the first positional argument.
2. Call `fetch.GetSetlist(artist, date)` — silently continue with empty setlist
   if the show is not yet logged on setlist.fm (mid-show use case).
3. Format venue/city from setlist.fm response if `--venue`/`--city` not given
   by the user.
4. Build markdown note using a template (new file: `templates/concert.md.tmpl`).
5. Resolve output path:
   `{vault}/music/concerts/{YYYY-MM-DD} - {Artist} - {Venue}.md`
6. If the file already exists, print a warning and exit without overwriting
   (same convention as artist stubs).
7. Create an artist stub at `{vault}/music/artists/{Artist}.md` if absent.
8. Write the note (mode `0644`) and print the path to stdout.
9. Print a reminder to re-run after the show to update the setlist if it was
   captured mid-show.

### New files

- `templates/concert.md.tmpl` — Go text/template for the concert note body.
- `internal/render/concert.go` — `WriteConcertNote(opts ConcertNoteOpts)` function.
- `internal/render/concert_test.go` — unit tests for template rendering.

### Tests

- `TestWriteConcertNote_basic` — artist + venue + date + setlist → expected
  frontmatter and body.
- `TestWriteConcertNote_noSetlist` — empty setlist section renders correctly.
- `TestWriteConcertNote_skipExisting` — note is not overwritten when file exists.
- `TestWriteConcertNote_artistStubCreated` — artist stub is created when absent.

---

## Phase 2 Sketch (Web Micro-App)

A single `concert-notes.html` file (or a new `cmd/concert-web/` Go server):

- **Stack:** Vanilla HTML + CSS + JS. Zero build step. No npm.
- **setlist.fm fetch:** Client-side JS `fetch()` to setlist.fm REST API
  (`https://api.setlist.fm/rest/1.0/search/setlists?artistName=&date=`).
  User supplies their own API key in a `<input type="password">` field (stored
  in `localStorage`, never sent server-side).
- **Obsidian URI export:** On submit, construct
  `obsidian://new?vault=<VaultName>&file=music/concerts/<filename>&content=<encoded-md>`
  and open it. Falls back to a `<a download>` link for long notes.
- **Offline-first:** All logic runs client-side; no server required.

Distribute as a file dropped into the vault root or opened directly from disk.

---

## Vault Schema Compatibility

No changes to the existing vault schema are required. The concert note tag
(`music/live-artist/<Artist Name>`) is already referenced in the artist-stub
Dataview query documented in `docs/commands.md`.

The new `concert` command must write notes that are compatible with this tag
so the Dataview block on artist pages picks them up automatically.

---

## Open Questions

1. **setlist.fm mid-show latency:** setlist.fm data for a show often appears
   during or 15–30 min after the show ends. Should the CLI support a
   `spotify-garden concert update "<Artist>" --date YYYY-MM-DD` sub-command
   that re-fetches the setlist and patches an existing note?
2. **Multi-artist nights:** Festivals or multi-band bills — does the note cover
   one artist or the full night? Recommend one note per headliner for v1.
3. **Photo attachment:** Out of scope for v1; the Notes section can hold manual
   `![[photo.jpg]]` wikilinks added post-show.
4. **Obsidian Sync vs. iCloud path:** The Phase 2 web app needs to know the
   vault path. Users on Obsidian Sync may have the vault at
   `~/Library/Mobile Documents/iCloud~md~obsidian/Documents/<VaultName>/`.
   Should this be configurable via `OBSIDIAN_VAULT_PATH` (already in `.env`)?
   Yes — same env var, same resolution logic.

---

## Version Bump

Completing Phase 1 (the `concert` command) constitutes a new major feature and
warrants a **minor version bump** per project policy (e.g., `0.4.0 → 0.5.0`).
