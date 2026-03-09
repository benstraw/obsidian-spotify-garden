package plays

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/benstraw/spotify-garden/internal/models"
)

func TestMerge_dedup(t *testing.T) {
	existing := []models.Play{
		{PlayedAt: "2026-02-21T10:00:00Z", TrackName: "Track A"},
		{PlayedAt: "2026-02-21T09:00:00Z", TrackName: "Track B"},
	}
	incoming := []models.Play{
		{PlayedAt: "2026-02-21T10:00:00Z", TrackName: "Track A"}, // duplicate
		{PlayedAt: "2026-02-21T11:00:00Z", TrackName: "Track C"}, // new
	}
	result := Merge(existing, incoming)
	if len(result) != 3 {
		t.Errorf("expected 3 plays after merge, got %d", len(result))
	}
}

func TestMerge_sortedDescending(t *testing.T) {
	existing := []models.Play{
		{PlayedAt: "2026-02-21T09:00:00Z"},
	}
	incoming := []models.Play{
		{PlayedAt: "2026-02-21T11:00:00Z"},
		{PlayedAt: "2026-02-21T10:00:00Z"},
	}
	result := Merge(existing, incoming)
	for i := 1; i < len(result); i++ {
		if result[i-1].PlayedAt < result[i].PlayedAt {
			t.Errorf("result not sorted descending at index %d: %s < %s",
				i, result[i-1].PlayedAt, result[i].PlayedAt)
		}
	}
}

func TestMerge_bothEmpty(t *testing.T) {
	result := Merge(nil, nil)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d plays", len(result))
	}
}

func TestMerge_incomingOnly(t *testing.T) {
	incoming := []models.Play{
		{PlayedAt: "2026-02-21T10:00:00Z", TrackName: "Track A"},
	}
	result := Merge(nil, incoming)
	if len(result) != 1 {
		t.Errorf("expected 1 play, got %d", len(result))
	}
}

