package fetch

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/benstraw/spotify-garden/internal/client"
	"github.com/benstraw/spotify-garden/internal/models"
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

type topArtistItem struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Genres       []string          `json:"genres"`
	ExternalURLs map[string]string `json:"external_urls"`
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
		})
	}
	return artists, nil
}
