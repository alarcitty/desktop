package shikimori

// Anime represents metadata fetched from Shikimori GraphQL API.
type Anime struct {
	ID            int          `json:"id"`
	MalID         int          `json:"mal_id,omitempty"`
	Name          string       `json:"name"`
	Russian       string       `json:"russian,omitempty"`
	English       string       `json:"english,omitempty"`
	URL           string       `json:"url,omitempty"`
	Description   string       `json:"description,omitempty"`
	Score         float64      `json:"score,omitempty"`
	Status        string       `json:"status,omitempty"`
	Kind          string       `json:"kind,omitempty"`
	Rating        string       `json:"rating,omitempty"`
	Episodes      int          `json:"episodes,omitempty"`
	EpisodesAired int          `json:"episodes_aired,omitempty"`
	Duration      int          `json:"duration,omitempty"`
	AiredOn       string       `json:"aired_on,omitempty"`
	Year          int          `json:"year,omitempty"`
	NextEpisodeAt string       `json:"next_episode_at,omitempty"`
	Poster        *Poster      `json:"poster,omitempty"`
	Screenshots   []Screenshot `json:"screenshots,omitempty"`
	Videos        []Video      `json:"videos,omitempty"`
	Genres        []Genre      `json:"genres,omitempty"`
	Studios       []Studio     `json:"studios,omitempty"`
}

// Poster holds Shikimori poster URLs at multiple resolutions.
type Poster struct {
	ID          string `json:"id,omitempty"`
	OriginalURL string `json:"original_url,omitempty"`
	MainURL     string `json:"main_url,omitempty"`
	Main2xURL   string `json:"main2x_url,omitempty"`
}

// Screenshot holds scene frame / banner image URLs from Shikimori.
type Screenshot struct {
	ID          string `json:"id,omitempty"`
	OriginalURL string `json:"original_url,omitempty"`
	X332URL     string `json:"x332_url,omitempty"`
	X166URL     string `json:"x166_url,omitempty"`
}

// Video represents a trailer, PV, or OP/ED entry from Shikimori.
type Video struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	URL       string `json:"url,omitempty"`
	ImageURL  string `json:"image_url,omitempty"`
	PlayerURL string `json:"player_url,omitempty"`
	Kind      string `json:"kind,omitempty"`
}

// Genre represents a Shikimori genre tag.
type Genre struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Russian string `json:"russian,omitempty"`
}

// Studio represents an animation studio from Shikimori.
type Studio struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}
