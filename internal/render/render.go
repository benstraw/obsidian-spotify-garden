package render

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/benstraw/spotify-garden/internal/models"
)

// --- Weekly note ---

// WeekBounds returns the Monday 00:00:00 local and the following Monday 00:00:00 local
// for the ISO week containing date.
func WeekBounds(date time.Time) (monday, nextMonday time.Time) {
	d := date.Local()
	weekday := int(d.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday = 7
	}
	mon := d.AddDate(0, 0, -(weekday - 1))
	monday = time.Date(mon.Year(), mon.Month(), mon.Day(), 0, 0, 0, 0, time.Local)
	nextMonday = monday.AddDate(0, 0, 7)
	return
}

// WeekStr returns the ISO week string "YYYY-Www" for the given date.
func WeekStr(date time.Time) string {
	isoYear, isoWeek := date.ISOWeek()
	return fmt.Sprintf("%d-W%02d", isoYear, isoWeek)
}

// PlaysForWeek returns plays that fall within the ISO week of date (local time), sorted ascending.
func PlaysForWeek(plays []models.Play, date time.Time) []models.Play {
	monday, nextMonday := WeekBounds(date)
	var result []models.Play
	for _, p := range plays {
		t, err := time.Parse(time.RFC3339, p.PlayedAt)
		if err != nil {
			// Try with milliseconds
			t, err = time.Parse("2006-01-02T15:04:05.000Z", p.PlayedAt)
			if err != nil {
				continue
			}
		}
		localT := t.Local()
		if !localT.Before(monday) && localT.Before(nextMonday) {
			result = append(result, p)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].PlayedAt < result[j].PlayedAt
	})
	return result
}

// PlaysForDay returns plays that fall on the calendar day of day (local time), sorted ascending.
func PlaysForDay(plays []models.Play, day time.Time) []models.Play {
	d := day.Local()
	start := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 0, 1)
	var result []models.Play
	for _, p := range plays {
		t, err := parsePlayedAt(p.PlayedAt)
		if err != nil {
			continue
		}
		localT := t.Local()
		if !localT.Before(start) && localT.Before(end) {
			result = append(result, p)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].PlayedAt < result[j].PlayedAt
	})
	return result
}

// fmtDuration formats milliseconds as "Xh Ymin" or "Ymin".
func fmtDuration(totalMS int) string {
	totalMin := totalMS / 60000
	hours := totalMin / 60
	mins := totalMin % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dmin", hours, mins)
	}
	return fmt.Sprintf("%dmin", mins)
}

