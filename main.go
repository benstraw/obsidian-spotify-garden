package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/benstraw/spotify-garden/internal/auth"
	"github.com/benstraw/spotify-garden/internal/client"
	"github.com/benstraw/spotify-garden/internal/fetch"
	"github.com/benstraw/spotify-garden/internal/plays"
	"github.com/benstraw/spotify-garden/internal/render"
)

const playsFile = "data/plays.json"

func main() {
	loadDotEnv(".env")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "auth":
		runAuth()
	case "collect":
		runCollect()
	case "weekly":
		runWeekly(args)
	case "catch-up":
		runCatchUp(args)
	case "persona":
		runPersona()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`spotify-garden — Spotify listening data → Obsidian markdown

Usage:
  spotify-garden auth                       Authenticate with Spotify via OAuth
  spotify-garden collect                    Fetch last 50 recently-played, dedup, append to plays.json
  spotify-garden weekly [--date YYYY-MM-DD] Generate weekly note for date's ISO week (default: current)
  spotify-garden catch-up [--weeks N]       Generate missing weekly notes (default: 8 weeks back)
  spotify-garden persona                    Regenerate Music Taste context pack

Flags:
  --date   Date in YYYY-MM-DD format (default: today)
  --weeks  Number of weeks to check (default: 8)
`)
}

// loadDotEnv reads a .env file and sets environment variables.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // .env is optional
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		if key != "" {
			os.Setenv(key, val)
		}
	}
}

// vaultPath returns the Obsidian vault path from the environment.
func vaultPath() string {
	v := os.Getenv("OBSIDIAN_VAULT_PATH")
	if v == "" {
		fmt.Fprintln(os.Stderr, "OBSIDIAN_VAULT_PATH not set")
		os.Exit(1)
	}
	return v
}

// templatesDir returns the path to the templates directory.
func templatesDir() string {
	if td := os.Getenv("SPOTIFY_TEMPLATES_DIR"); td != "" {
		return td
	}
	if _, err := os.Stat("templates"); err == nil {
		return "templates"
	}
	exe, _ := os.Executable()
	return filepath.Join(filepath.Dir(exe), "templates")
}

// getClient loads tokens (refreshing if needed) and returns an API client.
func getClient() (*client.Client, error) {
	token, err := auth.RefreshIfNeeded()
	if err != nil {
		return nil, fmt.Errorf("authentication error: %w\nRun 'spotify-garden auth' to authenticate.", err)
	}
	return client.NewClient(token), nil
}

// parseDate parses a YYYY-MM-DD date string or returns today.
func parseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Now(), nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q (expected YYYY-MM-DD): %w", s, err)
	}
	return t, nil
}

// ensurePlaysDir creates the data/ directory if needed.
func ensurePlaysDir() error {
	return os.MkdirAll("data", 0755)
}

// --- Subcommands ---

func runAuth() {
	if err := auth.StartAuthFlow(); err != nil {
		fmt.Fprintln(os.Stderr, "auth failed:", err)
		os.Exit(1)
	}
}

func runCollect() {
	c, err := getClient()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println("Fetching recently played tracks...")
	incoming, err := fetch.GetRecentlyPlayed(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fetch error:", err)
		os.Exit(1)
	}

	if err := ensurePlaysDir(); err != nil {
		fmt.Fprintln(os.Stderr, "data dir error:", err)
		os.Exit(1)
	}

	existing, err := plays.Load(playsFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load error:", err)
		os.Exit(1)
	}

	merged := plays.Merge(existing, incoming)
	newCount := len(merged) - len(existing)

	if err := plays.Save(playsFile, merged); err != nil {
		fmt.Fprintln(os.Stderr, "save error:", err)
		os.Exit(1)
	}

	fmt.Printf("Added %d new plays (%d total).\n", newCount, len(merged))
}

