// Package androidpkg derives and validates the per-tenant suffix of the Android
// applicationId (ch.leys.comanda.<suffix>).
//
// The value is written once at tenant creation and never changes: Android keys
// an install off the applicationId, so a different suffix is a different app —
// no upgrade path, re-pairing, and two icons on the launcher.
package androidpkg

import (
	"regexp"
	"strings"
)

// Prefix keeps the segment starting with a letter. Package segments must match
// [a-zA-Z_][a-zA-Z0-9_]*, while tenant slugs are DNS labels that may start with
// a digit.
const Prefix = "t_"

// pattern is what a derived suffix always looks like, and the only shape the
// public download routes accept.
var pattern = regexp.MustCompile(`^t_[a-z0-9_]{1,60}$`)

// ForSlug derives the package suffix for a tenant slug. Slugs are DNS labels
// ([a-z0-9-], never underscore), so the hyphen substitution is injective and
// two tenants can never derive the same suffix.
func ForSlug(slug string) string {
	return Prefix + strings.ReplaceAll(strings.ToLower(strings.TrimSpace(slug)), "-", "_")
}

// Valid reports whether s is a well-formed suffix. Callers on public routes use
// this to reject junk before it reaches a release-asset lookup.
func Valid(s string) bool {
	return pattern.MatchString(s)
}

// APKAssetName is the release asset a package's build publishes. The neutral
// build carries no suffix and keeps the bare name.
func APKAssetName(pkg string) string {
	if pkg == "" {
		return "comanda-pos.apk"
	}
	return "comanda-pos-" + pkg + ".apk"
}
