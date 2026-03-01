package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/benstraw/spotify-garden/internal/models"
)

// --- TagSlug ---

func TestTagSlug(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"The Beatles", "the-beatles"},
		{"Guns N' Roses", "guns-n-roses"},
		{"Madison Square Garden", "madison-square-garden"},
		{"AC/DC", "ac-dc"},
		{"Orla Gartland", "orla-gartland"},
		{"Madison Square Garden", "madison-square-garden"},
		{"St. Vincent", "st-vincent"},
		{"P!nk", "p-nk"},
		{"Smashing Pumpkins", "smashing-pumpkins"},
		{"simple", "simple"},
		{"already-slug", "already-slug"},
		{"  extra spaces  ", "extra-spaces"},
	}
	for _, c := range cases {
		got := TagSlug(c.input)
		if got != c.want {
			t.Errorf("TagSlug(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// --- WeekStr ---

func TestWeekStr(t *testing.T) {
	cases := []struct {
		date string
		want string
	}{
		{"2026-02-16", "2026-W08"}, // Monday
		{"2026-02-22", "2026-W08"}, // Sunday
		{"2026-12-28", "2026-W53"}, // last week of 2026
		{"2026-01-01", "2026-W01"}, // first week of 2026
		{"2024-12-30", "2025-W01"}, // ISO year differs from calendar year
	}
	for _, c := range cases {
		d, _ := time.Parse("2006-01-02", c.date)
		got := WeekStr(d)
		if got != c.want {
			t.Errorf("WeekStr(%s) = %q, want %q", c.date, got, c.want)
		}
	}
}

// --- WeekBounds ---

// localNoon constructs a local-time date at noon to avoid UTC→local day shifts in tests.
func localNoon(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 12, 0, 0, 0, time.Local)
}

func TestWeekBounds_monday(t *testing.T) {
	// Monday — should be its own week start
	d := localNoon(2026, 2, 16)
	monday, next := WeekBounds(d)
	if monday.Format("2006-01-02") != "2026-02-16" {
		t.Errorf("monday = %s, want 2026-02-16", monday.Format("2006-01-02"))
	}
	if next.Format("2006-01-02") != "2026-02-23" {
		t.Errorf("nextMonday = %s, want 2026-02-23", next.Format("2006-01-02"))
	}
}

func TestWeekBounds_sunday(t *testing.T) {
	// Sunday is treated as weekday 7, so same week as previous Monday
	d := localNoon(2026, 2, 22)
	monday, next := WeekBounds(d)
	if monday.Format("2006-01-02") != "2026-02-16" {
		t.Errorf("monday = %s, want 2026-02-16", monday.Format("2006-01-02"))
	}
	if next.Format("2006-01-02") != "2026-02-23" {
		t.Errorf("nextMonday = %s, want 2026-02-23", next.Format("2006-01-02"))
	}
}

func TestWeekBounds_wednesday(t *testing.T) {
	d := localNoon(2026, 2, 18)
	monday, next := WeekBounds(d)
	if monday.Format("2006-01-02") != "2026-02-16" {
		t.Errorf("monday = %s, want 2026-02-16", monday.Format("2006-01-02"))
	}
	if next.Format("2006-01-02") != "2026-02-23" {
		t.Errorf("nextMonday = %s, want 2026-02-23", next.Format("2006-01-02"))
	}
}

func TestWeekBounds_midnight(t *testing.T) {
	// Bounds should be at midnight local
	d := localNoon(2026, 2, 18)
	monday, next := WeekBounds(d)
	if monday.Hour() != 0 || monday.Minute() != 0 || monday.Second() != 0 {
		t.Errorf("monday not at midnight: %v", monday)
	}
	if next.Hour() != 0 || next.Minute() != 0 || next.Second() != 0 {
		t.Errorf("nextMonday not at midnight: %v", next)
	}
}

// --- PlaysForWeek ---

func TestPlaysForWeek_filtersCorrectly(t *testing.T) {
	plays := []models.Play{
		{PlayedAt: "2026-02-16T12:00:00Z"}, // week W08 (UTC Mon)
		{PlayedAt: "2026-02-20T12:00:00Z"}, // week W08
		{PlayedAt: "2026-02-23T12:00:00Z"}, // week W09
		{PlayedAt: "2026-02-09T12:00:00Z"}, // week W07
	}
	// Use a UTC-based date to avoid timezone flakiness in CI
	d := time.Date(2026, 2, 18, 0, 0, 0, 0, time.UTC)
	result := PlaysForWeek(plays, d)

	// All results must fall within W08
	for _, p := range result {
		t2, _ := parsePlayedAt(p.PlayedAt)
		week := WeekStr(t2.Local())
		monday, _ := WeekBounds(d)
		wantWeek := WeekStr(monday)
		if week != wantWeek {
			t.Errorf("play %s in wrong week %s (want %s)", p.PlayedAt, week, wantWeek)
		}
	}
}

func TestPlaysForWeek_sortedAscending(t *testing.T) {
	plays := []models.Play{
		{PlayedAt: "2026-02-20T12:00:00Z"},
		{PlayedAt: "2026-02-16T08:00:00Z"},
		{PlayedAt: "2026-02-18T10:00:00Z"},
	}
	d := time.Date(2026, 2, 18, 0, 0, 0, 0, time.UTC)
	result := PlaysForWeek(plays, d)
	for i := 1; i < len(result); i++ {
		if result[i-1].PlayedAt > result[i].PlayedAt {
			t.Errorf("not sorted ascending at index %d: %s > %s",
				i, result[i-1].PlayedAt, result[i].PlayedAt)
		}
	}
}

func TestPlaysForWeek_empty(t *testing.T) {
	d := time.Date(2026, 2, 18, 0, 0, 0, 0, time.UTC)
	result := PlaysForWeek(nil, d)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d plays", len(result))
	}
}

func TestPlaysForWeek_millisecondsFormat(t *testing.T) {
	// Spotify returns played_at with milliseconds
	plays := []models.Play{
		{PlayedAt: "2026-02-18T12:00:00.000Z"},
	}
	d := time.Date(2026, 2, 18, 0, 0, 0, 0, time.UTC)
	result := PlaysForWeek(plays, d)
	if len(result) == 0 {
		t.Error("expected play with millisecond timestamp to be included")
	}
}

// --- fmtDuration ---

func TestFmtDuration(t *testing.T) {
	cases := []struct {
		ms   int
		want string
	}{
		{0, "0min"},
		{60000, "1min"},
		{3600000, "1h 0min"},
		{3660000, "1h 1min"},
		{9000000, "2h 30min"},
		{59999, "0min"},    // rounds down
		{3599999, "59min"}, // just under 1h
	}
	for _, c := range cases {
		got := fmtDuration(c.ms)
		if got != c.want {
			t.Errorf("fmtDuration(%d) = %q, want %q", c.ms, got, c.want)
		}
	}
}

// --- parsePlayedAt ---

func TestParsePlayedAt_RFC3339(t *testing.T) {
	ts := "2026-02-21T14:30:00Z"
	got, err := parsePlayedAt(ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Year() != 2026 || got.Month() != 2 || got.Day() != 21 {
		t.Errorf("wrong date: %v", got)
	}
}

func TestParsePlayedAt_Milliseconds(t *testing.T) {
	ts := "2026-02-21T14:30:00.000Z"
	got, err := parsePlayedAt(ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Year() != 2026 || got.Month() != 2 || got.Day() != 21 {
		t.Errorf("wrong date: %v", got)
	}
}

func TestParsePlayedAt_Invalid(t *testing.T) {
	_, err := parsePlayedAt("not-a-timestamp")
	if err == nil {
		t.Error("expected error for invalid timestamp")
	}
}

// --- RenderWeekly (smoke test, no vault I/O) ---

func TestRenderWeekly_noPlays(t *testing.T) {
	dir := t.TempDir()
	date := time.Date(2026, 2, 18, 0, 0, 0, 0, time.UTC)

	content, err := RenderWeekly(nil, date, dir)
	if err != nil {
		t.Fatalf("RenderWeekly: %v", err)
	}

	checks := []string{
		"type: note",
		"tags: [music, weekly-music]",
		"# Week in Music:",
		"## Stats",
		"## Notes",
	}
	for _, s := range checks {
		if !strings.Contains(content, s) {
			t.Errorf("output missing %q", s)
		}
	}
}

func TestRenderWeekly_noPlayLog(t *testing.T) {
	dir := t.TempDir()
	date := time.Date(2026, 2, 18, 0, 0, 0, 0, time.UTC)
	content, err := RenderWeekly(nil, date, dir)
	if err != nil {
		t.Fatalf("RenderWeekly: %v", err)
	}
	if strings.Contains(content, "## Play Log") {
		t.Error("weekly note should not contain ## Play Log")
	}
}

func TestRenderWeekly_withPlays(t *testing.T) {
	dir := t.TempDir()
	date := time.Date(2026, 2, 18, 0, 0, 0, 0, time.UTC)

	plays := []models.Play{
		{
			PlayedAt:   "2026-02-18T12:00:00Z",
			TrackName:  "My Song",
			ArtistName: "My Artist",
			AlbumName:  "My Album",
			DurationMS: 200000,
		},
		{
			PlayedAt:   "2026-02-18T12:05:00Z",
			TrackName:  "My Song",
			ArtistName: "My Artist",
			AlbumName:  "My Album",
			DurationMS: 200000,
		},
	}
	content, err := RenderWeekly(plays, date, dir)
	if err != nil {
		t.Fatalf("RenderWeekly: %v", err)
	}

	checks := []string{
		"[[My Artist]]",
		"My Song",
		"My Album",
		"## Repeated Tracks",
		"## Albums This Week",
		"## Artists in Rotation",
	}
	for _, s := range checks {
		if !strings.Contains(content, s) {
			t.Errorf("output missing %q", s)
		}
	}
}

// --- PlaysForDay ---

func TestPlaysForDay_empty(t *testing.T) {
	d := time.Date(2026, 2, 22, 12, 0, 0, 0, time.Local)
	result := PlaysForDay(nil, d)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d plays", len(result))
	}
}

func TestPlaysForDay_filtersCorrectly(t *testing.T) {
	loc := time.Local
	plays := []models.Play{
		{PlayedAt: time.Date(2026, 2, 22, 9, 0, 0, 0, loc).UTC().Format(time.RFC3339)},
		{PlayedAt: time.Date(2026, 2, 22, 21, 0, 0, 0, loc).UTC().Format(time.RFC3339)},
		{PlayedAt: time.Date(2026, 2, 21, 23, 59, 0, 0, loc).UTC().Format(time.RFC3339)}, // previous day
		{PlayedAt: time.Date(2026, 2, 23, 0, 0, 0, 0, loc).UTC().Format(time.RFC3339)},   // next day
	}
	d := time.Date(2026, 2, 22, 12, 0, 0, 0, loc)
	result := PlaysForDay(plays, d)
	if len(result) != 2 {
		t.Errorf("expected 2 plays, got %d", len(result))
	}
}

func TestPlaysForDay_sortedAscending(t *testing.T) {
	loc := time.Local
	plays := []models.Play{
		{PlayedAt: time.Date(2026, 2, 22, 15, 0, 0, 0, loc).UTC().Format(time.RFC3339)},
		{PlayedAt: time.Date(2026, 2, 22, 9, 0, 0, 0, loc).UTC().Format(time.RFC3339)},
		{PlayedAt: time.Date(2026, 2, 22, 12, 0, 0, 0, loc).UTC().Format(time.RFC3339)},
	}
	d := time.Date(2026, 2, 22, 12, 0, 0, 0, loc)
	result := PlaysForDay(plays, d)
	for i := 1; i < len(result); i++ {
		if result[i-1].PlayedAt > result[i].PlayedAt {
			t.Errorf("not sorted ascending at index %d: %s > %s",
				i, result[i-1].PlayedAt, result[i].PlayedAt)
		}
	}
}

// --- RenderDaily ---

func TestRenderDaily_noPlays(t *testing.T) {
	dir := t.TempDir()
	date := time.Date(2026, 2, 22, 12, 0, 0, 0, time.Local)
	content, err := RenderDaily(nil, date, dir)
	if err != nil {
		t.Fatalf("RenderDaily: %v", err)
	}
	if content != "" {
		t.Errorf("expected empty string for no plays, got non-empty content")
	}
}

func TestRenderDaily_withPlays(t *testing.T) {
	dir := t.TempDir()
	loc := time.Local
	date := time.Date(2026, 2, 22, 12, 0, 0, 0, loc)
	plays := []models.Play{
		{
			PlayedAt:   time.Date(2026, 2, 22, 9, 3, 0, 0, loc).UTC().Format(time.RFC3339),
			TrackName:  "Souvenirs",
			ArtistName: "Orla Gartland",
			AlbumName:  "Woman on the Internet",
			DurationMS: 210000,
		},
		{
			PlayedAt:   time.Date(2026, 2, 22, 9, 7, 0, 0, loc).UTC().Format(time.RFC3339),
			TrackName:  "Madison",
			ArtistName: "Orla Gartland",
			AlbumName:  "Woman on the Internet",
			DurationMS: 180000,
		},
	}
	content, err := RenderDaily(plays, date, dir)
	if err != nil {
		t.Fatalf("RenderDaily: %v", err)
	}
	checks := []string{
		"type: note",
		"tags: [music, daily-music]",
		"date: 2026-02-22",
		"# Daily Listening: 2026-02-22",
		"## Stats",
		"## Play Log",
		"## Songs Played",
		"## Artists Played",
		"## Albums Played",
		"Souvenirs",
		"[[Orla Gartland]]",
		"_(Woman on the Internet)_",
		"## Notes",
	}
	for _, s := range checks {
		if !strings.Contains(content, s) {
			t.Errorf("output missing %q", s)
		}
	}
}

// --- EnsureArtistStub tag slug ---

func TestEnsureArtistStub_tagSlugInFrontmatter(t *testing.T) {
	dir := t.TempDir()

	// Artist with spaces and apostrophe
	err := EnsureArtistStub("Guns N' Roses", "https://open.spotify.com/artist/123", nil, "2026-03-01", dir)
	if err != nil {
		t.Fatalf("EnsureArtistStub: %v", err)
	}

	data, readErr := os.ReadFile(filepath.Join(dir, "music", "artists", "Guns N' Roses.md"))
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	content := string(data)

	checks := []string{
		"live_artist_tag: music/live-artist/guns-n-roses",
		"WHERE contains(tags, this.live_artist_tag)",
	}
	for _, s := range checks {
		if !strings.Contains(content, s) {
			t.Errorf("artist stub missing %q", s)
		}
	}
	// The old broken pattern must not be present
	if strings.Contains(content, `"music/live-artist/" + this.file.name`) {
		t.Error("artist stub still contains old broken tag pattern")
	}
}
