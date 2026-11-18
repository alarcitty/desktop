package anime

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// MatchInfo describes one side of a cross-source title match.
type MatchInfo struct {
	ShikimoriID int
	MalID       int
	Titles      []string
	Year        int
	Kind        string
}

const (
	// MatchAcceptScore is the minimum score for a confirmed cross-source link.
	MatchAcceptScore  = 500
	matchScoreShikiID = 10000
	matchScoreMalID   = 9500
	matchScoreExact   = 900
)

var titleStopWords = map[string]bool{
	"the": true, "a": true, "an": true, "of": true, "in": true, "on": true,
	"to": true, "and": true, "for": true, "no": true, "wa": true, "ga": true,
	"ni": true, "wo": true, "season": true, "part": true, "tv": true,
	"сезон": true, "часть": true,
}

var specialTitleRe = regexp.MustCompile(
	`(?i)(^|[^\pL\pN])(specials?|сп[еэ]ш[еэ]?л[ы]?|спецвыпуск[и]?|ova|ова|ona|она|recap|рекап)($|[^\pL\pN])`,
)

var seasonNumRe = regexp.MustCompile(`(?i)(?:season|сезон|tv[-\s]?|part|часть)\s*[-:]?\s*(\d+)`)
var numSeasonRe = regexp.MustCompile(`(?i)(\d+)\s*(?:st|nd|rd|th)?\s*(?:season|сезон|part|часть)`)

var ruOrdinals = map[string]int{
	"первый": 1, "второй": 2, "третий": 3, "четвертый": 4, "четвёртый": 4,
	"пятый": 5, "шестой": 6, "седьмой": 7, "восьмой": 8,
}

// NormalizeTitle lowercases, strips diacritics and punctuation, and collapses whitespace.
func NormalizeTitle(raw string) string {
	lower := strings.ReplaceAll(strings.ToLower(raw), "ё", "е")
	var b strings.Builder
	for _, r := range norm.NFD.String(lower) {
		switch {
		case unicode.Is(unicode.Mn, r):
			// drop combining marks
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// TitleTokens returns meaningful normalized tokens, falling back to all tokens.
func TitleTokens(raw string) []string {
	all := strings.Fields(NormalizeTitle(raw))
	meaningful := make([]string, 0, len(all))
	for _, w := range all {
		if titleStopWords[w] {
			continue
		}
		if len([]rune(w)) >= 2 || isAllDigits(w) {
			meaningful = append(meaningful, w)
		}
	}
	if len(meaningful) > 0 {
		return meaningful
	}
	return all
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ExtractSeasonNumber infers the franchise season ordinal from titles (1 when absent).
func ExtractSeasonNumber(titles []string) int {
	for _, raw := range titles {
		s := strings.ToLower(raw)
		if m := seasonNumRe.FindStringSubmatch(s); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n >= 1 {
				return n
			}
		}
		if m := numSeasonRe.FindStringSubmatch(s); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n >= 1 {
				return n
			}
		}
		for word, n := range ruOrdinals {
			if strings.Contains(s, word+" сезон") {
				return n
			}
		}
	}
	return 1
}

// IsSpecialTitle reports whether any title marks a special/OVA/recap release.
func IsSpecialTitle(titles []string) bool {
	for _, t := range titles {
		if specialTitleRe.MatchString(t) {
			return true
		}
	}
	return false
}

func kindGroup(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "tv", "ona", "web":
		return "series"
	case "movie":
		return "movie"
	case "ova", "special":
		return "short"
	case "music":
		return "music"
	default:
		return ""
	}
}

// ScoreMatch scores a candidate against a target; -1 means incompatible.
func ScoreMatch(target, cand MatchInfo) int {
	if target.ShikimoriID > 0 && target.ShikimoriID == cand.ShikimoriID {
		return matchScoreShikiID
	}
	if target.MalID > 0 && target.MalID == cand.MalID {
		return matchScoreMalID
	}

	candTitles := cleanTitles(cand.Titles)
	targetTitles := cleanTitles(target.Titles)
	if len(candTitles) == 0 || len(targetTitles) == 0 {
		return -1
	}

	if IsSpecialTitle(candTitles) != IsSpecialTitle(targetTitles) {
		return -1
	}
	if ExtractSeasonNumber(candTitles) != ExtractSeasonNumber(targetTitles) {
		return -1
	}
	cg, tg := kindGroup(cand.Kind), kindGroup(target.Kind)
	if cg != "" && tg != "" && cg != tg {
		return -1
	}

	best := -1
	for _, cTitle := range candTitles {
		cNorm := NormalizeTitle(cTitle)
		cToks := TitleTokens(cTitle)
		if cNorm == "" || len(cToks) == 0 {
			continue
		}
		for _, tTitle := range targetTitles {
			tNorm := NormalizeTitle(tTitle)
			tToks := TitleTokens(tTitle)
			if tNorm == "" || len(tToks) == 0 {
				continue
			}

			if cNorm == tNorm {
				score := matchScoreExact
				if target.Year > 0 && cand.Year > 0 && abs(target.Year-cand.Year) <= 1 {
					score += 40
				}
				if score > best {
					best = score
				}
				continue
			}

			tSet := make(map[string]bool, len(tToks))
			for _, tok := range tToks {
				tSet[tok] = true
			}
			matches := 0
			for _, tok := range cToks {
				if tSet[tok] {
					matches++
				}
			}
			if matches == 0 {
				continue
			}

			minLen, maxLen := len(cToks), len(tToks)
			if minLen > maxLen {
				minLen, maxLen = maxLen, minLen
			}
			containment := float64(matches) / float64(minLen)
			jaccard := float64(matches) / float64(maxLen)
			if containment < 0.8 || jaccard < 0.55 {
				continue
			}

			score := int(500*jaccard + 100*containment)
			if target.Year > 0 && cand.Year > 0 {
				switch d := abs(target.Year - cand.Year); {
				case d <= 1:
					score += 40
				case d >= 3:
					score -= 120
				}
			}
			if score > best {
				best = score
			}
		}
	}
	return best
}

// BestMatch picks the highest-scoring compatible candidate above MatchAcceptScore.
func BestMatch(target MatchInfo, cands []MatchInfo) (MatchInfo, int, bool) {
	var best MatchInfo
	bestScore := -1
	for _, cand := range cands {
		score := ScoreMatch(target, cand)
		if score > bestScore {
			best, bestScore = cand, score
		}
	}
	if bestScore < MatchAcceptScore {
		return MatchInfo{}, bestScore, false
	}
	return best, bestScore, true
}

func cleanTitles(titles []string) []string {
	out := make([]string, 0, len(titles))
	for _, t := range titles {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}