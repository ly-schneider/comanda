package pdf

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

// 0x10 voucher golden vector (source of truth: backend/internal/qrsign/voucher_test.go).
const (
	goldenVoucherToken   = "EHRlbmFudF9zZWVkMVZjaDdBYjJDZDlFZmVT8QBlVUKACUxpdmUgQmFuZAIGRHJpbmtzBEZvb2S8kQe39D7m2BieSGN4fzWqDHyN18POcyqGizisVN_jU_bQ3r3EMF-OkMRl9WSPaAhWksbXZX7nOXKY_vgLgxYL"
	goldenVoucherBodyHex = "1074656e616e745f73656564315663683741623243643945666553f10065554280094c6976652042616e6402064472696e6b7304466f6f64"
	ed25519SigLen        = 64
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
	pdfBytes, err := RenderVoucherSlips([]VoucherSlip{
		{Label: "Live Band", Categories: []string{"Drinks", "Food"}, QRBytes: raw},
		{Label: "VIP Guest", Categories: []string{"Beer"}, QRBytes: raw},
	}, VoucherDefaultCols)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF")) {
		t.Fatal("output is not a PDF")
	}
}

func TestRenderVoucherSlipsClampsColumns(t *testing.T) {
	raw, _ := base64.RawURLEncoding.DecodeString(goldenVoucherToken)
	slip := []VoucherSlip{{Label: "x", Categories: []string{"y"}, QRBytes: raw}}
	for _, cols := range []int{-1, 0, 999} {
		if _, err := RenderVoucherSlips(slip, cols); err != nil {
			t.Fatalf("cols=%d: %v", cols, err)
		}
	}
}

func TestRenderVoucherSlipsEmpty(t *testing.T) {
	pdfBytes, err := RenderVoucherSlips(nil, VoucherDefaultCols)
	if err != nil {
		t.Fatalf("render empty: %v", err)
	}
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF")) {
		t.Fatal("empty output is not a PDF")
	}
}
