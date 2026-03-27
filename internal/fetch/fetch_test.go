package fetch

import (
	"testing"

	"github.com/benstraw/spotify-garden/internal/models"
)

func TestItemToPlay_primaryArtist(t *testing.T) {
	item := recentlyPlayedItem{
		PlayedAt: "2026-02-21T14:30:00.000Z",
		Track: &spotifyTrack{
			ID:   "track1",
			Name: "My Song",
			Artists: []spotifyArtist{
				{ID: "a1", Name: "Primary Artist", ExternalURLs: map[string]string{"spotify": "https://open.spotify.com/artist/a1"}},
				{ID: "a2", Name: "Featured Artist"},
			},
			Album:        spotifyAlbum{Name: "My Album"},
			DurationMS:   210000,
			ExternalURLs: map[string]string{"spotify": "https://open.spotify.com/track/track1"},
		},
	}

	p := itemToPlay(item)

	if p.PlayedAt != "2026-02-21T14:30:00.000Z" {
		t.Errorf("PlayedAt = %q", p.PlayedAt)
	}
	if p.TrackID != "track1" {
		t.Errorf("TrackID = %q", p.TrackID)
	}
	if p.TrackName != "My Song" {
		t.Errorf("TrackName = %q", p.TrackName)
	}
	if p.ArtistID != "a1" {
		t.Errorf("ArtistID = %q, want primary artist only", p.ArtistID)
	}
	if p.ArtistName != "Primary Artist" {
		t.Errorf("ArtistName = %q", p.ArtistName)
	}
	if p.ArtistSpotifyURL != "https://open.spotify.com/artist/a1" {
		t.Errorf("ArtistSpotifyURL = %q", p.ArtistSpotifyURL)
	}
	if p.AlbumName != "My Album" {
		t.Errorf("AlbumName = %q", p.AlbumName)
	}
	if p.DurationMS != 210000 {
		t.Errorf("DurationMS = %d", p.DurationMS)
	}
	if p.TrackSpotifyURL != "https://open.spotify.com/track/track1" {
		t.Errorf("TrackSpotifyURL = %q", p.TrackSpotifyURL)
	}
}

func TestItemToPlay_noArtists(t *testing.T) {
	item := recentlyPlayedItem{
		PlayedAt: "2026-02-21T14:30:00.000Z",
		Track: &spotifyTrack{
			ID:      "track1",
			Name:    "My Song",
			Artists: []spotifyArtist{},
		},
	}

	p := itemToPlay(item)

	if p.ArtistID != "" || p.ArtistName != "" || p.ArtistSpotifyURL != "" {
		t.Errorf("expected empty artist fields for track with no artists, got id=%q name=%q url=%q",
			p.ArtistID, p.ArtistName, p.ArtistSpotifyURL)
	}
}

func TestItemToPlay_noExternalURLs(t *testing.T) {
	item := recentlyPlayedItem{
		PlayedAt: "2026-02-21T14:30:00.000Z",
		Track: &spotifyTrack{
			ID:      "track1",
			Name:    "My Song",
			Artists: []spotifyArtist{{ID: "a1", Name: "Artist"}},
		},
	}

	p := itemToPlay(item)

	if p.TrackSpotifyURL != "" {
		t.Errorf("expected empty TrackSpotifyURL, got %q", p.TrackSpotifyURL)
	}
	if p.ArtistSpotifyURL != "" {
		t.Errorf("expected empty ArtistSpotifyURL, got %q", p.ArtistSpotifyURL)
	}
}

func TestToModelImages(t *testing.T) {
	imgs := []spotifyImage{
		{URL: "https://img-640", Height: 640, Width: 640},
		{URL: "https://img-320", Height: 320, Width: 320},
	}

	got := toModelImages(imgs)
	want := []models.ArtistImage{
		{URL: "https://img-640", Height: 640, Width: 640},
		{URL: "https://img-320", Height: 320, Width: 320},
	}

	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
