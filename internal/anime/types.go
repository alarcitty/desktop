package anime

import (
	"mila/internal/aniliberty"
	"mila/internal/shikimori"
)

// PosterSet contains best available poster URLs merged from Shikimori and AniLiberty.
type PosterSet struct {
	Original  string `json:"original,omitempty"`
	Main      string `json:"main,omitempty"`
	Preview   string `json:"preview,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty"`
}

// BannerImage represents a widescreen screenshot/banner from Shikimori or AniLiberty.
type BannerImage struct {
	Original string `json:"original"`
	Preview  string `json:"preview,omitempty"`
}

// Episode represents a normalized playable episode with HLS streams and skip intervals.
type Episode struct {
	ID        string                `json:"id"`
	Ordinal   float64               `json:"ordinal"`
	Name      string                `json:"name,omitempty"`
	Duration  int                   `json:"duration"`
	Preview   string                `json:"preview,omitempty"`
	Opening   *aniliberty.TimeRange `json:"opening,omitempty"`
	Ending    *aniliberty.TimeRange `json:"ending,omitempty"`
	HLS480    string                `json:"hls_480,omitempty"`
	HLS720    string                `json:"hls_720,omitempty"`
	HLS1080   string                `json:"hls_1080,omitempty"`
	UpdatedAt string                `json:"updated_at,omitempty"`
}

// Anime represents a unified anime summary combining AniLiberty playback metadata and Shikimori art/descriptions.
type Anime struct {
	ID            int           `json:"id"`
	Alias         string        `json:"alias"`
	ShikimoriID   int           `json:"shikimori_id,omitempty"`
	MalID         int           `json:"mal_id,omitempty"`
	Title         string        `json:"title"`
	TitleEnglish  string        `json:"title_english,omitempty"`
	TitleOriginal string        `json:"title_original,omitempty"`
	Description   string        `json:"description,omitempty"`
	Year          int           `json:"year,omitempty"`
	Season        string        `json:"season,omitempty"`
	Type          string        `json:"type,omitempty"`
	Status        string        `json:"status,omitempty"`
	AgeRating     string        `json:"age_rating,omitempty"`
	IsOngoing     bool          `json:"is_ongoing"`
	EpisodesTotal *int          `json:"episodes_total,omitempty"`
	EpisodesAired int           `json:"episodes_aired,omitempty"`
	Duration      int           `json:"duration,omitempty"`
	Score         float64       `json:"score,omitempty"`
	NextEpisodeAt string        `json:"next_episode_at,omitempty"`
	Poster        PosterSet     `json:"poster"`
	Banner        string        `json:"banner,omitempty"`
	Banners       []BannerImage `json:"banners,omitempty"`
	Genres        []string      `json:"genres,omitempty"`
	Studios       []string      `json:"studios,omitempty"`
	LatestEpisode *Episode      `json:"latest_episode,omitempty"`
	ShikimoriURL  string        `json:"shikimori_url,omitempty"`
}

// Detail represents full anime information including all playable episodes, videos, and torrents.
type Detail struct {
	Anime
	Episodes []Episode            `json:"episodes"`
	Videos   []shikimori.Video    `json:"videos,omitempty"`
	Torrents []aniliberty.Torrent `json:"torrents,omitempty"`
}

// CatalogPage is a paginated list of unified Anime entries.
type CatalogPage struct {
	Items      []Anime               `json:"items"`
	Pagination aniliberty.Pagination `json:"pagination"`
}

// ScheduleEntry combines a release summary with its scheduled episode info.
type ScheduleEntry struct {
	Anime             Anime    `json:"anime"`
	FullSeason        bool     `json:"full_season_is_released"`
	PublishedEpisode  *Episode `json:"published_episode,omitempty"`
	NextEpisodeNumber *int     `json:"next_episode_number,omitempty"`
}

// ScheduleNow groups schedule entries for yesterday, today, and tomorrow.
type ScheduleNow struct {
	Today     []ScheduleEntry `json:"today"`
	Tomorrow  []ScheduleEntry `json:"tomorrow"`
	Yesterday []ScheduleEntry `json:"yesterday"`
}
