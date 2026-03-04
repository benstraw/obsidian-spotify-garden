# Genre System

**Date:** 2026-03-03

## Summary

Surface Spotify artist genres throughout the Obsidian vault via `[[genre]]` wiki links in weekly/daily notes and populated artist stub frontmatter.

## Changes

### New: `internal/genres/genres.go`
Genre cache (artist ID -> genres) with Load/Save/Update/GenresForPlays/UncachedArtistIDs. Cache stored at `data/genres.json`.

### Modified: `internal/fetch/fetch.go`
Added `GetArtists` (single batch, max 50) and `GetArtistsBatch` (auto-chunking) for `/v1/artists?ids=...` endpoint.

### Modified: `internal/render/render.go`
- `RenderWeekly` and `RenderDaily` accept `artistGenres map[string][]string` parameter
- Weekly notes get "## Genres This Week" section (genre links with play counts, sorted by count)
- Daily notes get "## Genres" section (alphabetical genre links)
- `EnsureArtistStub` now receives genres from cache when available
- New `UpdateArtistGenres` function updates the `genres:` frontmatter line in existing stubs

### Modified: `main.go`
- `runtimePaths` includes `genresPath` with same state dir resolution
- `collect`: fetches genres for uncached artists after saving plays
- `weekly`: loads genre cache, passes to RenderWeekly, updates artist stubs
- `daily`: loads genre cache, passes to RenderDaily, updates artist stubs
- `catch-up`: loads genre cache for daily note generation
- `persona`: updates genre cache from all fetched top artists
- New `genre-backfill` command: seeds cache from all historical plays, updates all artist stubs

## Design Decisions

- **No genre stub pages**: `[[genre]]` links appear as searchable unresolved links in Obsidian graph view
- **Separate cache file**: genres belong to artists not plays; `data/genres.json` mirrors `data/plays.json` pattern
- **Backward compatible**: nil artistGenres omits genre sections entirely
