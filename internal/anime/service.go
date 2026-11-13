package anime

import (
	"context"
	"strings"

	"mila/internal/aniliberty"
	"mila/internal/logger"
	"mila/internal/shikimori"
)

// Service orchestrates AniLiberty release/streaming data and Shikimori metadata/artwork enrichment.
type Service struct {
	lib   *aniliberty.Client
	shiki *shikimori.Client
	log   *logger.Logger
}

// NewService constructs an anime service backed by AniLiberty and Shikimori clients.
func NewService(lib *aniliberty.Client, shiki *shikimori.Client) *Service {
	return &Service{
		lib:   lib,
		shiki: shiki,
		log:   logger.Make("anime"),
	}
}

// Latest returns the most recently updated AniLiberty releases enriched with Shikimori metadata.
func (s *Service) Latest(ctx context.Context, limit int) ([]Anime, error) {
	releases, err := s.lib.Latest(ctx, limit)
	if err != nil {
		return nil, err
	}
	return s.enrichList(ctx, releases), nil
}

// Catalog returns paginated, filtered AniLiberty releases enriched with Shikimori metadata.
func (s *Service) Catalog(ctx context.Context, params aniliberty.CatalogParams) (CatalogPage, error) {
	res, err := s.lib.Catalog(ctx, params)
	if err != nil {
		return CatalogPage{}, err
	}
	return CatalogPage{
		Items:      s.enrichList(ctx, res.Data),
		Pagination: res.Pagination,
	}, nil
}

// Search finds matching releases on AniLiberty and enriches them with Shikimori metadata.
func (s *Service) Search(ctx context.Context, query string) ([]Anime, error) {
	releases, err := s.lib.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	return s.enrichList(ctx, releases), nil
}

// GetRelease fetches full release details (episodes, HLS URLs, torrents) enriched with Shikimori banners and info.
func (s *Service) GetRelease(ctx context.Context, idOrAlias string) (Detail, error) {
	rel, err := s.lib.Release(ctx, idOrAlias)
	if err != nil {
		return Detail{}, err
	}

	var shikiMeta *shikimori.Anime
	if rel.Shikimori != nil && rel.Shikimori.ID > 0 {
		meta, err := s.shiki.GetByID(ctx, rel.Shikimori.ID)
		if err != nil {
			s.log.Warn("shikimori.enrich_failed", "shiki_id", rel.Shikimori.ID, "err", err.Error())
		} else {
			shikiMeta = meta
		}
	}

	episodes := make([]Episode, 0, len(rel.Episodes))
	for i := range rel.Episodes {
		ep := rel.Episodes[i]
		episodes = append(episodes, mapEpisode(&ep))
	}

	detail := Detail{
		Anime:    mergeRelease(rel, shikiMeta),
		Episodes: episodes,
		Torrents: rel.Torrents,
	}
	if shikiMeta != nil && len(shikiMeta.Videos) > 0 {
		detail.Videos = shikiMeta.Videos
	}
	return detail, nil
}

// ScheduleNow returns yesterday, today, and tomorrow schedules enriched with Shikimori metadata.
func (s *Service) ScheduleNow(ctx context.Context) (ScheduleNow, error) {
	sched, err := s.lib.ScheduleNow(ctx)
	if err != nil {
		return ScheduleNow{}, err
	}

	all := make([]aniliberty.Release, 0, len(sched.Today)+len(sched.Tomorrow)+len(sched.Yesterday))
	for _, it := range sched.Today {
		all = append(all, it.Release)
	}
	for _, it := range sched.Tomorrow {
		all = append(all, it.Release)
	}
	for _, it := range sched.Yesterday {
		all = append(all, it.Release)
	}
	shikiMap := s.fetchShikimoriMap(ctx, all)

	return ScheduleNow{
		Today:     mapScheduleItems(sched.Today, shikiMap),
		Tomorrow:  mapScheduleItems(sched.Tomorrow, shikiMap),
		Yesterday: mapScheduleItems(sched.Yesterday, shikiMap),
	}, nil
}

// ScheduleWeek returns the weekly broadcast schedule enriched with Shikimori metadata.
func (s *Service) ScheduleWeek(ctx context.Context) ([]ScheduleEntry, error) {
	items, err := s.lib.ScheduleWeek(ctx)
	if err != nil {
		return nil, err
	}
	releases := make([]aniliberty.Release, len(items))
	for i, it := range items {
		releases[i] = it.Release
	}
	shikiMap := s.fetchShikimoriMap(ctx, releases)
	return mapScheduleItems(items, shikiMap), nil
}

