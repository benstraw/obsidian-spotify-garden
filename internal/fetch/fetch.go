package fetch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/benstraw/music-garden/internal/client"
	"github.com/benstraw/music-garden/internal/models"
)

// --- Spotify API response types ---

type recentlyPlayedResponse struct {
	Items []recentlyPlayedItem `json:"items"`
}

type recentlyPlayedItem struct {
	Track    *spotifyTrack `json:"track"` // nil for podcasts
	PlayedAt string        `json:"played_at"`
}

type spotifyTrack struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Artists      []spotifyArtist   `json:"artists"`
	Album        spotifyAlbum      `json:"album"`
	DurationMS   int               `json:"duration_ms"`
	ExternalURLs map[string]string `json:"external_urls"`
}

type spotifyArtist struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Genres       []string          `json:"genres"`
	ExternalURLs map[string]string `json:"external_urls"`
}

type spotifyAlbum struct {
	Name string `json:"name"`
}

type topTracksResponse struct {
	Items []topTrackItem `json:"items"`
}

type topTrackItem struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Artists []spotifyArtist `json:"artists"`
}

type topArtistsResponse struct {
	Items []topArtistItem `json:"items"`
}

type spotifyImage struct {
	URL    string `json:"url"`
	Height int    `json:"height"`
	Width  int    `json:"width"`
}

type topArtistItem struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Genres       []string          `json:"genres"`
	ExternalURLs map[string]string `json:"external_urls"`
	Images       []spotifyImage    `json:"images"`
}

func toModelImages(imgs []spotifyImage) []models.ArtistImage {
	result := make([]models.ArtistImage, 0, len(imgs))
	for _, img := range imgs {
		result = append(result, models.ArtistImage{
			URL:    img.URL,
			Height: img.Height,
			Width:  img.Width,
		})
	}
	return result
}

// GetRecentlyPlayed fetches up to 50 recently played tracks.
// Podcast episodes (items with no track key) are filtered silently.
func GetRecentlyPlayed(c *client.Client) ([]models.Play, error) {
	params := url.Values{}
	params.Set("limit", "50")

	body, err := c.Get("/me/player/recently-played", params)
	if err != nil {
		return nil, fmt.Errorf("recently-played: %w", err)
	}

	var resp recentlyPlayedResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("recently-played decode: %w", err)
	}

	var plays []models.Play
	for _, item := range resp.Items {
		if item.Track == nil {
			continue // podcast episode — skip silently
		}
		p := itemToPlay(item)
		plays = append(plays, p)
	}
	return plays, nil
}

// itemToPlay maps a recently-played API item to a Play struct (primary artist only).
func itemToPlay(item recentlyPlayedItem) models.Play {
	t := item.Track
	var artistID, artistName, artistURL string
	if len(t.Artists) > 0 {
		a := t.Artists[0]
		artistID = a.ID
		artistName = a.Name
		artistURL = a.ExternalURLs["spotify"]
	}
	trackURL := t.ExternalURLs["spotify"]

	return models.Play{
		PlayedAt:         item.PlayedAt,
		TrackID:          t.ID,
		TrackName:        t.Name,
		ArtistID:         artistID,
		ArtistName:       artistName,
		ArtistSpotifyURL: artistURL,
		AlbumName:        t.Album.Name,
		DurationMS:       t.DurationMS,
		TrackSpotifyURL:  trackURL,
	}
}

// GetTopTracks fetches the user's top 50 tracks for the given time range.
// timeRange: "short_term" | "medium_term" | "long_term"
func GetTopTracks(c *client.Client, timeRange string) ([]models.TopTrack, error) {
	params := url.Values{}
	params.Set("limit", "50")
	params.Set("time_range", timeRange)

	body, err := c.Get("/me/top/tracks", params)
	if err != nil {
		return nil, fmt.Errorf("top/tracks: %w", err)
	}

	var resp topTracksResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("top/tracks decode: %w", err)
	}

	tracks := make([]models.TopTrack, 0, len(resp.Items))
	for _, item := range resp.Items {
		var artistName string
		if len(item.Artists) > 0 {
			artistName = item.Artists[0].Name
		}
		tracks = append(tracks, models.TopTrack{
			ID:         item.ID,
			Name:       item.Name,
			ArtistName: artistName,
		})
	}
	return tracks, nil
}

// --- Batch artist lookup ---

type artistsResponse struct {
	Artists []topArtistItem `json:"artists"`
}

// GetArtists fetches artist details for up to 50 IDs in a single request.
func GetArtists(c *client.Client, ids []string) ([]models.TopArtist, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > 50 {
		return nil, fmt.Errorf("GetArtists: max 50 IDs per request, got %d", len(ids))
	}

	params := url.Values{}
	params.Set("ids", joinIDs(ids))

	body, err := c.Get("/artists", params)
	if err != nil {
		return nil, fmt.Errorf("artists: %w", err)
	}

	var resp artistsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("artists decode: %w", err)
	}

	artists := make([]models.TopArtist, 0, len(resp.Artists))
	for _, item := range resp.Artists {
		artists = append(artists, models.TopArtist{
			ID:         item.ID,
			Name:       item.Name,
			Genres:     item.Genres,
			SpotifyURL: item.ExternalURLs["spotify"],
			Images:     toModelImages(item.Images),
		})
	}
	return artists, nil
}

