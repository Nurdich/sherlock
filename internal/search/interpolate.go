package search

import "strings"

// Interpolate replaces every "{}" placeholder in strings nested inside v.
// Map keys are left unchanged, matching the Python helper.
func Interpolate(v any, username string) any {
	switch t := v.(type) {
	case string:
		return strings.ReplaceAll(t, "{}", username)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			out[k] = Interpolate(child, username)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			out[i] = Interpolate(child, username)
		}
		return out
	default:
		return v
	}
}

// ProfileURL builds the human-facing profile URL.
// Spaces in the username are encoded as %20, matching the Python CLI.
func ProfileURL(template, username string) string {
	return strings.ReplaceAll(template, "{}", strings.ReplaceAll(username, " ", "%20"))
}

// ProbeURL builds the URL that is actually requested.
// A missing probe template falls back to the already-rendered profile URL.
// Explicit probe templates are interpolated with the raw username.
func ProbeURL(siteURLProbe, profile, username string) string {
	if siteURLProbe == "" {
		return profile
	}
	return strings.ReplaceAll(siteURLProbe, "{}", username)
}
