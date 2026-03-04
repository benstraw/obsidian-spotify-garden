package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/benstraw/spotify-garden/internal/models"
)

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func canonicalPath(t *testing.T, p string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(p)
	if err == nil {
		return resolved
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

func TestParseDate_UsesLocalTimezone(t *testing.T) {
	origLocal := time.Local
	time.Local = time.FixedZone("TEST", -8*3600)
	t.Cleanup(func() { time.Local = origLocal })

	got, err := parseDate("2026-02-21")
	if err != nil {
		t.Fatalf("parseDate returned error: %v", err)
	}

	if got.Year() != 2026 || got.Month() != 2 || got.Day() != 21 {
		t.Fatalf("parseDate returned wrong date: %s", got.Format("2006-01-02"))
	}
	if got.Location() != time.Local {
		t.Fatalf("parseDate location = %s, want %s", got.Location(), time.Local)
	}
	if got.Hour() != 0 || got.Minute() != 0 || got.Second() != 0 {
		t.Fatalf("parseDate expected local midnight, got %v", got)
	}
}

func TestParseDate_Invalid(t *testing.T) {
	if _, err := parseDate("2026/02/21"); err == nil {
		t.Fatal("expected parseDate to fail for invalid format")
	}
}

func TestGenerateDailyNote_CreatesArtistStub(t *testing.T) {
	vault := t.TempDir()
	t.Setenv("OBSIDIAN_VAULT_PATH", vault)

	loc := time.Local
	date := time.Date(2026, 2, 22, 12, 0, 0, 0, loc)
	plays := []models.Play{
		{
			PlayedAt:         time.Date(2026, 2, 22, 9, 0, 0, 0, loc).UTC().Format(time.RFC3339),
			TrackName:        "Song",
			ArtistName:       "Artist One",
			AlbumName:        "Album",
			DurationMS:       180000,
			ArtistSpotifyURL: "https://open.spotify.com/artist/abc",
		},
	}

	generateDailyNote(plays, date, false, nil)

	dailyPath := filepath.Join(vault, "music", "listening", "spotify-2026-02-22.md")
	if _, err := os.Stat(dailyPath); err != nil {
		t.Fatalf("expected daily note to be written: %v", err)
	}

	stubPath := filepath.Join(vault, "music", "artists", "Artist One.md")
	if _, err := os.Stat(stubPath); err != nil {
		t.Fatalf("expected artist stub to be written: %v", err)
	}
}

func TestGenerateDailyNote_SkipExistingNoteStillCreatesStub(t *testing.T) {
	vault := t.TempDir()
	t.Setenv("OBSIDIAN_VAULT_PATH", vault)

	listeningDir := filepath.Join(vault, "music", "listening")
	if err := os.MkdirAll(listeningDir, 0755); err != nil {
		t.Fatalf("mkdir listening dir: %v", err)
	}
	existingDaily := filepath.Join(listeningDir, "spotify-2026-02-22.md")
	if err := os.WriteFile(existingDaily, []byte("existing"), 0644); err != nil {
		t.Fatalf("write existing daily: %v", err)
	}

	loc := time.Local
	date := time.Date(2026, 2, 22, 12, 0, 0, 0, loc)
	plays := []models.Play{
		{
			PlayedAt:         time.Date(2026, 2, 22, 9, 0, 0, 0, loc).UTC().Format(time.RFC3339),
			TrackName:        "Song",
			ArtistName:       "Artist Two",
			AlbumName:        "Album",
			DurationMS:       180000,
			ArtistSpotifyURL: "https://open.spotify.com/artist/def",
		},
	}

	generateDailyNote(plays, date, false, nil)

	stubPath := filepath.Join(vault, "music", "artists", "Artist Two.md")
	if _, err := os.Stat(stubPath); err != nil {
		t.Fatalf("expected artist stub to be written even when daily note exists: %v", err)
	}
}

func TestResolveRuntimePaths_StateDirPreferred(t *testing.T) {
	cwd := t.TempDir()
	stateDir := t.TempDir()

	mustWriteFile(t, filepath.Join(cwd, ".env"), "SPOTIFY_CLIENT_ID=cwd\n")
	mustWriteFile(t, filepath.Join(cwd, "tokens.json"), `{"access_token":"cwd","refresh_token":"cwd","expires_at":"2026-01-01T00:00:00Z"}`)
	mustWriteFile(t, filepath.Join(cwd, "data", "plays.json"), "[]")

	mustWriteFile(t, filepath.Join(stateDir, ".env"), "SPOTIFY_CLIENT_ID=state\n")
	mustWriteFile(t, filepath.Join(stateDir, "tokens.json"), `{"access_token":"state","refresh_token":"state","expires_at":"2026-01-01T00:00:00Z"}`)
	mustWriteFile(t, filepath.Join(stateDir, "data", "plays.json"), "[]")
	mustWriteFile(t, filepath.Join(stateDir, "data", "genres.json"), "{}")

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir cwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	t.Setenv("SPOTIFY_STATE_DIR", stateDir)

	paths := resolveRuntimePaths()

	if canonicalPath(t, paths.dotEnvPath) != canonicalPath(t, filepath.Join(stateDir, ".env")) {
		t.Fatalf("dotEnvPath = %s, want %s", paths.dotEnvPath, filepath.Join(stateDir, ".env"))
	}
	if canonicalPath(t, paths.tokensPath) != canonicalPath(t, filepath.Join(stateDir, "tokens.json")) {
		t.Fatalf("tokensPath = %s, want %s", paths.tokensPath, filepath.Join(stateDir, "tokens.json"))
	}
	if canonicalPath(t, paths.playsPath) != canonicalPath(t, filepath.Join(stateDir, "data", "plays.json")) {
		t.Fatalf("playsPath = %s, want %s", paths.playsPath, filepath.Join(stateDir, "data", "plays.json"))
	}
	if canonicalPath(t, paths.genresPath) != canonicalPath(t, filepath.Join(stateDir, "data", "genres.json")) {
		t.Fatalf("genresPath = %s, want %s", paths.genresPath, filepath.Join(stateDir, "data", "genres.json"))
	}
	if paths.dotEnvFallback || paths.tokensFallback || paths.playsFallback || paths.genresFallback {
		t.Fatalf("unexpected fallback flags: %+v", paths)
	}
}

func TestResolveRuntimePaths_StateDirFallbackToCWD(t *testing.T) {
	cwd := t.TempDir()
	stateDir := t.TempDir()

	mustWriteFile(t, filepath.Join(cwd, ".env"), "SPOTIFY_CLIENT_ID=cwd\n")
	mustWriteFile(t, filepath.Join(cwd, "tokens.json"), `{"access_token":"cwd","refresh_token":"cwd","expires_at":"2026-01-01T00:00:00Z"}`)
	mustWriteFile(t, filepath.Join(cwd, "data", "plays.json"), "[]")

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir cwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	t.Setenv("SPOTIFY_STATE_DIR", stateDir)

	paths := resolveRuntimePaths()

	if canonicalPath(t, paths.dotEnvPath) != canonicalPath(t, filepath.Join(cwd, ".env")) {
		t.Fatalf("dotEnvPath = %s, want %s", paths.dotEnvPath, filepath.Join(cwd, ".env"))
	}
	if canonicalPath(t, paths.tokensPath) != canonicalPath(t, filepath.Join(cwd, "tokens.json")) {
		t.Fatalf("tokensPath = %s, want %s", paths.tokensPath, filepath.Join(cwd, "tokens.json"))
	}
	if canonicalPath(t, paths.playsPath) != canonicalPath(t, filepath.Join(cwd, "data", "plays.json")) {
		t.Fatalf("playsPath = %s, want %s", paths.playsPath, filepath.Join(cwd, "data", "plays.json"))
	}
	if !paths.dotEnvFallback || !paths.tokensFallback || !paths.playsFallback {
		t.Fatalf("expected fallback flags to be true: %+v", paths)
	}
}

func TestLoadDotEnv_MissingIsOptional(t *testing.T) {
	if err := loadDotEnv(filepath.Join(t.TempDir(), ".env")); err != nil {
		t.Fatalf("loadDotEnv returned error for missing file: %v", err)
	}
}