// GetArtistsBatch fetches artist details for any number of IDs, chunking into batches of 50.
func GetArtistsBatch(c *client.Client, ids []string) ([]models.TopArtist, error) {
	var all []models.TopArtist
	for i := 0; i < len(ids); i += 50 {
		end := min(i+50, len(ids))
		batch, err := GetArtists(c, ids[i:end])
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
	}
	return all, nil
}

// joinIDs joins IDs with commas.
func joinIDs(ids []string) string {
	return strings.Join(ids, ",")
}

// --- Setlist.fm ---

// setlist.fm API response types (internal use only).
type setlistfmResponse struct {
	Setlist []setlistfmSetlist `json:"setlist"`
}

type setlistfmSetlist struct {
	EventDate string            `json:"eventDate"` // "DD-MM-YYYY"
	Artist    setlistfmArtist   `json:"artist"`
	Venue     setlistfmVenue    `json:"venue"`
	URL       string            `json:"url"`
	Sets      setlistfmSetsCont `json:"sets"`
}

type setlistfmArtist struct {
	Name string `json:"name"`
}

type setlistfmVenue struct {
	Name string        `json:"name"`
	City setlistfmCity `json:"city"`
}

type setlistfmCity struct {
	Name      string           `json:"name"`
	StateCode string           `json:"stateCode"`
	Country   setlistfmCountry `json:"country"`
}

type setlistfmCountry struct {
	Code string `json:"code"`
}

type setlistfmSetsCont struct {
	Set []setlistfmSet `json:"set"`
}

type setlistfmSet struct {
	Name string          `json:"name"` // "" for main set, "Encore" etc.
	Song []setlistfmSong `json:"song"`
}

type setlistfmSong struct {
	Name string `json:"name"`
}

// setlistGet performs a GET request to the setlist.fm REST API.
func setlistGet(path string, params url.Values) ([]byte, error) {
	apiKey := os.Getenv("SETLISTFM_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("SETLISTFM_API_KEY not set")
	}

	reqURL := "https://api.setlist.fm/rest/1.0" + path
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("setlist.fm build request: %w", err)
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Accept", "application/json")

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("setlist.fm request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("setlist.fm read body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no setlist found")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("setlist.fm returned %d", resp.StatusCode)
	}
	return body, nil
}

// GetSetlist fetches the most recent setlist for artistName on date (YYYY-MM-DD).
// Returns the first matching result from setlist.fm.
func GetSetlist(artistName, date string) (models.Setlist, error) {
	// Convert YYYY-MM-DD to DD-MM-YYYY for setlist.fm
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return models.Setlist{}, fmt.Errorf("invalid date %q: %w", date, err)
	}
	apiDate := t.Format("02-01-2006")

	params := url.Values{}
	params.Set("artistName", artistName)
	params.Set("date", apiDate)
	params.Set("p", "1")

	body, err := setlistGet("/search/setlists", params)
	if err != nil {
		return models.Setlist{}, err
	}

	var resp setlistfmResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return models.Setlist{}, fmt.Errorf("setlist.fm decode: %w", err)
	}

	if len(resp.Setlist) == 0 {
		return models.Setlist{}, fmt.Errorf("no setlist found for %q on %s", artistName, date)
	}

	raw := resp.Setlist[0]

	city := raw.Venue.City.Name
	if raw.Venue.City.StateCode != "" {
		city += ", " + raw.Venue.City.StateCode
	}

	var sets []models.SetlistSet
	for _, s := range raw.Sets.Set {
		var songs []string
		for _, song := range s.Song {
			songs = append(songs, song.Name)
		}
		sets = append(sets, models.SetlistSet{
			Name:  s.Name,
			Songs: songs,
		})
	}

	return models.Setlist{
		EventDate:  raw.EventDate,
		ArtistName: raw.Artist.Name,
		VenueName:  raw.Venue.Name,
		CityName:   city,
		URL:        raw.URL,
		Sets:       sets,
	}, nil
}

// GetTopArtists fetches the user's top 50 artists for the given time range.
// timeRange: "short_term" | "medium_term" | "long_term"
func GetTopArtists(c *client.Client, timeRange string) ([]models.TopArtist, error) {
	params := url.Values{}
	params.Set("limit", "50")
	params.Set("time_range", timeRange)

	body, err := c.Get("/me/top/artists", params)
	if err != nil {
		return nil, fmt.Errorf("top/artists: %w", err)
	}

	var resp topArtistsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("top/artists decode: %w", err)
	}

	artists := make([]models.TopArtist, 0, len(resp.Items))
	for _, item := range resp.Items {
		artists = append(artists, models.TopArtist{
			ID:         item.ID,
			Name:       item.Name,
			Genres:     item.Genres,
			SpotifyURL: item.ExternalURLs["spotify"],
			Images:     toModelImages(item.Images),
		})
	}
	return artists, nil
}
