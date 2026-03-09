package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/benstraw/spotify-garden/internal/auth"
	"github.com/benstraw/spotify-garden/internal/client"
	"github.com/benstraw/spotify-garden/internal/fetch"
	"github.com/benstraw/spotify-garden/internal/genres"
	"github.com/benstraw/spotify-garden/internal/models"
	"github.com/benstraw/spotify-garden/internal/plays"
	"github.com/benstraw/spotify-garden/internal/render"
)

// version is set at build time via -ldflags "-X main.version=vX.Y.Z"
var version = "dev"

type runtimePaths struct {
	cwd             string
	stateDir        string
	dotEnvPath      string
	tokensPath      string
	playsPath       string
	playsDir        string
	genresPath      string
	dotEnvFallback  bool
	tokensFallback  bool
	playsFallback   bool
	genresFallback  bool
}

func main() {
	paths := resolveRuntimePaths()
	if err := loadDotEnv(paths.dotEnvPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load %s: %v\n", paths.dotEnvPath, err)
	}

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]
	emitFallbackWarnings(paths, cmd)

	switch cmd {
	case "auth":
		runAuth(paths)
	case "collect":
		runCollect(paths)
	case "weekly":
		runWeekly(args, paths)
	case "daily":
		runDaily(args, paths)
	case "catch-up":
		runCatchUp(args, paths)
	case "persona":
		runPersona(paths)
	case "genre-backfill":
		runGenreBackfill(paths)
	case "setlist":
		runSetlist(args)
	case "doctor":
		os.Exit(runDoctor(paths))
	case "version", "--version":
		fmt.Println("spotify-garden", version)
	case "help", "--help", "-h":
		printUsage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`spotify-garden %s — Spotify listening data → Obsidian markdown

Usage:
  spotify-garden auth                           Authenticate with Spotify via OAuth
  spotify-garden collect                        Fetch last 50 recently-played, dedup, append to plays.json
  spotify-garden weekly [--date YYYY-MM-DD]     Generate weekly note for date's ISO week (default: current)
  spotify-garden daily [--date YYYY-MM-DD]      Generate daily note for date (default: today)
  spotify-garden catch-up [--weeks N]           Generate missing weekly + daily notes (default: 8 weeks back)
  spotify-garden persona                        Regenerate Music Taste context pack
  spotify-garden genre-backfill                 Fetch genres for all artists in plays.json
  spotify-garden setlist <artist> [--date DATE] Look up setlist on setlist.fm (default: today)
  spotify-garden doctor                         Print effective runtime config and diagnostics
  spotify-garden version                        Print version

Flags:
  --date   Date in YYYY-MM-DD format (default: today)
  --weeks  Number of weeks to check (default: 8)
`, version)
}

func resolveRuntimePaths() runtimePaths {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		cwd = "."
	}

	p := runtimePaths{
		cwd:        cwd,
		dotEnvPath: filepath.Join(cwd, ".env"),
		tokensPath: filepath.Join(cwd, "tokens.json"),
		playsPath:  filepath.Join(cwd, "data", "plays.json"),
		playsDir:   filepath.Join(cwd, "data", "plays"),
		genresPath: filepath.Join(cwd, "data", "genres.json"),
	}

	stateDir := strings.TrimSpace(os.Getenv("SPOTIFY_STATE_DIR"))
	if stateDir == "" {
		// Auto-discover the well-known installed state dir if it exists.
		home, err := os.UserHomeDir()
		if err == nil {
			candidate := filepath.Join(home, "Library", "Application Support", "spotify-garden", "state")
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				stateDir = candidate
			}
		}
	}
	if stateDir == "" {
		return p
	}

	absState, err := filepath.Abs(stateDir)
	if err != nil {
		absState = stateDir
	}
	p.stateDir = absState

	p.dotEnvPath, p.dotEnvFallback = chooseStatePath(absState, ".env", p.dotEnvPath)
	p.tokensPath, p.tokensFallback = chooseStatePath(absState, "tokens.json", p.tokensPath)
	p.playsPath, p.playsFallback = chooseStatePath(absState, filepath.Join("data", "plays.json"), p.playsPath)
	p.genresPath, p.genresFallback = chooseStatePath(absState, filepath.Join("data", "genres.json"), p.genresPath)
	p.playsDir = filepath.Join(filepath.Dir(p.playsPath), "plays")

	return p
}