// Genres returns AniLiberty genre definitions.
func (s *Service) Genres(ctx context.Context) ([]aniliberty.Genre, error) {
	return s.lib.Genres(ctx)
}

// References returns all AniLiberty catalog filter dictionaries.
func (s *Service) References(ctx context.Context) (aniliberty.References, error) {
	return s.lib.References(ctx)
}

// SearchShikimori queries Shikimori directly for metadata, posters, and banners.
func (s *Service) SearchShikimori(ctx context.Context, query string, limit int) ([]shikimori.Anime, error) {
	return s.shiki.Search(ctx, query, limit)
}

// GetShikimoriByID fetches a single Shikimori anime record by ID.
func (s *Service) GetShikimoriByID(ctx context.Context, id int) (*shikimori.Anime, error) {
	return s.shiki.GetByID(ctx, id)
}

func (s *Service) enrichList(ctx context.Context, releases []aniliberty.Release) []Anime {
	shikiMap := s.fetchShikimoriMap(ctx, releases)
	out := make([]Anime, 0, len(releases))
	for _, rel := range releases {
		var meta *shikimori.Anime
		if rel.Shikimori != nil {
			if found, ok := shikiMap[rel.Shikimori.ID]; ok {
				copyMeta := found
				meta = &copyMeta
			}
		}
		out = append(out, mergeRelease(rel, meta))
	}
	return out
}

