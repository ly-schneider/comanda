package pdf

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func testIconPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: 10, G: 120, B: 220, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode icon: %v", err)
	}
	return buf.Bytes()
}

// 0x10 voucher golden vector (source of truth: backend/internal/qrsign/voucher_test.go).
const (
	goldenVoucherToken   = "EHRlbmFudF9zZWVkMVZjaDdBYjJDZDlFZmVT8QBlVUKACUxpdmUgQmFuZAIGRHJpbmtzBEZvb2S8kQe39D7m2BieSGN4fzWqDHyN18POcyqGizisVN_jU_bQ3r3EMF-OkMRl9WSPaAhWksbXZX7nOXKY_vgLgxYL"
	goldenVoucherBodyHex = "1074656e616e745f73656564315663683741623243643945666553f10065554280094c6976652042616e6402064472696e6b7304466f6f64"
	ed25519SigLen        = 64

	goldenEntitlementToken = "EXRlbmFudF9zZWVkMUVudDdBYjJDZDlFZmVT8QBlVUKADlN0cmFzc2VuYWt0aW9uAnByb2RfYnVyZ2VyMQJwcm9kX2NvbGEwMDEBNyeWMz-nngXnxaing0Xb2R9bgcpZ2DxaBU5PnS4gn4eaCL_ugE8WJ0WnadMiMZfAa3d6yUAttvWfIBVvOAMWAQ"
)

// The decoded token bytes fed into the QR must match the golden wire format the
// offline station verifier expects, byte-for-byte.
func TestVoucherQRBytesMatchGoldenVector(t *testing.T) {
	raw, err := base64.RawURLEncoding.DecodeString(goldenVoucherToken)
	if err != nil {
		t.Fatalf("decode token: %v", err)
	}
	body, err := hex.DecodeString(goldenVoucherBodyHex)
	if err != nil {
		t.Fatalf("decode body hex: %v", err)
	}
	if len(raw) != len(body)+ed25519SigLen {
		t.Fatalf("raw token length = %d, want body(%d)+sig(%d)", len(raw), len(body), ed25519SigLen)
	}
	if !bytes.Equal(raw[:len(body)], body) {
		t.Fatalf("token body drift:\n got %s\nwant %s", hex.EncodeToString(raw[:len(body)]), goldenVoucherBodyHex)
	}
}

func TestRenderVoucherSlips(t *testing.T) {
	raw, err := base64.RawURLEncoding.DecodeString(goldenVoucherToken)
	if err != nil {
		t.Fatalf("decode token: %v", err)
	}
	pdfBytes, err := RenderVoucherSlips(
		VoucherSlip{Label: "Live Band", Details: []string{"Drinks", "Food"}, QRBytes: raw},
		VoucherDefaultCount,
	)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF")) {
		t.Fatal("output is not a PDF")
	}
}

func TestRenderVoucherSlipsWithBrandHeader(t *testing.T) {
	raw, err := base64.RawURLEncoding.DecodeString(goldenVoucherToken)
	if err != nil {
		t.Fatalf("decode token: %v", err)
	}
	cases := []VoucherSlip{
		{Label: "Live Band", Details: []string{"Drinks"}, QRBytes: raw, BrandName: "Kulturfest", BrandIconPNG: testIconPNG(t)},
		{Label: "Live Band", Details: []string{"Drinks"}, QRBytes: raw, BrandName: "Name only, no icon"},
		{Label: "Live Band", Details: []string{"Drinks"}, QRBytes: raw, BrandIconPNG: testIconPNG(t)},
	}
	for i, slip := range cases {
		pdfBytes, err := RenderVoucherSlips(slip, VoucherDefaultCount)
		if err != nil {
			t.Fatalf("case %d render: %v", i, err)
		}
		if !bytes.HasPrefix(pdfBytes, []byte("%PDF")) {
			t.Fatalf("case %d: output is not a PDF", i)
		}
	}
}

func TestRenderVoucherSlipsClampsCount(t *testing.T) {
	raw, _ := base64.RawURLEncoding.DecodeString(goldenVoucherToken)
	slip := VoucherSlip{Label: "x", Details: []string{"y"}, QRBytes: raw}
	for _, count := range []int{-1, 0, 99999} {
		if _, err := RenderVoucherSlips(slip, count); err != nil {
			t.Fatalf("count=%d: %v", count, err)
		}
	}
}

func TestRenderVoucherBatchSlips(t *testing.T) {
	raw, err := base64.RawURLEncoding.DecodeString(goldenVoucherToken)
	if err != nil {
		t.Fatalf("decode token: %v", err)
	}
	entitlement, err := base64.RawURLEncoding.DecodeString(goldenEntitlementToken)
	if err != nil {
		t.Fatalf("decode entitlement token: %v", err)
	}

	slips := []VoucherSlip{
		{Label: "Strassenaktion", Details: []string{"1× Hamburger"}, QRBytes: entitlement, BrandName: "Kulturfest", BrandIconPNG: testIconPNG(t)},
		{Label: "Strassenaktion", Details: []string{"1× Hamburger"}, QRBytes: raw},
	}
	pdfBytes, err := RenderVoucherBatchSlips(slips)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF")) {
		t.Fatal("output is not a PDF")
	}
}

func TestRenderVoucherBatchSlipsPaginates(t *testing.T) {
	raw, _ := base64.RawURLEncoding.DecodeString(goldenEntitlementToken)
	slips := make([]VoucherSlip, 13)
	for i := range slips {
		slips[i] = VoucherSlip{Label: "Strassenaktion", Details: []string{"1× Hamburger"}, QRBytes: raw}
	}
	pdfBytes, err := RenderVoucherBatchSlips(slips)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF")) {
		t.Fatal("output is not a PDF")
	}
}
