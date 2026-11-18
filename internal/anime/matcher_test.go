package anime

import "testing"

func mi(titles []string, year int, kind string) MatchInfo {
	return MatchInfo{Titles: titles, Year: year, Kind: kind}
}

func TestScoreMatch(t *testing.T) {
	cases := []struct {
		name   string
		target MatchInfo
		cand   MatchInfo
		want   int // exact expected score, or -2 for "below accept", or -1 for incompatible
	}{
		// External IDs
		{"shiki id wins over titles", MatchInfo{ShikimoriID: 5, Titles: []string{"A"}}, MatchInfo{ShikimoriID: 5, Titles: []string{"B"}}, 10000},
		{"mal id wins over titles", MatchInfo{MalID: 21, Titles: []string{"A"}}, MatchInfo{MalID: 21, Titles: []string{"B"}}, 9500},
		{"different shiki ids fall through", MatchInfo{ShikimoriID: 5, Titles: []string{"Naruto"}}, MatchInfo{ShikimoriID: 6, Titles: []string{"Naruto"}}, 900},
		{"zero ids never match", MatchInfo{Titles: []string{"X"}}, MatchInfo{Titles: []string{"Y"}}, -1},

		// Exact title matches
		{"exact same year", mi([]string{"One Piece"}, 1999, "tv"), mi([]string{"One Piece"}, 1999, "tv"), 940},
		{"exact no years", mi([]string{"One Piece"}, 0, ""), mi([]string{"One Piece"}, 0, ""), 900},
		{"exact year off by one", mi([]string{"One Piece"}, 1999, ""), mi([]string{"One Piece"}, 2000, ""), 940},
		{"exact year far apart no penalty", mi([]string{"One Piece"}, 1999, ""), mi([]string{"One Piece"}, 2024, ""), 900},
		{"case insensitive", mi([]string{"ONE PIECE"}, 0, ""), mi([]string{"one piece"}, 0, ""), 900},
		{"punctuation stripped", mi([]string{"Kaguya-sama wa Kokurasetai?"}, 0, ""), mi([]string{"Kaguya-sama wa Kokurasetai"}, 0, ""), 900},
		{"colon glued kept", mi([]string{"Re:Zero kara Hajimeru Isekai Seikatsu"}, 0, ""), mi([]string{"Re Zero kara Hajimeru Isekai Seikatsu"}, 0, ""), 900},
		{"diacritics stripped", mi([]string{"Pokémon"}, 0, ""), mi([]string{"Pokemon"}, 0, ""), 900},
		{"russian yo normalized", mi([]string{"Ван-Пис"}, 0, ""), mi([]string{"Ван Пис"}, 0, ""), 900},
		{"synonym in title list", mi([]string{"Sousou no Frieren", "Frieren: Beyond Journey's End"}, 0, ""), mi([]string{"Frieren: Beyond Journey's End"}, 0, ""), 900},
		{"russian vs english via lists", mi([]string{"Магическая битва", "Jujutsu Kaisen"}, 0, ""), mi([]string{"Jujutsu Kaisen", "Магическая битва"}, 0, ""), 900},

		// Sequel / season protection
		{"s1 vs s2 rejected", mi([]string{"Jujutsu Kaisen"}, 2020, "tv"), mi([]string{"Jujutsu Kaisen 2nd Season"}, 2023, "tv"), -1},
		{"s1 vs season 3 rejected", mi([]string{"Kaguya-sama wa Kokurasetai"}, 0, ""), mi([]string{"Kaguya-sama wa Kokurasetai Season 3"}, 0, ""), -1},
		{"s2 vs s2 exact", mi([]string{"Jujutsu Kaisen Season 2"}, 2023, "tv"), mi([]string{"Jujutsu Kaisen Season 2"}, 2023, "tv"), 940},
		{"russian ordinal sequel rejected", mi([]string{"Магическая битва"}, 0, ""), mi([]string{"Магическая битва второй сезон"}, 0, ""), -1},
		{"russian season number rejected", mi([]string{"Ван-Пис"}, 0, ""), mi([]string{"Ван-Пис 2 сезон"}, 0, ""), -1},
		{"final season subtitle below threshold", mi([]string{"Attack on Titan"}, 2013, "tv"), mi([]string{"Attack on Titan Final Season"}, 2020, "tv"), -2},
		{"ultra romantic sequel below threshold", mi([]string{"Kaguya-sama wa Kokurasetai"}, 0, ""), mi([]string{"Kaguya-sama wa Kokurasetai Ultra Romantic"}, 0, ""), -2},
		{"part sequel rejected", mi([]string{"Mushoku Tensei"}, 0, ""), mi([]string{"Mushoku Tensei Part 2"}, 0, ""), -1},

		// Special / OVA protection
		{"tv vs special rejected", mi([]string{"Naruto"}, 0, "tv"), mi([]string{"Naruto Special"}, 0, "special"), -1},
		{"tv vs ova rejected", mi([]string{"Naruto"}, 0, "tv"), mi([]string{"Naruto OVA"}, 0, "ova"), -1},
		{"special vs special exact", mi([]string{"Naruto OVA"}, 0, "ova"), mi([]string{"Naruto OVA"}, 0, "ova"), 900},
		{"russian special rejected", mi([]string{"Наруто"}, 0, ""), mi([]string{"Наруто спецвыпуск"}, 0, ""), -1},
		{"recap rejected", mi([]string{"Bleach"}, 0, ""), mi([]string{"Bleach Recap"}, 0, ""), -1},

		// Kind protection
		{"tv vs movie rejected", mi([]string{"Jujutsu Kaisen"}, 0, "tv"), mi([]string{"Jujutsu Kaisen"}, 0, "movie"), -1},
		{"movie vs movie exact", mi([]string{"Suzume"}, 2022, "movie"), mi([]string{"Suzume"}, 2022, "movie"), 940},
		{"unknown kind skips check", mi([]string{"Suzume"}, 0, ""), mi([]string{"Suzume"}, 0, "movie"), 900},
		{"ona grouped with tv", mi([]string{"Cyberpunk Edgerunners"}, 2022, "tv"), mi([]string{"Cyberpunk Edgerunners"}, 2022, "ona"), 940},
		{"ova vs special same group", mi([]string{"Test Short"}, 0, "ova"), mi([]string{"Test Short"}, 0, "special"), 900},

		// Fuzzy token scoring
		{"stopword tv variant accepted", mi([]string{"Jujutsu Kaisen"}, 2020, ""), mi([]string{"Jujutsu Kaisen TV"}, 2020, ""), 640},
		{"four of five tokens same year", mi([]string{"Some Long Title Here"}, 2020, ""), mi([]string{"Some Long Title Here Extra"}, 2020, ""), 540},
		{"four of five tokens year penalty", mi([]string{"Some Long Title Here"}, 2020, ""), mi([]string{"Some Long Title Here Extra"}, 2015, ""), 380},
		{"short subset below jaccard", mi([]string{"Sousou no Frieren"}, 0, ""), mi([]string{"Frieren"}, 0, ""), -1},
		{"no shared tokens", mi([]string{"Naruto"}, 0, ""), mi([]string{"Bleach"}, 0, ""), -1},
		{"empty target titles", mi(nil, 0, ""), mi([]string{"Naruto"}, 0, ""), -1},
		{"empty cand titles", mi([]string{"Naruto"}, 0, ""), mi(nil, 0, ""), -1},
		{"whitespace only titles", mi([]string{"   "}, 0, ""), mi([]string{"Naruto"}, 0, ""), -1},
		{"movie franchise entry below threshold", mi([]string{"One Piece"}, 1999, "tv"), mi([]string{"One Piece Film Red"}, 2022, "movie"), -1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ScoreMatch(c.target, c.cand)
			switch c.want {
			case -2:
				if got >= MatchAcceptScore {
					t.Fatalf("expected below accept threshold, got %d", got)
				}
			default:
				if got != c.want {
					t.Fatalf("ScoreMatch = %d, want %d", got, c.want)
				}
			}
		})
	}
}

