package artistvoucher

import (
	"bytes"
	"image"
	_ "image/jpeg"
	"image/png"

	_ "golang.org/x/image/webp"
)

// decodeToPNG re-encodes an uploaded branding image (PNG/JPEG/WebP) to PNG, the
// raster format fpdf embeds directly, so a WebP favicon still prints on the badge.
func decodeToPNG(data []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
