# Plan: Sharded Plays Storage

**Date:** 2026-03-09  
**Status:** Implemented in `v0.5.0`

## Problem

`data/plays.json` is a single append-only JSON array. At 5 collects/day × ~50 plays each, a year of history can easily exceed several MB. Loading the full file on every command grows in cost proportionally with history depth, and the file becomes cumbersome to inspect or manage manually.

## Decision

Write plays directly to annual/weekly shard files at collection time. No scheduled split job is needed.

**Chosen layout:**
```
data/plays/
  2025/
    2025-W01.json
    2025-W52.json
  2026/
    2026-W10.json
    2026-W11.json
```

- One file per ISO week (`YYYY-WNN.json`). No file is created for weeks with no plays.
- Annual subdirectories keep each year's files grouped.
- Each weekly file is a JSON array sorted descending by `played_at` — same format as the legacy file, just scoped to one week.

## Alternatives Considered

1. **Write to flat file, batch-split on a weekly schedule** — More complex scheduler logic; splitting requires a lock/rename dance; no benefit over direct sharding.
2. **Monthly files** — Better granularity than one file but worse than weekly; weekly aligns naturally with the weekly note cadence.
3. **SQLite** — Would be an external dependency and breaks the "stdlib only" constraint.

## Timezone Handling

All shard key calculations use **UTC** — Spotify timestamps are already UTC (`played_at` is RFC3339/UTC). Week assignment is therefore deterministic regardless of where the binary runs. Display and filtering for notes continues to use the local timezone (existing behaviour), so play logs display in the user's timezone while storage is timezone-agnostic.

ISO week year (`time.ISOWeek()`) is used to handle Dec 31 / Jan 1 boundaries correctly (e.g. 2025-12-29 belongs to 2026-W01).

## Migration

On the first `collect` run after upgrade:
1. `plays.MigrateToSharded(legacyPath, baseDir)` detects `data/plays.json`.
2. Reads all existing plays and writes them into the sharded structure.
3. Renames `data/plays.json` → `data/plays.json.bak` atomically (via `os.Rename`).
4. Subsequent runs skip migration because `.bak` already exists.

The `.bak` file is kept as a safety net. Users can delete it once they are satisfied with the migration.

## New API (`internal/plays`)

| Function | Description |
|---|---|
| `WeekKey(t time.Time) string` | ISO year-week string: `"2026-W10"` |
| `ShardedPath(baseDir string, t time.Time) string` | `baseDir/YYYY/YYYY-WNN.json` |
| `LoadSharded(baseDir string)` | Walk full tree, return all plays sorted descending |
| `LoadShardedRange(baseDir string, from, to time.Time)` | Load only weeks overlapping a UTC date range |
| `SaveSharded(baseDir string, incoming []Play) (int, error)` | Route to week files, merge+dedup per file, return new count |
| `MigrateToSharded(legacyPath, baseDir string) error` | One-shot migration; idempotent (no-op if `.bak` exists or legacy absent) |

## `runtimePaths` Change

`playsDir` is derived from `filepath.Dir(playsPath) + "/plays"`. It inherits the same `SPOTIFY_STATE_DIR` override logic as `playsPath` — no new env var needed.

## Tests Added

- `TestWeekKey` — ISO week year edge cases (Dec 31 / Jan 1 boundary)
- `TestShardedPath` — path layout
- `TestSaveSharded_createsWeeklyFiles` — routing, file creation
- `TestSaveSharded_dedupsOnSecondCall` — merge/dedup per-week file
- `TestLoadSharded_roundtrip` — save + load all
- `TestLoadSharded_missingDir` — no error on absent dir
- `TestLoadShardedRange` — range scoping
- `TestMigrateToSharded` — migration + rename to `.bak`
- `TestMigrateToSharded_idempotent` — second call is a no-op
- `TestResolveRuntimePaths_*` updated to verify `playsDir` value
