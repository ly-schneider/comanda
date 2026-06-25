package payrexx

import "testing"

func TestNormalizeInstance(t *testing.T) {
	cases := map[string]string{
		"blessthun":                              "blessthun",
		"blessthun.payrexx.com":                  "blessthun",
		"https://blessthun.payrexx.com":          "blessthun",
		"http://blessthun.payrexx.com/":          "blessthun",
		"  blessthun  ":                          "blessthun",
		"blessthun\n":                            "blessthun",
		"https://blessthun.payrexx.com/?foo=bar": "blessthun",
		"":                                       "",
		" ":                                      "",
	}
	for in, want := range cases {
		if got := NormalizeInstance(in); got != want {
			t.Errorf("NormalizeInstance(%q) = %q, want %q", in, got, want)
		}
	}
}
