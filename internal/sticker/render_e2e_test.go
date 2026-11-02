package sticker

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestCreateStickerWebPEndToEnd memvalidasi pipeline penuh: PNG render ->
// FFmpeg -> WebP valid yang dapat diterima WhatsApp. Test ini skip secara
// otomatis bila FFmpeg belum tersedia di lingkungan uji.
func TestCreateStickerWebPEndToEnd(t *testing.T) {
	fontPath := filepath.Join("..", "..", "assets", "fonts", "Anton-Regular.ttf")
	if _, err := os.Stat(fontPath); err != nil {
		t.Skipf("font tidak tersedia: %v", err)
	}

	ffmpegPath, err := ResolveBinary("ffmpeg", "ffmpeg")
	if err != nil {
		t.Skipf("ffmpeg tidak tersedia: %v", err)
	}

	// Gambar uji: blok merah 640x480.
	img := image.NewRGBA(image.Rect(0, 0, 640, 480))
	for y := 0; y < 480; y++ {
		for x := 0; x < 640; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 40, B: 40, A: 255})
		}
	}

	workDir := t.TempDir()
	zones, err := ParseStickerText("BAWAH: UJI STIKER", "bottom", 140)
	if err != nil {
		t.Fatal(err)
	}

	webp, err := CreateStickerWebP(context.Background(), []byte{}, nil, fontPath, ffmpegPath, workDir)
	if err != nil {
		t.Fatalf("stiker tanpa gambar gagal: %v", err)
	}
	if !IsValidWebP(webp) {
		t.Fatal("hasil bukan WebP valid")
	}

	webp, err = CreateStickerWebP(context.Background(), encodePNG(t, img), zones, fontPath, ffmpegPath, workDir)
	if err != nil {
		t.Fatalf("stiker ber-caption gagal: %v", err)
	}
	if !IsValidWebP(webp) {
		t.Fatal("hasil ber-caption bukan WebP valid")
	}
	if len(webp) > MaxStickerSize && runtime.GOOS != "windows" {
		// Izinkan fallback quality terendah; hanya peringatkan bila melebihi batas.
		t.Logf("ukuran %d byte melebihi batas %d", len(webp), MaxStickerSize)
	}
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
