# Repository Guidelines

## Project Structure & Module Organization
- `main.go`: CLI entrypoint and command wiring (`auth`, `collect`, `weekly`, `catch-up`, `persona`, `setlist`).
- `internal/`: core packages by responsibility (`auth/`, `client/`, `fetch/`, `plays/`, `render/`, `models/`). Keep business logic here, not in `main.go`.
- `templates/`: markdown templates used for note generation.
- `data/`: local runtime data (for example `data/plays.json`, git-ignored).
- `docs/`: command, architecture, and auth-flow reference docs.
- Root scripts (`run_collect_spotify.sh`, `run_weekly_spotify.sh`) support launchd automation.

## Build, Test, and Development Commands
- `go build -o spotify-garden .`: build local CLI binary.
- `go vet ./...`: run static checks used by CI.
- `go test ./...`: run all unit tests.
- `./spotify-garden auth`: complete OAuth flow and write `tokens.json`.
- `./spotify-garden collect` / `weekly` / `catch-up --weeks 8` / `persona`: core runtime workflows.

## Coding Style & Naming Conventions
- Follow standard Go formatting: run `gofmt` on changed files before committing.
- Use tabs for indentation (Go defaults); exported identifiers in `CamelCase`, internal helpers in `camelCase`.
- Keep packages focused and small; place tests next to implementation as `*_test.go`.
- Prefer explicit, wrapped errors (e.g., `fmt.Errorf("context: %w", err)`).

## Testing Guidelines
- Framework: Go `testing` package (no external test deps).
- Naming: `Test<Function>_<scenario>` (examples in `internal/render/render_test.go`).
- Add/extend tests for any behavior change, especially date boundaries, dedup logic, and rendering output.
- Minimum pre-PR check: `go vet ./... && go test ./... && go build -o spotify-garden .`.

## Commit & Pull Request Guidelines
- Commit style in history is concise, imperative, and scoped (e.g., `Add unit tests, CI workflow...`, `v0.2.0: add setlist command...`).
- Keep commits focused; use clear subjects that describe user-visible behavior.
- PRs should include: purpose, key changes, test evidence (commands + results), and linked issue/context.
- For output/template changes, include a short sample of generated markdown in the PR description.

## Security & Configuration Tips
- Never commit `.env`, `tokens.json`, or `data/plays.json`.
- Use `.env.example` as the source of required variables.
- Ensure `SPOTIFY_REDIRECT_URI` and local callback port settings match your Spotify app config.