func (s *Service) fetchShikimoriMap(ctx context.Context, releases []aniliberty.Release) map[int]shikimori.Anime {
	ids := make([]int, 0, len(releases))
	for _, r := range releases {
		if r.Shikimori != nil && r.Shikimori.ID > 0 {
			ids = append(ids, r.Shikimori.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	res, err := s.shiki.GetByIDs(ctx, ids)
	if err != nil {
		s.log.Warn("shikimori.batch_enrich_failed", "count", len(ids), "err", err.Error())
		return res
	}
	return res
}

func mapScheduleItems(items []aniliberty.ScheduleItem, shikiMap map[int]shikimori.Anime) []ScheduleEntry {
	out := make([]ScheduleEntry, 0, len(items))
	for _, it := range items {
		var meta *shikimori.Anime
		if it.Release.Shikimori != nil {
			if found, ok := shikiMap[it.Release.Shikimori.ID]; ok {
				copyMeta := found
				meta = &copyMeta
			}
		}
		var ep *Episode
		if it.PublishedEpisode != nil {
			mapped := mapEpisode(it.PublishedEpisode)
			ep = &mapped
		}
		out = append(out, ScheduleEntry{
			Anime:             mergeRelease(it.Release, meta),
			FullSeason:        it.FullSeasonIsReleased,
			PublishedEpisode:  ep,
			NextEpisodeNumber: it.NextEpisodeNumber,
		})
	}
	return out
}

func mergeRelease(rel aniliberty.Release, shiki *shikimori.Anime) Anime {
	a := Anime{
		ID:            rel.ID,
		Alias:         rel.Alias,
		Title:         rel.Name.Main,
		TitleEnglish:  rel.Name.English,
		Description:   strings.TrimSpace(rel.Description),
		Year:          rel.Year,
		IsOngoing:     rel.IsOngoing,
		EpisodesTotal: rel.EpisodesTotal,
	}
	if rel.Season != nil {
		a.Season = rel.Season.Value
	}
	if rel.Type != nil {
		a.Type = rel.Type.Value
	}
	if rel.AgeRating != nil {
		a.AgeRating = rel.AgeRating.Label
	}
	if rel.AverageDurationOfEpisode != nil {
		a.Duration = *rel.AverageDurationOfEpisode
	}
	if rel.Shikimori != nil {
		a.ShikimoriID = rel.Shikimori.ID
		a.ShikimoriURL = rel.Shikimori.URL
		a.Score = rel.Shikimori.Rating
	}
	if rel.MAL != nil {
		a.MalID = rel.MAL.ID
		if a.Score == 0 {
			a.Score = rel.MAL.Rating
		}
	}
	if rel.Poster != nil {
		a.Poster = PosterSet{
			Original:  firstNonEmpty(rel.Poster.Src, optimizedSrc(rel.Poster)),
			Main:      firstNonEmpty(optimizedPreview(rel.Poster), rel.Poster.Preview, rel.Poster.Src),
			Preview:   firstNonEmpty(rel.Poster.Preview, rel.Poster.Src),
			Thumbnail: firstNonEmpty(optimizedThumb(rel.Poster), rel.Poster.Thumbnail),
		}
	}
	for _, g := range rel.Genres {
		if g.Name != "" {
			a.Genres = append(a.Genres, g.Name)
		}
	}
	for _, bg := range rel.BackgroundCovers {
		orig := firstNonEmpty(bg.Src, optimizedSrc(&bg))
		if orig != "" {
			a.Banners = append(a.Banners, BannerImage{
				Original: orig,
				Preview:  firstNonEmpty(bg.Preview, optimizedPreview(&bg)),
			})
		}
	}
	if rel.LatestEpisode != nil {
		ep := mapEpisode(rel.LatestEpisode)
		a.LatestEpisode = &ep
	}

	if shiki != nil {
		if shiki.Name != "" {
			a.TitleOriginal = shiki.Name
		}
		if a.TitleEnglish == "" && shiki.English != "" {
			a.TitleEnglish = shiki.English
		}
		if shiki.Description != "" {
			a.Description = shiki.Description
		}
		if shiki.URL != "" {
			a.ShikimoriURL = shiki.URL
		}
		if shiki.Score > 0 {
			a.Score = shiki.Score
		}
		if shiki.Status != "" {
			a.Status = shiki.Status
		}
		if shiki.EpisodesAired > 0 {
			a.EpisodesAired = shiki.EpisodesAired
		}
		if a.Duration == 0 && shiki.Duration > 0 {
			a.Duration = shiki.Duration
		}
		if shiki.NextEpisodeAt != "" {
			a.NextEpisodeAt = shiki.NextEpisodeAt
		}
		if shiki.MalID > 0 && a.MalID == 0 {
			a.MalID = shiki.MalID
		}
		if shiki.Poster != nil {
			a.Poster.Original = firstNonEmpty(shiki.Poster.OriginalURL, a.Poster.Original)
			a.Poster.Main = firstNonEmpty(shiki.Poster.Main2xURL, shiki.Poster.MainURL, a.Poster.Main)
		}
		if len(shiki.Screenshots) > 0 {
			// Shikimori screenshots serve as widescreen banners / backdrop stills.
			banners := make([]BannerImage, 0, len(shiki.Screenshots)+len(a.Banners))
			for _, sc := range shiki.Screenshots {
				if sc.OriginalURL != "" {
					banners = append(banners, BannerImage{
						Original: sc.OriginalURL,
						Preview:  firstNonEmpty(sc.X332URL, sc.X166URL),
					})
				}
			}
			banners = append(banners, a.Banners...)
			a.Banners = banners
		}
		for _, st := range shiki.Studios {
			if st.Name != "" {
				a.Studios = append(a.Studios, st.Name)
			}
		}
		if len(a.Genres) == 0 {
			for _, g := range shiki.Genres {
				if name := firstNonEmpty(g.Russian, g.Name); name != "" {
					a.Genres = append(a.Genres, name)
				}
			}
		}
	}

	if len(a.Banners) > 0 {
		a.Banner = a.Banners[0].Original
	} else {
		a.Banner = firstNonEmpty(a.Poster.Original, a.Poster.Main)
	}
	return a
}

func mapEpisode(ep *aniliberty.Episode) Episode {
	out := Episode{
		ID:        ep.ID,
		Ordinal:   ep.Ordinal,
		Duration:  ep.Duration,
		Opening:   ep.Opening,
		Ending:    ep.Ending,
		UpdatedAt: ep.UpdatedAt,
	}
	if ep.Name != nil && *ep.Name != "" {
		out.Name = *ep.Name
	} else if ep.NameEnglish != nil {
		out.Name = *ep.NameEnglish
	}
	if ep.Preview != nil {
		out.Preview = firstNonEmpty(
			optimizedPreview(ep.Preview),
			optimizedSrc(ep.Preview),
			ep.Preview.Preview,
			ep.Preview.Src,
			optimizedThumb(ep.Preview),
			ep.Preview.Thumbnail,
		)
	}
	if ep.HLS480 != nil {
		out.HLS480 = *ep.HLS480
	}
	if ep.HLS720 != nil {
		out.HLS720 = *ep.HLS720
	}
	if ep.HLS1080 != nil {
		out.HLS1080 = *ep.HLS1080
	}
	return out
}

func optimizedSrc(img *aniliberty.MediaImage) string {
	if img != nil && img.Optimized != nil {
		return img.Optimized.Src
	}
	return ""
}

func optimizedPreview(img *aniliberty.MediaImage) string {
	if img != nil && img.Optimized != nil {
		return img.Optimized.Preview
	}
	return ""
}

func optimizedThumb(img *aniliberty.MediaImage) string {
	if img != nil && img.Optimized != nil {
		return img.Optimized.Thumbnail
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
