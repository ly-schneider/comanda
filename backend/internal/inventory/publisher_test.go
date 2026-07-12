package inventory

import (
	"testing"
	"time"
)

func TestMarshalParseUpdate_RoundTrip(t *testing.T) {
	variant := "vAr1AntXyz12"
	want := Update{
		TenantID:  "tEnAntXyz123",
		ProductID: "prOdXyz12345",
		VariantID: &variant,
		NewStock:  42,
		Delta:     -3,
		Timestamp: time.Now().UTC().Truncate(time.Second),
	}

	payload, err := MarshalUpdate(want)
	if err != nil {
		t.Fatalf("MarshalUpdate: %v", err)
	}

	got, err := ParseUpdate(payload)
	if err != nil {
		t.Fatalf("ParseUpdate: %v", err)
	}

	if got.TenantID != want.TenantID || got.ProductID != want.ProductID ||
		got.NewStock != want.NewStock || got.Delta != want.Delta ||
		!got.Timestamp.Equal(want.Timestamp) {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, want)
	}
	if got.VariantID == nil || *got.VariantID != *want.VariantID {
		t.Fatalf("variant id lost in round-trip: got %v", got.VariantID)
	}
}

func TestParseUpdate_Invalid(t *testing.T) {
	if _, err := ParseUpdate("not json"); err == nil {
		t.Fatal("expected error for invalid payload")
	}
}