func TestBestMatch(t *testing.T) {
	target := mi([]string{"Jujutsu Kaisen", "Магическая битва"}, 2020, "tv")
	cands := []MatchInfo{
		mi([]string{"Jujutsu Kaisen 0"}, 2021, "movie"),
		mi([]string{"Jujutsu Kaisen 2nd Season"}, 2023, "tv"),
		mi([]string{"Jujutsu Kaisen"}, 2020, "tv"),
		mi([]string{"Naruto"}, 2002, "tv"),
	}
	best, score, ok := BestMatch(target, cands)
	if !ok || score != 940 || best.Titles[0] != "Jujutsu Kaisen" {
		t.Fatalf("BestMatch = %+v score=%d ok=%v", best, score, ok)
	}

	if _, _, ok := BestMatch(target, []MatchInfo{mi([]string{"Naruto"}, 0, ""), mi([]string{"Bleach"}, 0, "")}); ok {
		t.Fatal("expected no acceptable match")
	}
	if _, _, ok := BestMatch(target, nil); ok {
		t.Fatal("expected no match for empty candidates")
	}

	idTarget := MatchInfo{ShikimoriID: 42, Titles: []string{"Whatever"}}
	best, score, ok = BestMatch(idTarget, []MatchInfo{{ShikimoriID: 42}, {ShikimoriID: 7}})
	if !ok || score != 10000 || best.ShikimoriID != 42 {
		t.Fatalf("id BestMatch = %+v score=%d ok=%v", best, score, ok)
	}
}

