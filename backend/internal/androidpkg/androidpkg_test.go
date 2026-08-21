package androidpkg

import "testing"

func TestForSlug(t *testing.T) {
	cases := map[string]string{
		"blessthun": "t_blessthun",
		"acme-food": "t_acme_food",
		"3sterne":   "t_3sterne",
		"a-b-c":     "t_a_b_c",
		"  Acme  ":  "t_acme",
	}
	for slug, want := range cases {
		if got := ForSlug(slug); got != want {
			t.Errorf("ForSlug(%q) = %q, want %q", slug, got, want)
		}
		if !Valid(ForSlug(slug)) {
			t.Errorf("ForSlug(%q) produced an invalid package %q", slug, ForSlug(slug))
		}
	}
}

func TestForSlugIsInjective(t *testing.T) {
	seen := map[string]string{}
	for _, slug := range []string{"a-b", "ab", "a-b-c", "abc", "x1", "x-1"} {
		pkg := ForSlug(slug)
		if prev, dup := seen[pkg]; dup {
			t.Fatalf("slugs %q and %q both derive %q", prev, slug, pkg)
		}
		seen[pkg] = slug
	}
}

func TestValidRejectsJunk(t *testing.T) {
	for _, s := range []string{
		"", "acme", "t_", "t_ACME", "t_acme-1", "t_acme.evil", "../../etc/passwd",
		"t_" + string(make([]byte, 61)),
	} {
		if Valid(s) {
			t.Errorf("Valid(%q) = true, want false", s)
		}
	}
}

func TestAPKAssetName(t *testing.T) {
	if got := APKAssetName(""); got != "comanda-pos.apk" {
		t.Errorf("neutral asset = %q", got)
	}
	if got := APKAssetName("t_acme"); got != "comanda-pos-t_acme.apk" {
		t.Errorf("tenant asset = %q", got)
	}
}
