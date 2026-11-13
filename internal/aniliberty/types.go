package aniliberty

// EnumValue represents a value+description pair returned by AniLiberty v1.
type EnumValue struct {
	Value       string `json:"value"`
	Description string `json:"description"`
}

// ReleaseName contains localized release titles.
type ReleaseName struct {
	Main        string  `json:"main"`
	English     string  `json:"english,omitempty"`
	Alternative *string `json:"alternative,omitempty"`
}

// ExternalRef contains Shikimori or MAL rating/ID references on a release.
type ExternalRef struct {
	ID     int     `json:"id"`
	URL    string  `json:"url"`
	Votes  int     `json:"votes"`
	Rating float64 `json:"rating"`
}

// ImagePaths represents standard image variants in AniLiberty responses.
type ImagePaths struct {
	Src       string `json:"src,omitempty"`
	Preview   string `json:"preview,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty"`
}

// MediaImage includes original and WebP-optimized image variants.
type MediaImage struct {
	Src       string      `json:"src,omitempty"`
	Preview   string      `json:"preview,omitempty"`
	Thumbnail string      `json:"thumbnail,omitempty"`
	Optimized *ImagePaths `json:"optimized,omitempty"`
}

// AgeRating describes the age restriction metadata.
type AgeRating struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	IsAdult     bool   `json:"is_adult"`
	Description string `json:"description"`
}

// PublishDay describes the weekly broadcast day.
type PublishDay struct {
	Value       int    `json:"value"`
	Description string `json:"description"`
}

// Genre describes an anime genre entry.
type Genre struct {
	ID            int         `json:"id"`
	Name          string      `json:"name"`
	Image         *MediaImage `json:"image,omitempty"`
	TotalReleases int         `json:"total_releases,omitempty"`
}

// TimeRange represents opening/ending start and stop seconds in an episode.
type TimeRange struct {
	Start *int `json:"start,omitempty"`
	Stop  *int `json:"stop,omitempty"`
}

// Episode represents a playable episode with HLS streams and skip markers.
type Episode struct {
	ID          string      `json:"id"`
	ReleaseID   int         `json:"release_id"`
	Name        *string     `json:"name,omitempty"`
	NameEnglish *string     `json:"name_english,omitempty"`
	Ordinal     float64     `json:"ordinal"`
	SortOrder   float64     `json:"sort_order"`
	Duration    int         `json:"duration"`
	Opening     *TimeRange  `json:"opening,omitempty"`
	Ending      *TimeRange  `json:"ending,omitempty"`
	Preview     *MediaImage `json:"preview,omitempty"`
	HLS480      *string     `json:"hls_480,omitempty"`
	HLS720      *string     `json:"hls_720,omitempty"`
	HLS1080     *string     `json:"hls_1080,omitempty"`
	RutubeID    *string     `json:"rutube_id,omitempty"`
	YouTubeID   *string     `json:"youtube_id,omitempty"`
	UpdatedAt   string      `json:"updated_at,omitempty"`
}

// Torrent holds torrent download metadata for a release.
type Torrent struct {
	ID          int        `json:"id"`
	Hash        string     `json:"hash"`
	Magnet      string     `json:"magnet"`
	Filename    string     `json:"filename"`
	Description string     `json:"description,omitempty"`
	Size        int64      `json:"size"`
	Seeders     int        `json:"seeders"`
	Leechers    int        `json:"leechers"`
	Quality     *EnumValue `json:"quality,omitempty"`
	IsHardsub   bool       `json:"is_hardsub"`
}

// Release represents an anime release from AniLiberty v1.
type Release struct {
	ID                       int          `json:"id"`
	Alias                    string       `json:"alias"`
	Type                     *EnumValue   `json:"type,omitempty"`
	Year                     int          `json:"year"`
	Season                   *EnumValue   `json:"season,omitempty"`
	Name                     ReleaseName  `json:"name"`
	Description              string       `json:"description,omitempty"`
	Notification             *string      `json:"notification,omitempty"`
	Shikimori                *ExternalRef `json:"shikimori,omitempty"`
	MAL                      *ExternalRef `json:"mal,omitempty"`
	Poster                   *MediaImage  `json:"poster,omitempty"`
	AgeRating                *AgeRating   `json:"age_rating,omitempty"`
	PublishDay               *PublishDay  `json:"publish_day,omitempty"`
	IsOngoing                bool         `json:"is_ongoing"`
	IsInProduction           bool         `json:"is_in_production"`
	IsBlockedByGeo           bool         `json:"is_blocked_by_geo"`
	IsBlockedByCopyrights    bool         `json:"is_blocked_by_copyrights"`
	EpisodesTotal            *int         `json:"episodes_total,omitempty"`
	AverageDurationOfEpisode *int         `json:"average_duration_of_episode,omitempty"`
	AddedInUsersFavorites    int          `json:"added_in_users_favorites"`
	FreshAt                  string       `json:"fresh_at,omitempty"`
	CreatedAt                string       `json:"created_at,omitempty"`
	UpdatedAt                string       `json:"updated_at,omitempty"`
	Genres                   []Genre      `json:"genres,omitempty"`
	LatestEpisode            *Episode     `json:"latest_episode,omitempty"`
	BackgroundCovers         []MediaImage `json:"background_covers,omitempty"`
	Episodes                 []Episode    `json:"episodes,omitempty"`
	Torrents                 []Torrent    `json:"torrents,omitempty"`
}

// Pagination mirrors AniLiberty catalog pagination metadata.
type Pagination struct {
	Total       int `json:"total"`
	Count       int `json:"count"`
	PerPage     int `json:"per_page"`
	CurrentPage int `json:"current_page"`
	TotalPages  int `json:"total_pages"`
}

// CatalogResponse represents the paginated /anime/catalog/releases payload.
type CatalogResponse struct {
	Data       []Release  `json:"data"`
	Pagination Pagination `json:"pagination"`
}

// CatalogParams specifies filters and pagination for catalog queries.
type CatalogParams struct {
	Page     int
	Limit    int
	Search   string
	Sorting  string
	Genres   []int
	Types    []string
	Seasons  []string
	FromYear int
	ToYear   int
}

// ScheduleItem represents one scheduled broadcast entry.
type ScheduleItem struct {
	Release              Release  `json:"release"`
	FullSeasonIsReleased bool     `json:"full_season_is_released"`
	PublishedEpisode     *Episode `json:"published_release_episode,omitempty"`
	NextEpisodeNumber    *int     `json:"next_release_episode_number,omitempty"`
}

// ScheduleNow groups schedule entries for today, tomorrow, and yesterday.
type ScheduleNow struct {
	Today     []ScheduleItem `json:"today"`
	Tomorrow  []ScheduleItem `json:"tomorrow"`
	Yesterday []ScheduleItem `json:"yesterday"`
}

// ReferenceItem represents a dictionary option (sorting, type, season).
type ReferenceItem struct {
	Value       string `json:"value"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

// References groups all catalog filter reference lists.
type References struct {
	Genres  []Genre         `json:"genres"`
	Sorting []ReferenceItem `json:"sorting"`
	Types   []ReferenceItem `json:"types"`
	Seasons []ReferenceItem `json:"seasons"`
}
