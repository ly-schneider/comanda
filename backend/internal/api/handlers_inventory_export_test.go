package api

import (
	"testing"
	"time"

	"backend/internal/xlsx"
)

func TestExportFilename(t *testing.T) {
	// Local midnight as the client sends it: 2026-06-30T22:00:00Z.
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, xlsx.Location()).UTC()
	cases := []struct {
		label string
		want  string
	}{
		{"Bar Ost", "Inventar_Bar-Ost_2026-07-01.xlsx"},
		{"Getränke", "Inventar_Getraenke_2026-07-01.xlsx"},
		{"Grosse Röschti / Süss", "Inventar_Grosse-Roeschti--Suess_2026-07-01.xlsx"},
		{"", "Inventar_Export_2026-07-01.xlsx"},
		{"?!*", "Inventar_Export_2026-07-01.xlsx"},
	}
	for _, c := range cases {
		got := exportFilename(xlsx.InventoryReport{ScopeLabel: c.label, From: from})
		if got != c.want {
			t.Fatalf("exportFilename(%q) = %q, want %q", c.label, got, c.want)
		}
	}
}
