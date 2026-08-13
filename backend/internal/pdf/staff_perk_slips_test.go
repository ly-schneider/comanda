package pdf

import (
	"bytes"
	"testing"
)

func TestRenderStaffPerkSlips_ProducesPDF(t *testing.T) {
	out, err := RenderStaffPerkSlips(SlipInput{
		CampaignName: "Helferessen Samstag",
		QRPayload:    "CAMP:tkn_camp___1",
		Products: []SlipProduct{
			{Name: "Pizza Margherita", Quantity: 1},
			{Name: "Getränk", Quantity: 1},
		},
		Count: 30,
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Fatalf("output missing PDF magic bytes; got first 8 bytes = %q", out[:8])
	}
	if len(out) < 2000 {
		t.Fatalf("PDF suspiciously small: %d bytes", len(out))
	}
}

func TestFormatProductLines_CollapsesChoiceGroups(t *testing.T) {
	lines := formatProductLines([]SlipProduct{
		{Name: "Wurst", Quantity: 1},
		{Name: "Pommes", Quantity: 1, ChoiceGroup: 1},
		{Name: "Getränk", Quantity: 2, ChoiceGroup: 1},
		{Name: "Bier", Quantity: 1, ChoiceGroup: 2},
	})
	want := []string{"1× Wurst", "1× Pommes oder 2× Getränk", "1× Bier"}
	if len(lines) != len(want) {
		t.Fatalf("got %v, want %v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestRenderStaffPerkSlips_RejectsBadInput(t *testing.T) {
	if _, err := RenderStaffPerkSlips(SlipInput{Count: 0, QRPayload: "x"}); err == nil {
		t.Fatal("expected error for count=0")
	}
	if _, err := RenderStaffPerkSlips(SlipInput{Count: 1, QRPayload: ""}); err == nil {
		t.Fatal("expected error for empty payload")
	}
}
