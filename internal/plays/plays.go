package plays

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/benstraw/music-garden/internal/models"
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

// parsePlayedAt parses a Spotify played_at timestamp (RFC3339 with optional sub-seconds).
func parsePlayedAt(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

// WeekKey returns the ISO year-week string for a UTC time, e.g. "2026-W10".
// Uses the ISO week year so that Dec 31 / Jan 1 boundaries are handled correctly.
func WeekKey(t time.Time) string {
	year, week := t.UTC().ISOWeek()
	return fmt.Sprintf("%d-W%02d", year, week)
}

// ShardedPath returns the file path for a play's ISO week within baseDir.
// Example: baseDir/2026/2026-W10.json
func ShardedPath(baseDir string, t time.Time) string {
	year, week := t.UTC().ISOWeek()
	yearStr := fmt.Sprintf("%d", year)
	weekFile := fmt.Sprintf("%d-W%02d.json", year, week)
	return filepath.Join(baseDir, yearStr, weekFile)
}

// LoadSharded reads all plays from the annual/weekly directory structure under baseDir.
// Returns an empty slice (no error) if baseDir does not exist.
// Files are expected at baseDir/YYYY/YYYY-WNN.json.
func LoadSharded(baseDir string) ([]models.Play, error) {
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return []models.Play{}, nil
	}
	var all []models.Play
	err := filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		ps, err := Load(path)
		if err != nil {
			return fmt.Errorf("load %s: %w", path, err)
		}
		all = append(all, ps...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].PlayedAt > all[j].PlayedAt
	})
	return all, nil
}

// LoadShardedRange reads plays from weekly files that overlap the given UTC range [from, to].
// Returns an empty slice (no error) if baseDir does not exist.
func LoadShardedRange(baseDir string, from, to time.Time) ([]models.Play, error) {
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return []models.Play{}, nil
	}
	var all []models.Play
	seen := make(map[string]bool)
	cur := from.UTC()
	end := to.UTC()
	for !cur.After(end) {
		key := WeekKey(cur)
		if !seen[key] {
			seen[key] = true
			path := ShardedPath(baseDir, cur)
			ps, err := Load(path)
			if err != nil {
				return nil, fmt.Errorf("load %s: %w", path, err)
			}
			all = append(all, ps...)
		}
		cur = cur.AddDate(0, 0, 7)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].PlayedAt > all[j].PlayedAt
	})
	return all, nil
}

// SaveSharded writes incoming plays into the annual/weekly directory structure under baseDir.
// Each play is routed to its ISO week file (baseDir/YYYY/YYYY-WNN.json), where it is merged
// with any plays already in that file. Returns the count of newly added plays.
// Plays with unparseable played_at values are silently skipped.
func SaveSharded(baseDir string, incoming []models.Play) (int, error) {
	// Group incoming plays by their week file path.
	type weekGroup struct {
		path  string
		plays []models.Play
	}
	groups := make(map[string]*weekGroup)
	for _, p := range incoming {
		t, err := parsePlayedAt(p.PlayedAt)
		if err != nil {
			continue // skip unparseable timestamp
		}
		path := ShardedPath(baseDir, t)
		if g, ok := groups[path]; ok {
			g.plays = append(g.plays, p)
		} else {
			groups[path] = &weekGroup{path: path, plays: []models.Play{p}}
		}
	}

	added := 0
	for _, g := range groups {
		existing, err := Load(g.path)
		if err != nil {
			return added, fmt.Errorf("load %s: %w", g.path, err)
		}
		merged := Merge(existing, g.plays)
		n := len(merged) - len(existing)
		if n <= 0 {
			continue // nothing new for this week
		}
		added += n
		if err := os.MkdirAll(filepath.Dir(g.path), 0755); err != nil {
			return added, fmt.Errorf("mkdir %s: %w", filepath.Dir(g.path), err)
		}
		if err := Save(g.path, merged); err != nil {
			return added, fmt.Errorf("save %s: %w", g.path, err)
		}
	}
	return added, nil
}

// MigrateToSharded reads plays from a legacy plays.json file, saves them into the sharded
// directory structure, and renames the legacy file to plays.json.bak on success.
// If the legacy file does not exist, or plays.json.bak already exists, MigrateToSharded
// is a no-op and returns nil.
func MigrateToSharded(legacyPath, baseDir string) error {
	// Skip if the legacy file is absent — nothing to migrate.
	if _, err := os.Stat(legacyPath); os.IsNotExist(err) {
		return nil
	}
	bakPath := legacyPath + ".bak"
	if _, err := os.Stat(bakPath); err == nil {
		return nil // already migrated
	}
	existing, err := Load(legacyPath)
	if err != nil {
		return fmt.Errorf("load legacy plays: %w", err)
	}
	if len(existing) == 0 {
		return nil // nothing to migrate
	}
	if _, err := SaveSharded(baseDir, existing); err != nil {
		return fmt.Errorf("save sharded: %w", err)
	}
	if err := os.Rename(legacyPath, bakPath); err != nil {
		return fmt.Errorf("rename legacy file: %w", err)
	}
	return nil
}
