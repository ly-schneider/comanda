// Package androidpkg derives and validates the per-tenant suffix of the Android
// applicationId (ch.leys.comanda.<suffix>).
package androidpkg

import (
	"regexp"
	"strings"
)

// Prefix keeps the segment starting with a letter; tenant slugs may start with a digit.
const Prefix = "t_"

var pattern = regexp.MustCompile(`^t_[a-z0-9_]{1,60}$`)

// ForSlug derives the package suffix for a tenant slug. Slugs are DNS labels
// ([a-z0-9-], never underscore), so the hyphen substitution cannot collide two
// tenants onto one suffix.
func ForSlug(slug string) string {
	return Prefix + strings.ReplaceAll(strings.ToLower(strings.TrimSpace(slug)), "-", "_")
}

func Valid(s string) bool {
	return pattern.MatchString(s)
}

func APKAssetName(pkg string) string {
	if pkg == "" {
		return "comanda-pos.apk"
	}
	return "comanda-pos-" + pkg + ".apk"
}