func chooseStatePath(stateDir, relPath, fallbackPath string) (string, bool) {
	statePath := filepath.Join(stateDir, relPath)
	_, err := os.Stat(statePath)
	if err == nil {
		return statePath, false
	}
	if os.IsNotExist(err) {
		return fallbackPath, true
	}
	return statePath, false
}

func emitFallbackWarnings(paths runtimePaths, cmd string) {
	if paths.stateDir == "" {
		return
	}
	if cmd == "help" || cmd == "--help" || cmd == "-h" || cmd == "version" || cmd == "--version" {
		return
	}

	if paths.dotEnvFallback {
		fmt.Fprintf(os.Stderr, "warning: SPOTIFY_STATE_DIR is set but %s was not found; falling back to %s\n", filepath.Join(paths.stateDir, ".env"), paths.dotEnvPath)
	}

	tokensUsed := cmd == "auth" || cmd == "collect" || cmd == "persona" || cmd == "genre-backfill" || cmd == "doctor"
	if tokensUsed && paths.tokensFallback {
		fmt.Fprintf(os.Stderr, "warning: SPOTIFY_STATE_DIR is set but %s was not found; falling back to %s\n", filepath.Join(paths.stateDir, "tokens.json"), paths.tokensPath)
	}

	playsUsed := cmd == "collect" || cmd == "weekly" || cmd == "daily" || cmd == "catch-up" || cmd == "persona" || cmd == "genre-backfill" || cmd == "doctor"
	if playsUsed && paths.playsFallback {
		fmt.Fprintf(os.Stderr, "warning: SPOTIFY_STATE_DIR is set but %s was not found; falling back to %s\n", filepath.Join(paths.stateDir, "data", "plays"), paths.playsDir)
	}

	genresUsed := cmd == "collect" || cmd == "weekly" || cmd == "daily" || cmd == "catch-up" || cmd == "persona" || cmd == "genre-backfill"
	if genresUsed && paths.genresFallback {
		fmt.Fprintf(os.Stderr, "warning: SPOTIFY_STATE_DIR is set but %s was not found; falling back to %s\n", filepath.Join(paths.stateDir, "data", "genres.json"), paths.genresPath)
	}
}

// loadDotEnv reads a .env file and sets environment variables.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // .env is optional
		}
		return err
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

	return scanner.Err()
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
func getClient(paths runtimePaths) (*client.Client, error) {
	token, err := auth.RefreshIfNeeded(paths.tokensPath)
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
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q (expected YYYY-MM-DD): %w", s, err)
	}
	return t, nil
}

func envTrue(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// ensurePlaysDir creates the parent directory for plays.json.
func ensurePlaysDir(playsPath string) error {
	return os.MkdirAll(filepath.Dir(playsPath), 0755)
}

// --- Subcommands ---

func runAuth(paths runtimePaths) {
	if err := auth.StartAuthFlow(paths.tokensPath); err != nil {
		fmt.Fprintln(os.Stderr, "auth failed:", err)
		os.Exit(1)
	}
}

func runCollect(paths runtimePaths) {
	c, err := getClient(paths)
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

	// Migrate legacy plays.json to sharded structure on first use.
	if err := plays.MigrateToSharded(paths.playsPath, paths.playsDir); err != nil {
		fmt.Fprintln(os.Stderr, "migration error:", err)
		os.Exit(1)
	}

	newCount, err := plays.SaveSharded(paths.playsDir, incoming)
	if err != nil {
		fmt.Fprintln(os.Stderr, "save error:", err)
		os.Exit(1)
	}

	fmt.Printf("Added %d new plays.\n", newCount)

	// Load all plays for genre cache update and optional daily note.
	allPlays, err := plays.LoadSharded(paths.playsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: load plays: %v\n", err)
		allPlays = incoming
	}

	// Update genre cache for any new artists
	genreCache, err := genres.Load(paths.genresPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: load genre cache: %v\n", err)
		genreCache = map[string]genres.Entry{}
	}
	uncached := genres.UncachedArtistIDs(genreCache, incoming)
	if len(uncached) > 0 {
		fmt.Printf("Fetching genres for %d new artist(s)...\n", len(uncached))
		artists, err := fetch.GetArtistsBatch(c, uncached)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: fetch artist genres: %v\n", err)
		} else {
			for _, a := range artists {
				genres.Update(genreCache, a.ID, a.Name, a.Genres)
			}
			if err := genres.Save(paths.genresPath, genreCache); err != nil {
				fmt.Fprintf(os.Stderr, "warning: save genre cache: %v\n", err)
			}
		}
	}

	if envTrue("SPOTIFY_AUTO_DAILY_ON_COLLECT") {
		if os.Getenv("OBSIDIAN_VAULT_PATH") == "" {
			fmt.Fprintln(os.Stderr, "warning: SPOTIFY_AUTO_DAILY_ON_COLLECT is enabled but OBSIDIAN_VAULT_PATH is not set")
			return
		}
		ag := genres.GenresForPlays(genreCache, allPlays)
		generateDailyNote(allPlays, time.Now(), true, ag)
	}
}