func TestNormalizeTitle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"One Piece", "one piece"},
		{"  ONE   PIECE  ", "one piece"},
		{"Kaguya-sama wa Kokurasetai?", "kaguya sama wa kokurasetai"},
		{"Fate/stay night: Unlimited Blade Works", "fate stay night unlimited blade works"},
		{"Pokémon", "pokemon"},
		{"Ван-Пис", "ван пис"},
		{"Ёлка", "елка"},
		{"Re:Zero", "re zero"},
		{"!!!", ""},
		{"", ""},
		{"Ｃｙｂｅｒｐｕｎｋ", "ｃｙｂｅｒｐｕｎｋ"},
	}
	for _, c := range cases {
		if got := NormalizeTitle(c.in); got != c.want {
			t.Errorf("NormalizeTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTitleTokens(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"Sousou no Frieren", []string{"sousou", "frieren"}},
		{"The TV Season", []string{"the", "tv", "season"}}, // all stopwords -> fallback to all
		{"Attack on Titan", []string{"attack", "titan"}},
		{"One Piece 2", []string{"one", "piece", "2"}},
		{"", nil},
	}
	for _, c := range cases {
		got := TitleTokens(c.in)
		if len(got) != len(c.want) {
			t.Errorf("TitleTokens(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("TitleTokens(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestExtractSeasonNumber(t *testing.T) {
	cases := []struct {
		titles []string
		want   int
	}{
		{[]string{"Jujutsu Kaisen"}, 1},
		{[]string{"Jujutsu Kaisen 2nd Season"}, 2},
		{[]string{"Jujutsu Kaisen Season 3"}, 3},
		{[]string{"Show TV-2"}, 2},
		{[]string{"Show Part 4"}, 4},
		{[]string{"Магическая битва 2 сезон"}, 2},
		{[]string{"Магическая битва сезон 3"}, 3},
		{[]string{"Магическая битва второй сезон"}, 2},
		{[]string{"Наруто часть 2"}, 2},
		{[]string{"", "Bleach Season 5"}, 5},
		{nil, 1},
	}
	for _, c := range cases {
		if got := ExtractSeasonNumber(c.titles); got != c.want {
			t.Errorf("ExtractSeasonNumber(%v) = %d, want %d", c.titles, got, c.want)
		}
	}
}

func TestIsSpecialTitle(t *testing.T) {
	cases := []struct {
		title string
		want  bool
	}{
		{"Naruto Special", true},
		{"Naruto Specials", true},
		{"Naruto OVA", true},
		{"Naruto ONA", true},
		{"Naruto Recap", true},
		{"Наруто спешл", true},
		{"Наруто спецвыпуск", true},
		{"Наруто рекап", true},
		{"Naruto", false},
		{"Novae", false},
		{"Casshern Sins", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsSpecialTitle([]string{c.title}); got != c.want {
			t.Errorf("IsSpecialTitle(%q) = %v, want %v", c.title, got, c.want)
		}
	}
}