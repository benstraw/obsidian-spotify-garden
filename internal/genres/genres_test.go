package genres

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/benstraw/spotify-garden/internal/models"
)

func TestLoad_missingFile(t *testing.T) {
	cache, err := Load("/nonexistent/genres.json")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(cache) != 0 {
		t.Errorf("expected empty map, got %d entries", len(cache))
	}
}

func TestSaveLoad_roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genres.json")

	cache := map[string]Entry{
		"abc123": {Name: "Radiohead", Genres: []string{"alternative rock", "art rock"}, LastUpdated: "2026-03-01"},
	}

	if err := Save(path, cache); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(loaded) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(loaded))
	}
	entry := loaded["abc123"]
	if entry.Name != "Radiohead" {
		t.Errorf("name = %q, want Radiohead", entry.Name)
	}
	if len(entry.Genres) != 2 || entry.Genres[0] != "alternative rock" {
		t.Errorf("genres = %v, want [alternative rock, art rock]", entry.Genres)
	}
}

func TestSave_indented(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "genres.json")

	cache := map[string]Entry{
		"x": {Name: "Test", Genres: []string{"rock"}, LastUpdated: "2026-01-01"},
	}
	if err := Save(path, cache); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, _ := os.ReadFile(path)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("saved JSON is invalid: %v", err)
	}
}

func TestUpdate(t *testing.T) {
	cache := map[string]Entry{}
	Update(cache, "id1", "Artist One", []string{"pop", "dance"}, nil)

	entry, ok := cache["id1"]
	if !ok {
		t.Fatal("entry not found after Update")
	}
	if entry.Name != "Artist One" {
		t.Errorf("name = %q, want Artist One", entry.Name)
	}
	if len(entry.Genres) != 2 {
		t.Errorf("genres len = %d, want 2", len(entry.Genres))
	}
	if entry.LastUpdated == "" {
		t.Error("LastUpdated should be set")
	}
}

func TestUpdate_overwrites(t *testing.T) {
	cache := map[string]Entry{
		"id1": {Name: "Old", Genres: []string{"old-genre"}, LastUpdated: "2025-01-01"},
	}
	Update(cache, "id1", "New", []string{"new-genre"}, nil)

	if cache["id1"].Name != "New" {
		t.Errorf("name = %q, want New", cache["id1"].Name)
	}
	if cache["id1"].Genres[0] != "new-genre" {
		t.Errorf("genre = %q, want new-genre", cache["id1"].Genres[0])
	}
}

func TestUpdateImages_preservesGenresAndReplacesImages(t *testing.T) {
	cache := map[string]Entry{
		"id1": {
			Name:   "Artist One",
			Genres: []string{"ambient", "electronic"},
			Images: []models.ArtistImage{{URL: "https://old", Height: 64, Width: 64}},
		},
	}

	newImages := []models.ArtistImage{
		{URL: "https://img-1", Height: 640, Width: 640},
		{URL: "https://img-2", Height: 320, Width: 320},
	}
	UpdateImages(cache, "id1", newImages)

	entry := cache["id1"]
	if len(entry.Genres) != 2 || entry.Genres[0] != "ambient" {
		t.Fatalf("genres changed unexpectedly: %v", entry.Genres)
	}
	if len(entry.Images) != 2 || entry.Images[0].URL != "https://img-1" {
		t.Fatalf("images = %+v, want replacement images", entry.Images)
	}
	if entry.LastUpdated == "" {
		t.Fatal("LastUpdated should be set")
	}
}

func TestMissingImagesArtistIDs(t *testing.T) {
	cache := map[string]Entry{
		"b": {Name: "Has images", Images: []models.ArtistImage{{URL: "https://img"}}},
		"a": {Name: "Missing images"},
		"c": {Name: "Also missing images", Images: []models.ArtistImage{}},
	}

	got := MissingImagesArtistIDs(cache)
	if len(got) != 2 {
		t.Fatalf("expected 2 ids, got %d: %v", len(got), got)
	}
	if got[0] != "a" || got[1] != "c" {
		t.Fatalf("ids = %v, want [a c]", got)
	}
}

func TestGenresForPlays(t *testing.T) {
	cache := map[string]Entry{
		"a1": {Name: "Artist A", Genres: []string{"rock", "indie"}},
		"a2": {Name: "Artist B", Genres: []string{"pop"}},
	}
	plays := []models.Play{
		{ArtistID: "a1", ArtistName: "Artist A"},
		{ArtistID: "a1", ArtistName: "Artist A"},
		{ArtistID: "a2", ArtistName: "Artist B"},
		{ArtistID: "a3", ArtistName: "Artist C"}, // not in cache
	}

	result := GenresForPlays(cache, plays)
	if len(result) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result))
	}
	if len(result["Artist A"]) != 2 {
		t.Errorf("Artist A genres = %v, want 2", result["Artist A"])
	}
	if len(result["Artist B"]) != 1 {
		t.Errorf("Artist B genres = %v, want 1", result["Artist B"])
	}
	if _, ok := result["Artist C"]; ok {
		t.Error("Artist C should not be in result (not cached)")
	}
}

func TestGenresForPlays_emptyCache(t *testing.T) {
	result := GenresForPlays(map[string]Entry{}, []models.Play{
		{ArtistID: "a1", ArtistName: "X"},
	})
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestGenresForPlays_emptyGenres(t *testing.T) {
	cache := map[string]Entry{
		"a1": {Name: "Artist A", Genres: []string{}},
	}
	result := GenresForPlays(cache, []models.Play{
		{ArtistID: "a1", ArtistName: "Artist A"},
	})
	if _, ok := result["Artist A"]; ok {
		t.Error("Artist A with empty genres should not appear in result")
	}
}

func TestUncachedArtistIDs(t *testing.T) {
	cache := map[string]Entry{
		"a1": {Name: "Cached"},
	}
	plays := []models.Play{
		{ArtistID: "a1", ArtistName: "Cached"},
		{ArtistID: "a2", ArtistName: "New1"},
		{ArtistID: "a3", ArtistName: "New2"},
		{ArtistID: "a2", ArtistName: "New1"}, // duplicate
		{ArtistID: "", ArtistName: "Empty"},  // empty ID
	}

	ids := UncachedArtistIDs(cache, plays)
	if len(ids) != 2 {
		t.Fatalf("expected 2 uncached IDs, got %d: %v", len(ids), ids)
	}
	if ids[0] != "a2" || ids[1] != "a3" {
		t.Errorf("ids = %v, want [a2, a3]", ids)
	}
}

func TestUncachedArtistIDs_allCached(t *testing.T) {
	cache := map[string]Entry{
		"a1": {Name: "X"},
	}
	ids := UncachedArtistIDs(cache, []models.Play{{ArtistID: "a1"}})
	if len(ids) != 0 {
		t.Errorf("expected no uncached IDs, got %v", ids)
	}
}