func runWeekly(args []string) {
	fs := flag.NewFlagSet("weekly", flag.ExitOnError)
	dateStr := fs.String("date", "", "any date within the target week (default: this week)")
	_ = fs.Parse(args)

	date, err := parseDate(*dateStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	generateWeeklyNote(date)
}

// generateWeeklyNote fetches API data, filters plays, and writes the weekly note.
func generateWeeklyNote(date time.Time) {
	monday, _ := render.WeekBounds(date)
	weekStr := render.WeekStr(monday)

	vault := vaultPath()
	outDir := filepath.Join(vault, "music", "listening")
	outPath := filepath.Join(outDir, fmt.Sprintf("spotify-%s.md", weekStr))

	c, err := getClient()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("Fetching top tracks (short_term)...\n")
	topTracksShort, err := fetch.GetTopTracks(c, "short_term")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: top tracks: %v\n", err)
	}

	fmt.Printf("Fetching top artists (short_term)...\n")
	topArtistsShort, err := fetch.GetTopArtists(c, "short_term")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: top artists: %v\n", err)
	}

	allPlays, err := plays.Load(playsFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load plays error:", err)
		os.Exit(1)
	}

	weekPlays := render.PlaysForWeek(allPlays, date)
	fmt.Printf("Plays for week %s: %d\n", weekStr, len(weekPlays))

	content, err := render.RenderWeekly(allPlays, topTracksShort, topArtistsShort, date, vault)
	if err != nil {
		fmt.Fprintln(os.Stderr, "render error:", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir error:", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
		fmt.Fprintln(os.Stderr, "write error:", err)
		os.Exit(1)
	}

	fmt.Println("Written:", outPath)
}

func runCatchUp(args []string) {
	fs := flag.NewFlagSet("catch-up", flag.ExitOnError)
	weeks := fs.Int("weeks", 8, "number of weeks to check")
	_ = fs.Parse(args)

	vault := vaultPath()
	listeningDir := filepath.Join(vault, "music", "listening")

	now := time.Now()

	// Collect missing weeks
	var missingDates []time.Time
	for i := 0; i < *weeks; i++ {
		// Go back i full weeks from the current week
		weekDate := now.AddDate(0, 0, -(i * 7))
		monday, _ := render.WeekBounds(weekDate)
		weekStr := render.WeekStr(monday)
		notePath := filepath.Join(listeningDir, fmt.Sprintf("spotify-%s.md", weekStr))
		if _, err := os.Stat(notePath); os.IsNotExist(err) {
			missingDates = append(missingDates, weekDate)
		}
	}

	if len(missingDates) == 0 {
		fmt.Println("All caught up — no missing weekly notes.")
		return
	}

	fmt.Printf("Found %d missing weekly note(s), generating...\n", len(missingDates))

	// Generate oldest first
	for i := len(missingDates) - 1; i >= 0; i-- {
		generateWeeklyNote(missingDates[i])
	}

	fmt.Println("Done.")
}

func runPersona() {
	vault := vaultPath()
	outPath := filepath.Join(vault, "01-ai-brain", "context-packs", "Music Taste.md")
	tmplPath := filepath.Join(templatesDir(), "persona.md.tmpl")

	c, err := getClient()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println("Fetching top artists (short_term, medium_term, long_term)...")
	topArtistsShort, err := fetch.GetTopArtists(c, "short_term")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: top artists short_term: %v\n", err)
	}
	topArtistsMedium, err := fetch.GetTopArtists(c, "medium_term")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: top artists medium_term: %v\n", err)
	}
	topArtistsLong, err := fetch.GetTopArtists(c, "long_term")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: top artists long_term: %v\n", err)
	}

	allPlays, err := plays.Load(playsFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load plays error:", err)
		os.Exit(1)
	}
	weekPlays := render.PlaysForWeek(allPlays, time.Now())

	content, err := render.RenderPersona(topArtistsShort, topArtistsMedium, topArtistsLong, weekPlays, tmplPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "render error:", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir error:", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
		fmt.Fprintln(os.Stderr, "write error:", err)
		os.Exit(1)
	}

	fmt.Println("Written:", outPath)
}
