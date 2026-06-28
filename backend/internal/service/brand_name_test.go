package service

import (
	"context"
	"testing"

	"backend/internal/config"
	"backend/internal/generated/ent"
)

func TestBrandName(t *testing.T) {
	display := "Müller's Stand"

	cases := []struct {
		name string
		repo *fakeSettingsRepo
		want string
	}{
		{"display_name wins", &fakeSettingsRepo{got: &ent.Settings{DisplayName: &display}, tenant: "Incil"}, display},
		{"falls back to tenant name", &fakeSettingsRepo{got: &ent.Settings{}, tenant: "Incil"}, "Incil"},
		{"platform default when nothing is set", &fakeSettingsRepo{got: &ent.Settings{}}, "Comanda"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &emailService{cfg: config.PlunkConfig{FromName: "Comanda"}, settingsRepo: tc.repo}
			if got := s.brandName(context.Background()); got != tc.want {
				t.Fatalf("brandName = %q, want %q", got, tc.want)
			}
		})
	}
}