func TestMerge_existingOnly(t *testing.T) {
	existing := []models.Play{
		{PlayedAt: "2026-02-21T10:00:00Z", TrackName: "Track A"},
	}
	result := Merge(existing, nil)
	if len(result) != 1 || result[0].TrackName != "Track A" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestSaveLoad_roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plays.json")

	original := []models.Play{
		{PlayedAt: "2026-02-21T11:00:00Z", TrackName: "Track B", ArtistName: "Artist 2"},
		{PlayedAt: "2026-02-21T10:00:00Z", TrackName: "Track A", ArtistName: "Artist 1"},
	}

	if err := Save(path, original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(loaded) != len(original) {
		t.Errorf("expected %d plays, got %d", len(original), len(loaded))
	}
	for i, p := range loaded {
		if p.PlayedAt != original[i].PlayedAt || p.TrackName != original[i].TrackName {
			t.Errorf("play %d mismatch: got %+v, want %+v", i, p, original[i])
		}
	}
}

func TestLoad_missingFile(t *testing.T) {
	result, err := Load("/nonexistent/path/plays.json")
	if err != nil {
		t.Errorf("expected no error for missing file, got %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty slice for missing file, got %d plays", len(result))
	}
}

func TestSave_sortedDescending(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plays.json")

	plays := []models.Play{
		{PlayedAt: "2026-02-21T09:00:00Z"},
		{PlayedAt: "2026-02-21T11:00:00Z"},
		{PlayedAt: "2026-02-21T10:00:00Z"},
	}

	if err := Save(path, plays); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(path)
	var loaded []models.Play
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for i := 1; i < len(loaded); i++ {
		if loaded[i-1].PlayedAt < loaded[i].PlayedAt {
			t.Errorf("not sorted descending at index %d", i)
		}
	}
}

// --- Sharded storage tests ---

func TestWeekKey(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"2026-03-09T12:00:00Z", "2026-W11"}, // Mon 2026-03-09 is start of W11
		{"2026-01-01T00:00:00Z", "2026-W01"}, // 2026-01-01 is Thursday, W01
		{"2025-12-29T00:00:00Z", "2026-W01"}, // 2025-12-29 is Mon of the ISO week belonging to 2026
		{"2024-12-31T00:00:00Z", "2025-W01"}, // 2024-12-31 is Tue belonging to 2025-W01
	}
	for _, c := range cases {
		parsed, err := time.Parse(time.RFC3339, c.input)
		if err != nil {
			t.Fatalf("parse %s: %v", c.input, err)
		}
		got := WeekKey(parsed)
		if got != c.want {
			t.Errorf("WeekKey(%s) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestShardedPath(t *testing.T) {
	base := "/data/plays"
	ts, _ := time.Parse(time.RFC3339, "2026-03-09T12:00:00Z")
	got := ShardedPath(base, ts)
	want := filepath.Join(base, "2026", "2026-W11.json")
	if got != want {
		t.Errorf("ShardedPath = %q, want %q", got, want)
	}
}

func TestSaveSharded_createsWeeklyFiles(t *testing.T) {
	baseDir := t.TempDir()

	ps := []models.Play{
		{PlayedAt: "2026-03-09T10:00:00Z", TrackName: "Track A"},
		{PlayedAt: "2026-03-10T10:00:00Z", TrackName: "Track B"},
		{PlayedAt: "2026-03-16T10:00:00Z", TrackName: "Track C"}, // different week (W12)
	}

	added, err := SaveSharded(baseDir, ps)
	if err != nil {
		t.Fatalf("SaveSharded: %v", err)
	}
	if added != 3 {
		t.Errorf("SaveSharded added = %d, want 3", added)
	}

	w11 := filepath.Join(baseDir, "2026", "2026-W11.json")
	w12 := filepath.Join(baseDir, "2026", "2026-W12.json")

	if _, err := os.Stat(w11); err != nil {
		t.Errorf("expected W11 file to exist: %v", err)
	}
	if _, err := os.Stat(w12); err != nil {
		t.Errorf("expected W12 file to exist: %v", err)
	}

	loaded, err := Load(w11)
	if err != nil {
		t.Fatalf("Load w11: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("W11 file has %d plays, want 2", len(loaded))
	}
}

func TestSaveSharded_dedupsOnSecondCall(t *testing.T) {
	baseDir := t.TempDir()

	ps := []models.Play{
		{PlayedAt: "2026-03-09T10:00:00Z", TrackName: "Track A"},
	}

	if _, err := SaveSharded(baseDir, ps); err != nil {
		t.Fatalf("first SaveSharded: %v", err)
	}
	// Call again with the same play plus a new one.
	added, err := SaveSharded(baseDir, []models.Play{
		{PlayedAt: "2026-03-09T10:00:00Z", TrackName: "Track A"}, // duplicate
		{PlayedAt: "2026-03-09T11:00:00Z", TrackName: "Track B"}, // new
	})
	if err != nil {
		t.Fatalf("second SaveSharded: %v", err)
	}
	if added != 1 {
		t.Errorf("second SaveSharded added = %d, want 1", added)
	}
}

func TestLoadSharded_roundtrip(t *testing.T) {
	baseDir := t.TempDir()

	ps := []models.Play{
		{PlayedAt: "2026-03-09T10:00:00Z", TrackName: "Track A"},
		{PlayedAt: "2026-03-16T10:00:00Z", TrackName: "Track B"},
	}

	if _, err := SaveSharded(baseDir, ps); err != nil {
		t.Fatalf("SaveSharded: %v", err)
	}

	loaded, err := LoadSharded(baseDir)
	if err != nil {
		t.Fatalf("LoadSharded: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("LoadSharded returned %d plays, want 2", len(loaded))
	}
	// Verify descending sort.
	if loaded[0].PlayedAt < loaded[1].PlayedAt {
		t.Errorf("LoadSharded result not sorted descending")
	}
}

func TestLoadSharded_missingDir(t *testing.T) {
	result, err := LoadSharded(filepath.Join(t.TempDir(), "nonexistent"))
	if err != nil {
		t.Errorf("expected no error for missing dir, got %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %d plays", len(result))
	}
}

func TestLoadShardedRange(t *testing.T) {
	baseDir := t.TempDir()

	ps := []models.Play{
		{PlayedAt: "2026-03-02T10:00:00Z", TrackName: "W10 Track"},  // 2026-W10
		{PlayedAt: "2026-03-09T10:00:00Z", TrackName: "W11 Track"},  // 2026-W11
		{PlayedAt: "2026-03-16T10:00:00Z", TrackName: "W12 Track"},  // 2026-W12
	}

	if _, err := SaveSharded(baseDir, ps); err != nil {
		t.Fatalf("SaveSharded: %v", err)
	}

	from, _ := time.Parse(time.RFC3339, "2026-03-09T00:00:00Z")
	to, _ := time.Parse(time.RFC3339, "2026-03-15T23:59:59Z")

	result, err := LoadShardedRange(baseDir, from, to)
	if err != nil {
		t.Fatalf("LoadShardedRange: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("LoadShardedRange returned %d plays, want 1", len(result))
	}
	if len(result) > 0 && result[0].TrackName != "W11 Track" {
		t.Errorf("LoadShardedRange got %q, want W11 Track", result[0].TrackName)
	}
}

func TestMigrateToSharded(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "plays.json")
	baseDir := filepath.Join(dir, "plays")

	ps := []models.Play{
		{PlayedAt: "2026-03-09T10:00:00Z", TrackName: "Track A"},
		{PlayedAt: "2026-03-16T10:00:00Z", TrackName: "Track B"},
	}
	if err := Save(legacyPath, ps); err != nil {
		t.Fatalf("Save legacy: %v", err)
	}

	if err := MigrateToSharded(legacyPath, baseDir); err != nil {
		t.Fatalf("MigrateToSharded: %v", err)
	}

	// Legacy file should be renamed to .bak.
	if _, err := os.Stat(legacyPath); err == nil {
		t.Error("legacy plays.json should have been renamed to .bak")
	}
	if _, err := os.Stat(legacyPath + ".bak"); err != nil {
		t.Errorf("expected plays.json.bak to exist: %v", err)
	}

	// Sharded files should contain all plays.
	loaded, err := LoadSharded(baseDir)
	if err != nil {
		t.Fatalf("LoadSharded after migration: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("LoadSharded after migration: got %d plays, want 2", len(loaded))
	}
}

func TestMigrateToSharded_idempotent(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "plays.json")
	baseDir := filepath.Join(dir, "plays")

	ps := []models.Play{
		{PlayedAt: "2026-03-09T10:00:00Z", TrackName: "Track A"},
	}
	if err := Save(legacyPath, ps); err != nil {
		t.Fatalf("Save legacy: %v", err)
	}

	if err := MigrateToSharded(legacyPath, baseDir); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	// Create legacy file again (simulate accidental recreation).
	if err := Save(legacyPath, ps); err != nil {
		t.Fatalf("re-save legacy: %v", err)
	}
	// Second call should be a no-op because .bak already exists.
	if err := MigrateToSharded(legacyPath, baseDir); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Error("second migrate should not have moved the legacy file again")
	}
}
