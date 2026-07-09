package api

import (
	"testing"

	"backend/internal/generated/api/generated"
	"backend/internal/generated/ent"
	"backend/internal/generated/ent/settings"
)

func TestToAPISettingsSumupMerchantCode(t *testing.T) {
	code := "MC12345"
	e := &ent.Settings{ID: "t1", PosMode: settings.PosModeQR_CODE, SumupMerchantCode: &code}

	got := toAPISettings(e, 0)
	if got.SumupMerchantCode == nil || *got.SumupMerchantCode != code {
		t.Fatalf("SumupMerchantCode = %v, want %q", got.SumupMerchantCode, code)
	}
}

func TestToAPISettingsSumupMerchantCodeAbsent(t *testing.T) {
	e := &ent.Settings{ID: "t1", PosMode: settings.PosModeQR_CODE}

	got := toAPISettings(e, 0)
	if got.SumupMerchantCode != nil {
		t.Fatalf("SumupMerchantCode = %v, want nil when unset", got.SumupMerchantCode)
	}
}

// A sumupMerchantCode-only PATCH must be treated as a branding-tier update so it
// rides the tenant-admin gate, not the lower shop path.
func TestBrandingPresentSumupMerchantCode(t *testing.T) {
	code := "MC12345"
	if !brandingPresent(&generated.SettingsUpdate{SumupMerchantCode: &code}) {
		t.Fatal("sumupMerchantCode-only update must be gated as branding")
	}
	if brandingPresent(&generated.SettingsUpdate{}) {
		t.Fatal("empty update must not be gated as branding")
	}
}