// RenderWeekly builds the weekly note for the ISO week containing date.
// It also creates artist stubs for new artists.
// artistGenres maps artist names to their genres; if nil, genre section is omitted.
func RenderWeekly(plays []models.Play, date time.Time, vaultPath string, artistGenres map[string][]string) (string, error) {
	weekPlays := PlaysForWeek(plays, date)
	monday, _ := WeekBounds(date)
	weekStr := WeekStr(monday)
	dateStr := time.Now().Format("2006-01-02")

	// Stats
	uniqueTracks := map[string]bool{}
	uniqueArtists := map[string]bool{}
	uniqueAlbums := map[string]bool{}
	totalMS := 0
	for _, p := range weekPlays {
		uniqueTracks[p.TrackName+"|"+p.ArtistName] = true
		uniqueArtists[p.ArtistName] = true
		if p.AlbumName != "" {
			uniqueAlbums[p.AlbumName] = true
		}
		totalMS += p.DurationMS
	}

	// Repeated tracks
	trackCounts := map[string]int{}
	trackMeta := map[string][2]string{} // key -> [trackName, artistName]
	for _, p := range weekPlays {
		key := p.TrackName + "||" + p.ArtistName
		trackCounts[key]++
		trackMeta[key] = [2]string{p.TrackName, p.ArtistName}
	}
	type repeatedEntry struct {
		trackName  string
		artistName string
		count      int
	}
	var repeated []repeatedEntry
	for key, count := range trackCounts {
		if count >= 2 {
			meta := trackMeta[key]
			repeated = append(repeated, repeatedEntry{meta[0], meta[1], count})
		}
	}
	sort.Slice(repeated, func(i, j int) bool {
		return repeated[i].count > repeated[j].count
	})

	// Albums
	albumCounts := map[string]int{}
	albumArtist := map[string]string{}
	for _, p := range weekPlays {
		if p.AlbumName != "" {
			albumCounts[p.AlbumName]++
			albumArtist[p.AlbumName] = p.ArtistName
		}
	}
	type albumEntry struct {
		albumName  string
		artistName string
		count      int
	}
	var albums []albumEntry
	for album, count := range albumCounts {
		albums = append(albums, albumEntry{album, albumArtist[album], count})
	}
	sort.Slice(albums, func(i, j int) bool {
		return albums[i].count > albums[j].count
	})

	// Artists in rotation (sorted)
	artistsSorted := make([]string, 0, len(uniqueArtists))
	for a := range uniqueArtists {
		artistsSorted = append(artistsSorted, a)
	}
	sort.Strings(artistsSorted)

	// New artists (not yet in vault)
	artistsDir := filepath.Join(vaultPath, "music", "artists")
	var newArtists []string
	for _, a := range artistsSorted {
		stubPath := filepath.Join(artistsDir, a+".md")
		if _, err := os.Stat(stubPath); os.IsNotExist(err) {
			newArtists = append(newArtists, a)
		}
	}

	// Create artist stubs for weekly plays.
	allArtistNames := map[string]bool{}
	for _, a := range artistsSorted {
		allArtistNames[a] = true
	}

	// Build url map from weekly plays.
	artistURLs := map[string]string{}
	for _, p := range weekPlays {
		if _, ok := artistURLs[p.ArtistName]; !ok {
			artistURLs[p.ArtistName] = p.ArtistSpotifyURL
		}
	}

	sortedAllArtists := make([]string, 0, len(allArtistNames))
	for name := range allArtistNames {
		sortedAllArtists = append(sortedAllArtists, name)
	}
	sort.Strings(sortedAllArtists)

	for _, name := range sortedAllArtists {
		var g []string
		if artistGenres != nil {
			g = artistGenres[name]
		}
		if err := EnsureArtistStub(name, artistURLs[name], g, dateStr, vaultPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not create artist stub for %s: %v\n", name, err)
		}
	}

	// Build the note string
	var sb strings.Builder

	// Frontmatter
	sb.WriteString("---\n")
	sb.WriteString("type: note\n")
	sb.WriteString("tags: [music, weekly-music]\n")
	fmt.Fprintf(&sb, "created: %s\n", dateStr)
	fmt.Fprintf(&sb, "week: %s\n", weekStr)
	sb.WriteString("---\n\n")

	// Title
	fmt.Fprintf(&sb, "# Week in Music: %s\n\n", weekStr)

	// Stats
	sb.WriteString("## Stats\n")
	fmt.Fprintf(&sb, "- Plays tracked: %d  |  Unique tracks: %d  |  Unique artists: %d  |  Unique albums: %d\n",
		len(weekPlays), len(uniqueTracks), len(uniqueArtists), len(uniqueAlbums))
	fmt.Fprintf(&sb, "- Estimated listening time: %s\n\n", fmtDuration(totalMS))

	// Repeated tracks
	if len(repeated) > 0 {
		sb.WriteString("## Repeated Tracks  (played \u22652\u00d7)\n")
		for _, r := range repeated {
			fmt.Fprintf(&sb, "- %s \u2014 [[%s]]  \u00d7%d\n", r.trackName, r.artistName, r.count)
		}
		sb.WriteString("\n")
	}

	// Albums
	if len(albums) > 0 {
		sb.WriteString("## Albums This Week\n")
		for _, a := range albums {
			artistPart := ""
			if a.artistName != "" {
				artistPart = fmt.Sprintf(" \u2014 [[%s]]", a.artistName)
			}
			playWord := "plays"
			if a.count == 1 {
				playWord = "play"
			}
			fmt.Fprintf(&sb, "- %s%s  (%d %s)\n", a.albumName, artistPart, a.count, playWord)
		}
		sb.WriteString("\n")
	}

	// Artists in rotation
	if len(artistsSorted) > 0 {
		sb.WriteString("## Artists in Rotation\n")
		links := make([]string, len(artistsSorted))
		for i, a := range artistsSorted {
			links[i] = "[[" + a + "]]"
		}
		sb.WriteString(strings.Join(links, ", "))
		sb.WriteString("\n\n")
	}

	// Genres this week
	if len(artistGenres) > 0 {
		genreCounts := map[string]int{}
		for _, p := range weekPlays {
			if gs, ok := artistGenres[p.ArtistName]; ok {
				for _, g := range gs {
					genreCounts[g]++
				}
			}
		}
		if len(genreCounts) > 0 {
			type genreCount struct {
				genre string
				count int
			}
			sorted := make([]genreCount, 0, len(genreCounts))
			for g, c := range genreCounts {
				sorted = append(sorted, genreCount{g, c})
			}
			sort.Slice(sorted, func(i, j int) bool {
				if sorted[i].count != sorted[j].count {
					return sorted[i].count > sorted[j].count
				}
				return sorted[i].genre < sorted[j].genre
			})

			sb.WriteString("## Genres This Week\n")
			parts := make([]string, len(sorted))
			for i, gc := range sorted {
				playWord := "plays"
				if gc.count == 1 {
					playWord = "play"
				}
				parts[i] = fmt.Sprintf("[[%s]] (%d %s)", gc.genre, gc.count, playWord)
			}
			sb.WriteString(strings.Join(parts, ", "))
			sb.WriteString("\n\n")
		}
	}

	// New artists
	if len(newArtists) > 0 {
		sb.WriteString("## New Artists  (first appearance in vault)\n")
		for _, a := range newArtists {
			fmt.Fprintf(&sb, "- [[%s]]\n", a)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Notes\n\n\n")

	return sb.String(), nil
}

// RenderDaily builds the daily note for the calendar day containing date.
// Returns ("", nil) when there are no plays for that day (caller skips writing).
// artistGenres maps artist names to their genres; if nil, genre section is omitted.
func RenderDaily(plays []models.Play, date time.Time, vaultPath string, artistGenres map[string][]string) (string, error) {
	dayPlays := PlaysForDay(plays, date)
	if len(dayPlays) == 0 {
		return "", nil
	}

	d := date.Local()
	dayStr := d.Format("2006-01-02")
	dateStr := dayStr

	// Stats
	uniqueTracks := map[string]bool{}
	uniqueArtists := map[string]bool{}
	uniqueAlbums := map[string]bool{}
	totalMS := 0
	for _, p := range dayPlays {
		uniqueTracks[p.TrackName+"|"+p.ArtistName] = true
		uniqueArtists[p.ArtistName] = true
		if p.AlbumName != "" {
			uniqueAlbums[p.AlbumName] = true
		}
		totalMS += p.DurationMS
	}

	var sb strings.Builder

	// Frontmatter
	sb.WriteString("---\n")
	sb.WriteString("type: note\n")
	sb.WriteString("tags: [music, daily-music]\n")
	fmt.Fprintf(&sb, "created: %s\n", dateStr)
	fmt.Fprintf(&sb, "date: %s\n", dayStr)
	sb.WriteString("---\n\n")

	// Title
	fmt.Fprintf(&sb, "# Daily Listening: %s\n\n", dayStr)

	// Stats
	sb.WriteString("## Stats\n")
	fmt.Fprintf(&sb, "- Plays: %d  |  Unique tracks: %d  |  Unique artists: %d  |  Unique albums: %d\n",
		len(dayPlays), len(uniqueTracks), len(uniqueArtists), len(uniqueAlbums))
	fmt.Fprintf(&sb, "- Estimated listening time: %s\n\n", fmtDuration(totalMS))

	// Play Log
	sb.WriteString("## Play Log\n")
	for _, p := range dayPlays {
		t, err := parsePlayedAt(p.PlayedAt)
		if err != nil {
			continue
		}
		localT := t.Local()
		timeStr := fmt.Sprintf("%d:%02d", localT.Hour(), localT.Minute())
		albumPart := ""
		if p.AlbumName != "" {
			albumPart = fmt.Sprintf("  _(%s)_", p.AlbumName)
		}
		fmt.Fprintf(&sb, "- %s  %s — [[%s]]%s\n", timeStr, p.TrackName, p.ArtistName, albumPart)
	}
	sb.WriteString("\n")

	type songEntry struct {
		trackName  string
		artistName string
		albumName  string
		count      int
	}
	songCounts := map[string]*songEntry{}
	for _, p := range dayPlays {
		key := p.TrackName + "||" + p.ArtistName + "||" + p.AlbumName
		if songCounts[key] == nil {
			songCounts[key] = &songEntry{
				trackName:  p.TrackName,
				artistName: p.ArtistName,
				albumName:  p.AlbumName,
			}
		}
		songCounts[key].count++
	}
	songs := make([]songEntry, 0, len(songCounts))
	for _, e := range songCounts {
		songs = append(songs, *e)
	}
	sort.Slice(songs, func(i, j int) bool {
		if songs[i].count != songs[j].count {
			return songs[i].count > songs[j].count
		}
		if songs[i].trackName != songs[j].trackName {
			return songs[i].trackName < songs[j].trackName
		}
		if songs[i].artistName != songs[j].artistName {
			return songs[i].artistName < songs[j].artistName
		}
		return songs[i].albumName < songs[j].albumName
	})

	type artistEntry struct {
		name  string
		count int
	}
	artistCounts := map[string]int{}
	for _, p := range dayPlays {
		artistCounts[p.ArtistName]++
	}
	artists := make([]artistEntry, 0, len(artistCounts))
	for name, count := range artistCounts {
		artists = append(artists, artistEntry{name: name, count: count})
	}
	sort.Slice(artists, func(i, j int) bool {
		if artists[i].count != artists[j].count {
			return artists[i].count > artists[j].count
		}
		return artists[i].name < artists[j].name
	})

	type albumEntry struct {
		albumName  string
		artistName string
		count      int
	}
	albumCounts := map[string]*albumEntry{}
	for _, p := range dayPlays {
		albumName := p.AlbumName
		if albumName == "" {
			albumName = "(No Album)"
		}
		key := albumName + "||" + p.ArtistName
		if albumCounts[key] == nil {
			albumCounts[key] = &albumEntry{
				albumName:  albumName,
				artistName: p.ArtistName,
			}
		}
		albumCounts[key].count++
	}
	albums := make([]albumEntry, 0, len(albumCounts))
	for _, e := range albumCounts {
		albums = append(albums, *e)
	}
	sort.Slice(albums, func(i, j int) bool {
		if albums[i].count != albums[j].count {
			return albums[i].count > albums[j].count
		}
		if albums[i].albumName != albums[j].albumName {
			return albums[i].albumName < albums[j].albumName
		}
		return albums[i].artistName < albums[j].artistName
	})

	sb.WriteString("## Songs Played\n")
	for _, s := range songs {
		albumPart := ""
		if s.albumName != "" {
			albumPart = fmt.Sprintf("  _(%s)_", s.albumName)
		}
		fmt.Fprintf(&sb, "- %s \u2014 [[%s]]%s  \u00d7%d\n", s.trackName, s.artistName, albumPart, s.count)
	}
	sb.WriteString("\n")

	sb.WriteString("## Artists Played\n")
	for _, a := range artists {
		playWord := "plays"
		if a.count == 1 {
			playWord = "play"
		}
		fmt.Fprintf(&sb, "- [[%s]]  (%d %s)\n", a.name, a.count, playWord)
	}
	sb.WriteString("\n")

	sb.WriteString("## Albums Played\n")
	for _, a := range albums {
		artistPart := ""
		if a.artistName != "" {
			artistPart = fmt.Sprintf(" \u2014 [[%s]]", a.artistName)
		}
		playWord := "plays"
		if a.count == 1 {
			playWord = "play"
		}
		fmt.Fprintf(&sb, "- %s%s  (%d %s)\n", a.albumName, artistPart, a.count, playWord)
	}
	sb.WriteString("\n")

	// Genres
	if len(artistGenres) > 0 {
		genreSet := map[string]bool{}
		for _, p := range dayPlays {
			if gs, ok := artistGenres[p.ArtistName]; ok {
				for _, g := range gs {
					genreSet[g] = true
				}
			}
		}
		if len(genreSet) > 0 {
			sorted := make([]string, 0, len(genreSet))
			for g := range genreSet {
				sorted = append(sorted, g)
			}
			sort.Strings(sorted)

			sb.WriteString("## Genres\n")
			links := make([]string, len(sorted))
			for i, g := range sorted {
				links[i] = "[[" + g + "]]"
			}
			sb.WriteString(strings.Join(links, ", "))
			sb.WriteString("\n\n")
		}
	}

	sb.WriteString("## Notes\n\n\n")

	return sb.String(), nil
}

// EnsureArtistStub creates an artist stub at {vaultPath}/music/artists/{name}.md
// if it doesn't exist. Never overwrites.
func EnsureArtistStub(name, spotifyURL string, genres []string, dateStr, vaultPath string) error {
	stubDir := filepath.Join(vaultPath, "music", "artists")
	stubPath := filepath.Join(stubDir, name+".md")

	if _, err := os.Stat(stubPath); err == nil {
		return nil // already exists
	}

	if err := os.MkdirAll(stubDir, 0755); err != nil {
		return err
	}

	genresYAML := "[]"
	if len(genres) > 0 {
		quoted := make([]string, len(genres))
		for i, g := range genres {
			quoted[i] = fmt.Sprintf("%q", g)
		}
		genresYAML = "[" + strings.Join(quoted, ", ") + "]"
	}

	content := fmt.Sprintf(`---
type: resource
tags: [music/artist]
created: %s
spotify_url: %s
genres: %s
---

# %s

[Open in Spotify](%s)

## Weekly Appearances

`+"```dataview"+`
LIST FROM "music/listening"
WHERE contains(file.outlinks, this.file.link)
SORT file.name DESC
`+"```"+`

## Concerts

`+"```dataview"+`
LIST FROM "music/concerts"
WHERE contains(tags, "music/live-artist/" + this.file.name)
SORT date DESC
`+"```"+`

## Notes

`, dateStr, spotifyURL, genresYAML, name, spotifyURL)

	if err := os.WriteFile(stubPath, []byte(content), 0644); err != nil {
		return err
	}
	fmt.Printf("  Created artist stub: %s.md\n", name)
	return nil
}

// UpdateArtistGenres updates the genres frontmatter line in an existing artist stub.
// No-op if the file doesn't exist or doesn't contain a genres line.
func UpdateArtistGenres(name string, genres []string, vaultPath string) error {
	if len(genres) == 0 {
		return nil
	}
	stubPath := filepath.Join(vaultPath, "music", "artists", name+".md")
	data, err := os.ReadFile(stubPath)
	if err != nil {
		return nil // file doesn't exist — no-op
	}

	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, "genres:") {
			quoted := make([]string, len(genres))
			for j, g := range genres {
				quoted[j] = fmt.Sprintf("%q", g)
			}
			lines[i] = "genres: [" + strings.Join(quoted, ", ") + "]"
			found = true
			break
		}
	}
	if !found {
		return nil
	}

	return os.WriteFile(stubPath, []byte(strings.Join(lines, "\n")), 0644)
}