func runWeekly(args []string, paths runtimePaths) {
	fs := flag.NewFlagSet("weekly", flag.ExitOnError)
	dateStr := fs.String("date", "", "any date within the target week (default: this week)")
	_ = fs.Parse(args)

	date, err := parseDate(*dateStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	generateWeeklyNote(date, paths)
}

// generateWeeklyNote filters local plays and writes the weekly summary note.
func generateWeeklyNote(date time.Time, paths runtimePaths) {
	monday, _ := render.WeekBounds(date)
	weekStr := render.WeekStr(monday)

	vault := vaultPath()
	outDir := filepath.Join(vault, "music", "listening")
	outPath := filepath.Join(outDir, fmt.Sprintf("spotify-%s.md", weekStr))

	allPlays, err := plays.LoadSharded(paths.playsDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load plays error:", err)
		os.Exit(1)
	}

	weekPlays := render.PlaysForWeek(allPlays, date)
	fmt.Printf("Plays for week %s: %d\n", weekStr, len(weekPlays))

	// Load genre cache
	genreCache, err := genres.Load(paths.genresPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: load genre cache: %v\n", err)
		genreCache = map[string]genres.Entry{}
	}
	artistGenres := genres.GenresForPlays(genreCache, weekPlays)

	content, err := render.RenderWeekly(allPlays, date, vault, artistGenres)
	if err != nil {
		fmt.Fprintln(os.Stderr, "render error:", err)
		os.Exit(1)
	}

	// Update artist stubs with genres
	for name, g := range artistGenres {
		if err := render.UpdateArtistGenres(name, g, vault); err != nil {
			fmt.Fprintf(os.Stderr, "warning: update genres for %s: %v\n", name, err)
		}
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

func generateDailyNote(allPlays []models.Play, date time.Time, overwrite bool, artistGenres map[string][]string) {
	d := date.Local()
	dayStr := d.Format("2006-01-02")
	vault := vaultPath()
	outDir := filepath.Join(vault, "music", "listening")
	outPath := filepath.Join(outDir, fmt.Sprintf("spotify-%s.md", dayStr))
	dayPlays := render.PlaysForDay(allPlays, date)
	if len(dayPlays) == 0 {
		return // no plays for this day
	}

	artistURLs := map[string]string{}
	for _, p := range dayPlays {
		if _, ok := artistURLs[p.ArtistName]; !ok {
			artistURLs[p.ArtistName] = p.ArtistSpotifyURL
		}
	}
	artists := make([]string, 0, len(artistURLs))
	for name := range artistURLs {
		artists = append(artists, name)
	}
	sort.Strings(artists)
	for _, name := range artists {
		var g []string
		if artistGenres != nil {
			g = artistGenres[name]
		}
		if err := render.EnsureArtistStub(name, artistURLs[name], g, dayStr, vault); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not create artist stub for %s: %v\n", name, err)
		}
		if len(g) > 0 {
			if err := render.UpdateArtistGenres(name, g, vault); err != nil {
				fmt.Fprintf(os.Stderr, "warning: update genres for %s: %v\n", name, err)
			}
		}
	}

	// Skip existing daily notes unless overwrite is enabled.
	if _, err := os.Stat(outPath); err == nil && !overwrite {
		fmt.Printf("  Skipping %s (already exists)\n", dayStr)
		return
	}

	content, err := render.RenderDaily(allPlays, date, vault, artistGenres)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render error for %s: %v\n", dayStr, err)
		return
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir error: %v\n", err)
		return
	}
	if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write error for %s: %v\n", dayStr, err)
		return
	}
	if overwrite {
		fmt.Println("Updated:", outPath)
	} else {
		fmt.Println("Written:", outPath)
	}
}

