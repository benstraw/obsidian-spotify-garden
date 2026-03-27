# Plan: Runtime Diagnostics, State-Dir Unification, Loop Refactor, and Governance

## Summary

Implement four coordinated improvements:

1. Add `doctor` command for runtime diagnostics.
2. Introduce `MUSIC_STATE_DIR` as unified state root for `.env`, `tokens.json`, and `data/plays.json`.
3. Refactor duplicated orchestration loops in `main.go`.
4. Add process policy to persist major-feature plans in `docs/plans/` and require a minor version bump when major features are completed.

## Runtime Path Contract

Apply this precedence for `auth`, `collect`, `weekly`, `daily`, `catch-up`, `persona`, `setlist`, and `doctor`:

1. CLI flags (where applicable)
2. Environment variables
3. `MUSIC_STATE_DIR` files
4. CWD fallback with warning

## Public Interface Changes

1. New command: `music-garden doctor`
2. New env var: `MUSIC_STATE_DIR`
3. Governance: major-feature plans saved to `docs/plans/`, completed major features require minor version bump

## Implementation Detail

### Runtime resolution

- Add centralized runtime path resolution in `main.go` for:
  - state dir
  - dotenv path
  - tokens path
  - plays path
- Use fallback-to-CWD behavior only when state-dir file is missing.
- Emit warnings when fallback is used while `MUSIC_STATE_DIR` is set.

### Auth token path parameterization

- Update `internal/auth` to take explicit token path:
  - `StartAuthFlow(tokensPath string)`
  - `RefreshIfNeeded(tokensPath string)`
  - `SaveTokens(tokensPath, tokens)`
  - `LoadTokens(tokensPath)`

### Doctor command

Print:

- working directory and executable path
- effective state dir and runtime file paths
- templates and output-vault paths
- fallback warnings
- launchd labels/log paths (derived from installer defaults)
- best-effort launchd loaded/not-loaded status

Exit nonzero when issues are found.

### Refactor duplication

Extract helpers in `main.go` for:

- weekly note path resolution
- daily note path resolution
- missing-week/missing-day detection
- generation loops in catch-up flow

## Tests and Validation

- Add/update tests for path resolution and optional `.env` behavior.
- Add auth token-path tests.
- Run:
  - `go test ./...`
  - `go vet ./...`
  - `go build -o music-garden .`

## Defaults and Assumptions

- `MUSIC_STATE_DIR` is canonical runtime root when configured.
- CWD fallback is temporary compatibility behavior and always warns.
- `doctor` output is human-readable.
- No `claude.md` currently exists; policy is recorded in `AGENTS.md` with future mirroring instruction.
