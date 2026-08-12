package staffperks

import (
	"errors"
	"testing"

	coreservice "backend/internal/service"
)

func product(id string, group int) coreservice.VolunteerCampaignProductView {
	return coreservice.VolunteerCampaignProductView{
		ProductID:   id,
		ProductName: id,
		Quantity:    1,
		ChoiceGroup: group,
	}
}

func ids(views []coreservice.VolunteerCampaignProductView) []string {
	out := make([]string, 0, len(views))
	for _, v := range views {
		out = append(out, v.ProductID)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestResolveCampaignSelection(t *testing.T) {
	t.Run("without groups every product is handed out", func(t *testing.T) {
		got, err := resolveCampaignSelection([]coreservice.VolunteerCampaignProductView{
			product("pommes", 0),
			product("getraenk", 0),
		}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []string{"pommes", "getraenk"}; !equal(ids(got), want) {
			t.Fatalf("got %v, want %v", ids(got), want)
		}
	})

	t.Run("a single-option group resolves on its own", func(t *testing.T) {
		got, err := resolveCampaignSelection([]coreservice.VolunteerCampaignProductView{
			product("pommes", 1),
		}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []string{"pommes"}; !equal(ids(got), want) {
			t.Fatalf("got %v, want %v", ids(got), want)
		}
	})

	t.Run("an unanswered group reports its options", func(t *testing.T) {
		_, err := resolveCampaignSelection([]coreservice.VolunteerCampaignProductView{
			product("wurst", 0),
			product("pommes", 1),
			product("getraenk", 1),
		}, nil)
		var choiceErr *coreservice.VolunteerChoiceRequiredError
		if !errors.As(err, &choiceErr) {
			t.Fatalf("expected choice required, got %v", err)
		}
		if len(choiceErr.Groups) != 1 || choiceErr.Groups[0].ChoiceGroup != 1 {
			t.Fatalf("unexpected groups: %+v", choiceErr.Groups)
		}
		if want := []string{"pommes", "getraenk"}; !equal(ids(choiceErr.Groups[0].Options), want) {
			t.Fatalf("got %v, want %v", ids(choiceErr.Groups[0].Options), want)
		}
	})

	t.Run("the picked option replaces its group", func(t *testing.T) {
		got, err := resolveCampaignSelection([]coreservice.VolunteerCampaignProductView{
			product("wurst", 0),
			product("pommes", 1),
			product("getraenk", 1),
		}, []coreservice.VolunteerChoiceSelection{{ChoiceGroup: 1, ProductID: "getraenk"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []string{"wurst", "getraenk"}; !equal(ids(got), want) {
			t.Fatalf("got %v, want %v", ids(got), want)
		}
	})

	t.Run("every group needs an answer", func(t *testing.T) {
		_, err := resolveCampaignSelection([]coreservice.VolunteerCampaignProductView{
			product("pommes", 1),
			product("salat", 1),
			product("bier", 2),
			product("wasser", 2),
		}, []coreservice.VolunteerChoiceSelection{{ChoiceGroup: 1, ProductID: "salat"}})
		var choiceErr *coreservice.VolunteerChoiceRequiredError
		if !errors.As(err, &choiceErr) {
			t.Fatalf("expected choice required, got %v", err)
		}
		if len(choiceErr.Groups) != 1 || choiceErr.Groups[0].ChoiceGroup != 2 {
			t.Fatalf("unexpected groups: %+v", choiceErr.Groups)
		}
	})

	t.Run("a product outside the group is rejected", func(t *testing.T) {
		_, err := resolveCampaignSelection([]coreservice.VolunteerCampaignProductView{
			product("pommes", 1),
			product("getraenk", 1),
		}, []coreservice.VolunteerChoiceSelection{{ChoiceGroup: 1, ProductID: "champagner"}})
		if !errors.Is(err, coreservice.ErrVolunteerChoiceInvalid) {
			t.Fatalf("expected choice invalid, got %v", err)
		}
	})

	t.Run("an unknown group is rejected", func(t *testing.T) {
		_, err := resolveCampaignSelection([]coreservice.VolunteerCampaignProductView{
			product("pommes", 1),
		}, []coreservice.VolunteerChoiceSelection{{ChoiceGroup: 7, ProductID: "pommes"}})
		if !errors.Is(err, coreservice.ErrVolunteerChoiceInvalid) {
			t.Fatalf("expected choice invalid, got %v", err)
		}
	})

	t.Run("conflicting picks for one group are rejected", func(t *testing.T) {
		_, err := resolveCampaignSelection([]coreservice.VolunteerCampaignProductView{
			product("pommes", 1),
			product("getraenk", 1),
		}, []coreservice.VolunteerChoiceSelection{
			{ChoiceGroup: 1, ProductID: "pommes"},
			{ChoiceGroup: 1, ProductID: "getraenk"},
		})
		if !errors.Is(err, coreservice.ErrVolunteerChoiceInvalid) {
			t.Fatalf("expected choice invalid, got %v", err)
		}
	})
}

func TestValidateProducts(t *testing.T) {
	t.Run("rejects the same product twice", func(t *testing.T) {
		err := validateProducts([]VolunteerCampaignProductInput{
			{ProductID: "pommes", Quantity: 1, ChoiceGroup: 1},
			{ProductID: "pommes", Quantity: 1, ChoiceGroup: 2},
		})
		if !errors.Is(err, coreservice.ErrVolunteerCampaignProductInvalid) {
			t.Fatalf("expected product invalid, got %v", err)
		}
	})

	t.Run("accepts distinct products across groups", func(t *testing.T) {
		err := validateProducts([]VolunteerCampaignProductInput{
			{ProductID: "pommes", Quantity: 1, ChoiceGroup: 1},
			{ProductID: "getraenk", Quantity: 2, ChoiceGroup: 1},
			{ProductID: "wurst", Quantity: 1},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