func runDaily(args []string, paths runtimePaths) {
	fs := flag.NewFlagSet("daily", flag.ExitOnError)
	dateStr := fs.String("date", "", "date in YYYY-MM-DD format (default: today)")
	_ = fs.Parse(args)

	date, err := parseDate(*dateStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	allPlays, err := plays.LoadSharded(paths.playsDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load plays error:", err)
		os.Exit(1)
	}

	genreCache, err := genres.Load(paths.genresPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: load genre cache: %v\n", err)
		genreCache = map[string]genres.Entry{}
	}
	artistGenres := genres.GenresForPlays(genreCache, allPlays)

	generateDailyNote(allPlays, date, false, artistGenres)
}

func weeklyNotePath(listeningDir string, date time.Time) string {
	monday, _ := render.WeekBounds(date)
	weekStr := render.WeekStr(monday)
	return filepath.Join(listeningDir, fmt.Sprintf("spotify-%s.md", weekStr))
}

func dailyNotePath(listeningDir string, date time.Time) string {
	dayStr := date.Local().Format("2006-01-02")
	return filepath.Join(listeningDir, fmt.Sprintf("spotify-%s.md", dayStr))
}

func missingWeeklyDates(listeningDir string, now time.Time, weeks int) []time.Time {
	missingDates := make([]time.Time, 0, weeks)
	for i := 0; i < weeks; i++ {
		weekDate := now.AddDate(0, 0, -(i * 7))
		notePath := weeklyNotePath(listeningDir, weekDate)
		if _, err := os.Stat(notePath); os.IsNotExist(err) {
			missingDates = append(missingDates, weekDate)
		}
	}
	return missingDates
}

func missingDailyDates(listeningDir string, now time.Time, totalDays int) []time.Time {
	missingDays := make([]time.Time, 0, totalDays)
	for i := 0; i < totalDays; i++ {
		day := now.AddDate(0, 0, -i)
		notePath := dailyNotePath(listeningDir, day)
		if _, err := os.Stat(notePath); os.IsNotExist(err) {
			missingDays = append(missingDays, day)
		}
	}
	return missingDays
}

func generateMissingWeeklyNotes(paths runtimePaths, missingDates []time.Time) {
	if len(missingDates) == 0 {
		fmt.Println("Weekly notes: all caught up.")
		return
	}
	fmt.Printf("Found %d missing weekly note(s), generating...\n", len(missingDates))
	for i := len(missingDates) - 1; i >= 0; i-- {
		generateWeeklyNote(missingDates[i], paths)
	}
}

func generateMissingDailyNotes(allPlays []models.Play, missingDays []time.Time, totalDays int, artistGenres map[string][]string) {
	fmt.Printf("Checking %d days for missing daily notes...\n", totalDays)
	for _, day := range missingDays {
		generateDailyNote(allPlays, day, false, artistGenres)
	}
}

func runCatchUp(args []string, paths runtimePaths) {
	fs := flag.NewFlagSet("catch-up", flag.ExitOnError)
	weeks := fs.Int("weeks", 8, "number of weeks to check")
	_ = fs.Parse(args)

	vault := vaultPath()
	listeningDir := filepath.Join(vault, "music", "listening")
	now := time.Now()

	missingWeeks := missingWeeklyDates(listeningDir, now, *weeks)
	generateMissingWeeklyNotes(paths, missingWeeks)

	allPlays, err := plays.LoadSharded(paths.playsDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load plays error:", err)
		os.Exit(1)
	}
	genreCache, err := genres.Load(paths.genresPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: load genre cache: %v\n", err)
		genreCache = map[string]genres.Entry{}
	}
	artistGenres := genres.GenresForPlays(genreCache, allPlays)

	totalDays := *weeks * 7
	missingDays := missingDailyDates(listeningDir, now, totalDays)
	generateMissingDailyNotes(allPlays, missingDays, totalDays, artistGenres)
	fmt.Println("Done.")
}

func runGenreBackfill(paths runtimePaths) {
	c, err := getClient(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	allPlays, err := plays.LoadSharded(paths.playsDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load plays error:", err)
		os.Exit(1)
	}

	if err := ensurePlaysDir(paths.genresPath); err != nil {
		fmt.Fprintln(os.Stderr, "data dir error:", err)
		os.Exit(1)
	}

	genreCache, err := genres.Load(paths.genresPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load genre cache error:", err)
		os.Exit(1)
	}

	uncached := genres.UncachedArtistIDs(genreCache, allPlays)
	fmt.Printf("Total plays: %d, cached artists: %d, uncached: %d\n", len(allPlays), len(genreCache), len(uncached))

	if len(uncached) == 0 {
		fmt.Println("All artists already cached.")
	} else {
		fmt.Printf("Fetching genres for %d artist(s)...\n", len(uncached))
		artists, err := fetch.GetArtistsBatch(c, uncached)
		if err != nil {
			fmt.Fprintln(os.Stderr, "fetch error:", err)
			os.Exit(1)
		}
		for _, a := range artists {
			genres.Update(genreCache, a.ID, a.Name, a.Genres)
		}
		if err := genres.Save(paths.genresPath, genreCache); err != nil {
			fmt.Fprintln(os.Stderr, "save genre cache error:", err)
			os.Exit(1)
		}
		fmt.Printf("Cached %d artist(s).\n", len(artists))
	}

	// Update all existing artist stubs
	vault := vaultPath()
	updated := 0
	for _, entry := range genreCache {
		if len(entry.Genres) > 0 {
			if err := render.UpdateArtistGenres(entry.Name, entry.Genres, vault); err != nil {
				fmt.Fprintf(os.Stderr, "warning: update genres for %s: %v\n", entry.Name, err)
			} else {
				updated++
			}
		}
	}
	fmt.Printf("Updated %d artist stub(s).\n", updated)
}

func runSetlist(args []string) {
	fs := flag.NewFlagSet("setlist", flag.ExitOnError)
	dateStr := fs.String("date", "", "date in YYYY-MM-DD format (default: today)")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: spotify-garden setlist <artist> [--date YYYY-MM-DD]")
		os.Exit(1)
	}
	artist := fs.Arg(0)

	date, err := parseDate(*dateStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	dateFormatted := date.Format("2006-01-02")

	setlist, err := fetch.GetSetlist(artist, dateFormatted)
	if err != nil {
		fmt.Fprintln(os.Stderr, "setlist.fm error:", err)
		os.Exit(1)
	}

	fmt.Printf("%s — %s — %s\n", setlist.ArtistName, setlist.VenueName, setlist.CityName)
	fmt.Printf("%s\n\n", dateFormatted)

	for _, s := range setlist.Sets {
		if s.Name != "" {
			fmt.Printf("%s:\n", s.Name)
		} else {
			fmt.Printf("Set 1:\n")
		}
		for i, song := range s.Songs {
			fmt.Printf("%d. %s\n", i+1, song)
		}
		fmt.Println()
	}

	if setlist.URL != "" {
		fmt.Printf("Setlist.fm: %s\n", setlist.URL)
	}
}

func runPersona(paths runtimePaths) {
	vault := vaultPath()
	outPath := filepath.Join(vault, "01-ai-brain", "context-packs", "Music Taste.md")
	tmplPath := filepath.Join(templatesDir(), "persona.md.tmpl")

	c, err := getClient(paths)
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

	allPlays, err := plays.LoadSharded(paths.playsDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load plays error:", err)
		os.Exit(1)
	}
	weekPlays := render.PlaysForWeek(allPlays, time.Now())

	// Update genre cache from top artists
	genreCache, err := genres.Load(paths.genresPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: load genre cache: %v\n", err)
		genreCache = map[string]genres.Entry{}
	}
	allTopArtists := append(append(topArtistsShort, topArtistsMedium...), topArtistsLong...)
	for _, a := range allTopArtists {
		if a.ID != "" {
			genres.Update(genreCache, a.ID, a.Name, a.Genres)
		}
	}
	if err := ensurePlaysDir(paths.genresPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: data dir error: %v\n", err)
	} else if err := genres.Save(paths.genresPath, genreCache); err != nil {
		fmt.Fprintf(os.Stderr, "warning: save genre cache: %v\n", err)
	}

	// Update artist stubs with genres
	for _, a := range allTopArtists {
		if len(a.Genres) > 0 {
			if err := render.UpdateArtistGenres(a.Name, a.Genres, vault); err != nil {
				fmt.Fprintf(os.Stderr, "warning: update genres for %s: %v\n", a.Name, err)
			}
		}
	}

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

func runDoctor(paths runtimePaths) int {
	issues := 0

	fmt.Println("Runtime configuration")
	fmt.Println("---------------------")
	fmt.Println("Working directory:", paths.cwd)
	fmt.Println("Executable:", executablePath())
	if paths.stateDir == "" {
		fmt.Println("SPOTIFY_STATE_DIR: (not set)")
	} else {
		fmt.Println("SPOTIFY_STATE_DIR:", paths.stateDir)
	}

	printPathStatus("Dotenv path", paths.dotEnvPath, true)
	printPathStatus("Tokens path", paths.tokensPath, false)
	printPathStatus("Plays dir", paths.playsDir, false)
	printPathStatus("Plays legacy", paths.playsPath, true)

	templates := templatesDir()
	printPathStatus("Templates dir", templates, false)

	vault := strings.TrimSpace(os.Getenv("OBSIDIAN_VAULT_PATH"))
	if vault == "" {
		fmt.Println("Vault path: (OBSIDIAN_VAULT_PATH not set)")
		issues++
	} else {
		listeningDir := filepath.Join(vault, "music", "listening")
		fmt.Println("Vault path:", vault)
		fmt.Println("Listening output:", listeningDir)
	}

	if paths.dotEnvFallback {
		issues++
		fmt.Printf("Warning: using CWD fallback for .env (%s) because %s is missing\n", paths.dotEnvPath, filepath.Join(paths.stateDir, ".env"))
	}
	if paths.tokensFallback {
		issues++
		fmt.Printf("Warning: using CWD fallback for tokens.json (%s) because %s is missing\n", paths.tokensPath, filepath.Join(paths.stateDir, "tokens.json"))
	}
	if paths.playsFallback {
		issues++
		fmt.Printf("Warning: using CWD fallback for plays dir (%s) because %s is missing\n", paths.playsDir, filepath.Join(paths.stateDir, "data", "plays"))
	}

	if strings.TrimSpace(os.Getenv("SPOTIFY_CLIENT_ID")) == "" {
		fmt.Println("Warning: SPOTIFY_CLIENT_ID is not set")
		issues++
	}
	if strings.TrimSpace(os.Getenv("SPOTIFY_CLIENT_SECRET")) == "" {
		fmt.Println("Warning: SPOTIFY_CLIENT_SECRET is not set")
		issues++
	}

	collectLabel, weeklyLabel, collectLog, weeklyLog := launchdDefaults()
	fmt.Println("Launchd collect label:", collectLabel)
	fmt.Println("Launchd weekly label:", weeklyLabel)
	fmt.Println("Collect log path:", collectLog)
	fmt.Println("Weekly log path:", weeklyLog)

	if status, err := launchdJobStatus(collectLabel); err == nil {
		fmt.Println("Collect job status:", status)
	} else {
		fmt.Println("Collect job status: unknown (", err, ")")
	}
	if status, err := launchdJobStatus(weeklyLabel); err == nil {
		fmt.Println("Weekly job status:", status)
	} else {
		fmt.Println("Weekly job status: unknown (", err, ")")
	}

	if issues > 0 {
		fmt.Printf("\nDoctor found %d issue(s).\n", issues)
		return 1
	}
	fmt.Println("\nDoctor found no issues.")
	return 0
}

func executablePath() string {
	exe, err := os.Executable()
	if err != nil {
		return "(unknown)"
	}
	return exe
}

func printPathStatus(label, path string, optional bool) {
	fi, err := os.Stat(path)
	if err == nil {
		kind := "file"
		if fi.IsDir() {
			kind = "dir"
		}
		fmt.Printf("%s: %s (%s, present)\n", label, path, kind)
		return
	} else if os.IsNotExist(err) {
		if optional {
			fmt.Printf("%s: %s (missing, optional)\n", label, path)
		} else {
			fmt.Printf("%s: %s (missing)\n", label, path)
		}
		return
	}
	fmt.Printf("%s: %s (error: %v)\n", label, path, err)
}

func launchdDefaults() (collectLabel, weeklyLabel, collectLog, weeklyLog string) {
	user := strings.TrimSpace(os.Getenv("USER"))
	if user == "" {
		user = "unknown"
	}

	collectLabel = os.Getenv("SPOTIFY_COLLECT_LABEL")
	if collectLabel == "" {
		collectLabel = fmt.Sprintf("com.%s.spotify-collect", user)
	}
	weeklyLabel = os.Getenv("SPOTIFY_WEEKLY_LABEL")
	if weeklyLabel == "" {
		weeklyLabel = fmt.Sprintf("com.%s.spotify-weekly", user)
	}

	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, "Library", "Application Support", "spotify-garden", "logs")
	collectLog = filepath.Join(logDir, "collect.log")
	weeklyLog = filepath.Join(logDir, "weekly.log")

	return collectLabel, weeklyLabel, collectLog, weeklyLog
}

func launchdJobStatus(label string) (string, error) {
	if _, err := exec.LookPath("launchctl"); err != nil {
		return "", fmt.Errorf("launchctl not found")
	}
	cmd := exec.Command("launchctl", "list", label)
	if err := cmd.Run(); err != nil {
		return "not loaded", nil
	}
	return "loaded", nil
}
