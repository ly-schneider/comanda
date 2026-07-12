package artistvoucher

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestDecodeToPNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	var pngBuf, jpegBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if err := jpeg.Encode(&jpegBuf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}

	for name, in := range map[string][]byte{"png": pngBuf.Bytes(), "jpeg": jpegBuf.Bytes()} {
		out, err := decodeToPNG(in)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, format, err := image.DecodeConfig(bytes.NewReader(out)); err != nil || format != "png" {
			t.Fatalf("%s: got format %q err %v, want png", name, format, err)
		}
	}

	if _, err := decodeToPNG([]byte("not an image")); err == nil {
		t.Fatal("expected error decoding garbage bytes")
	}
}
