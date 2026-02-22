package models

// Play represents a single Spotify track play event.
type Play struct {
	PlayedAt         string `json:"played_at"`
	TrackID          string `json:"track_id"`
	TrackName        string `json:"track_name"`
	ArtistID         string `json:"artist_id"`
	ArtistName       string `json:"artist_name"`
	ArtistSpotifyURL string `json:"artist_spotify_url"`
	AlbumName        string `json:"album_name"`
	DurationMS       int    `json:"duration_ms"`
	TrackSpotifyURL  string `json:"track_spotify_url"`
}

// TopTrack represents a track from the user's top tracks.
type TopTrack struct {
	ID         string
	Name       string
	ArtistName string
}

// TopArtist represents an artist from the user's top artists.
type TopArtist struct {
	ID         string
	Name       string
	Genres     []string
	SpotifyURL string
}

// Setlist represents a setlist.fm setlist result.
type Setlist struct {
	EventDate  string // "DD-MM-YYYY" from API
	ArtistName string
	VenueName  string
	CityName   string
	URL        string // setlist.fm URL
	Sets       []SetlistSet
}

// SetlistSet represents one set (main set or encore) within a setlist.
type SetlistSet struct {
	Name  string // "Encore", "" for main set
	Songs []string
}
