package plays

import (
	"encoding/json"
	"os"
	"sort"

	"github.com/benstraw/spotify-garden/internal/models"
)

// Load reads plays.json and returns the plays, or an empty slice if the file doesn't exist.
func Load(path string) ([]models.Play, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []models.Play{}, nil
	}
	if err != nil {
		return nil, err
	}
	var plays []models.Play
	if err := json.Unmarshal(data, &plays); err != nil {
		return nil, err
	}
	return plays, nil
}

// Save writes plays to path sorted descending by played_at with 0644 permissions.
func Save(path string, plays []models.Play) error {
	sort.Slice(plays, func(i, j int) bool {
		return plays[i].PlayedAt > plays[j].PlayedAt
	})
	data, err := json.MarshalIndent(plays, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Merge returns the union of existing and incoming plays, deduplicated by played_at,
// sorted descending.
func Merge(existing, incoming []models.Play) []models.Play {
	seen := make(map[string]bool, len(existing))
	for _, p := range existing {
		seen[p.PlayedAt] = true
	}
	combined := make([]models.Play, len(existing))
	copy(combined, existing)
	for _, p := range incoming {
		if !seen[p.PlayedAt] {
			combined = append(combined, p)
			seen[p.PlayedAt] = true
		}
	}
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].PlayedAt > combined[j].PlayedAt
	})
	return combined
}
