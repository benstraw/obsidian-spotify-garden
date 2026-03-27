package genres

import (
	"encoding/json"
	"os"
	"sort"
	"time"

	"github.com/benstraw/music-garden/internal/models"
)

// Entry represents a cached genre mapping for an artist.
type Entry struct {
	Name        string               `json:"name"`
	Genres      []string             `json:"genres"`
	Images      []models.ArtistImage `json:"images,omitempty"`
	LastUpdated string               `json:"last_updated"`
}

// Load reads the genre cache from path, returning an empty map if the file doesn't exist.
func Load(path string) (map[string]Entry, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	var cache map[string]Entry
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	return cache, nil
}

// Save writes the genre cache to path with 0644 permissions.
func Save(path string, cache map[string]Entry) error {
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Update sets or replaces the cache entry for the given artist ID.
func Update(cache map[string]Entry, id, name string, genres []string, images []models.ArtistImage) {
	cache[id] = Entry{
		Name:        name,
		Genres:      genres,
		Images:      images,
		LastUpdated: time.Now().Format("2006-01-02"),
	}
}

// UpdateImages updates only the images of an existing entry, leaving genres unchanged.
func UpdateImages(cache map[string]Entry, id string, images []models.ArtistImage) {
	if entry, ok := cache[id]; ok {
		entry.Images = images
		entry.LastUpdated = time.Now().Format("2006-01-02")
		cache[id] = entry
	}
}

// MissingImagesArtistIDs returns IDs of cached entries that have no images.
func MissingImagesArtistIDs(cache map[string]Entry) []string {
	var ids []string
	for id, entry := range cache {
		if len(entry.Images) == 0 {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// GenresForPlays returns a map of artist name to genres for all artists in plays
// that have entries in the cache.
func GenresForPlays(cache map[string]Entry, plays []models.Play) map[string][]string {
	result := make(map[string][]string)
	for _, p := range plays {
		if _, ok := result[p.ArtistName]; ok {
			continue
		}
		if entry, ok := cache[p.ArtistID]; ok && len(entry.Genres) > 0 {
			result[p.ArtistName] = entry.Genres
		}
	}
	return result
}

// UncachedArtistIDs returns artist IDs from plays that are not in the cache.
func UncachedArtistIDs(cache map[string]Entry, plays []models.Play) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, p := range plays {
		if p.ArtistID == "" || seen[p.ArtistID] {
			continue
		}
		seen[p.ArtistID] = true
		if _, ok := cache[p.ArtistID]; !ok {
			ids = append(ids, p.ArtistID)
		}
	}
	sort.Strings(ids)
	return ids
}
