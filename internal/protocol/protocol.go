// Package protocol registers the mila:// URI scheme and extracts deep links
// from process arguments so shared links open directly on the anime page.
package protocol

import "strings"

const SchemePrefix = "mila://"

// IsDeepLink reports whether s is a mila:// deep link.
func IsDeepLink(s string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(s)), SchemePrefix)
}

// FindLink returns the first mila:// argument, or an empty string.
func FindLink(args []string) string {
	for _, arg := range args {
		trimmed := strings.Trim(strings.TrimSpace(arg), `"`)
		if IsDeepLink(trimmed) {
			return trimmed
		}
	}
	return ""
}

// OpenCommand builds the registry shell command that forwards the URL to the app.
func OpenCommand(exePath string) string {
	return `"` + exePath + `" "%1"`
}