// --- Persona context pack ---

// PersonaData holds data for the persona template.
type PersonaData struct {
	DateStr          string
	TopArtistsShort  string
	TopArtistsMedium string
	TopArtistsLong   string
	TopGenres        string
	RecentArtists    string
}

// RenderPersona builds the Music Taste context pack content.
func RenderPersona(
	topArtistsShort, topArtistsMedium, topArtistsLong []models.TopArtist,
	weekPlays []models.Play,
	tmplPath string,
) (string, error) {
	dateStr := time.Now().Format("2006-01-02")

	artistList := func(artists []models.TopArtist) string {
		if len(artists) == 0 {
			return "_No data_"
		}
		names := make([]string, len(artists))
		for i, a := range artists {
			names[i] = a.Name
		}
		return strings.Join(names, ", ")
	}

	genreList := func(artists []models.TopArtist) string {
		seen := map[string]bool{}
		var genres []string
		for _, a := range artists {
			for _, g := range a.Genres {
				if !seen[g] {
					seen[g] = true
					genres = append(genres, g)
				}
			}
		}
		if len(genres) == 0 {
			return "_No data_"
		}
		if len(genres) > 15 {
			genres = genres[:15]
		}
		return strings.Join(genres, ", ")
	}

	recentNames := map[string]bool{}
	for _, p := range weekPlays {
		recentNames[p.ArtistName] = true
	}
	recentSorted := make([]string, 0, len(recentNames))
	for name := range recentNames {
		recentSorted = append(recentSorted, name)
	}
	sort.Strings(recentSorted)
	recentStr := "_No data_"
	if len(recentSorted) > 0 {
		recentStr = strings.Join(recentSorted, ", ")
	}

	data := PersonaData{
		DateStr:          dateStr,
		TopArtistsShort:  artistList(topArtistsShort),
		TopArtistsMedium: artistList(topArtistsMedium),
		TopArtistsLong:   artistList(topArtistsLong),
		TopGenres:        genreList(topArtistsShort),
		RecentArtists:    recentStr,
	}

	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		return "", fmt.Errorf("parse persona template: %w", err)
	}

	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("render persona template: %w", err)
	}
	return sb.String(), nil
}

// parsePlayedAt parses a Spotify played_at timestamp (RFC3339 or with milliseconds).
func parsePlayedAt(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02T15:04:05.000Z", s)
}
