package sticker

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fogleman/gg"
	_ "golang.org/x/image/webp"
)

const (
	StickerSize    = 512
	MaxStickerSize = 100 * 1024 // 100 KB
)

// RenderMemeText draws the text zones onto an existing image or a blank canvas.
func RenderMemeText(baseImg image.Image, zones *TextZones, fontPath string) (image.Image, error) {
	dc := gg.NewContext(StickerSize, StickerSize)

	// If base image provided, draw it centered and cropped to 512x512
	if baseImg != nil {
		bounds := baseImg.Bounds()
		bw := float64(bounds.Dx())
		bh := float64(bounds.Dy())

		// Scale so that smaller dimension fills 512
		scale := float64(StickerSize) / bw
		if float64(StickerSize)/bh > scale {
			scale = float64(StickerSize) / bh
		}

		scaledW := bw * scale
		scaledH := bh * scale
		offsetX := (float64(StickerSize) - scaledW) / 2
		offsetY := (float64(StickerSize) - scaledH) / 2

		dc.Push()
		dc.Translate(offsetX, offsetY)
		dc.Scale(scale, scale)
		dc.DrawImage(baseImg, 0, 0)
		dc.Pop()
	} else {
		// Pure white background for .t command
		dc.SetRGB(1, 1, 1)
		dc.Clear()
	}

	if zones == nil || !zones.HasText() {
		return dc.Image(), nil
	}

	// Load Anton font
	if err := dc.LoadFontFace(fontPath, 42); err != nil {
		// Fallback to default if font file missing
		return dc.Image(), fmt.Errorf("gagal memuat font: %w", err)
	}

	maxWidth := float64(StickerSize - 40) // 20px padding left/right

	// Helper to draw text with black outline and white fill
	drawOutlinedText := func(text string, yAnchor float64, alignY float64) {
		lines := dc.WordWrap(text, maxWidth)
		if len(lines) == 0 {
			return
		}

		// Calculate total height of block
		lineHeight := dc.FontHeight() * 1.05
		totalHeight := float64(len(lines)) * lineHeight
		startY := yAnchor - (totalHeight * alignY)

		for i, line := range lines {
			ly := startY + float64(i)*lineHeight
			// Black outline (draw around offsets)
			dc.SetRGB(0, 0, 0)
			outlineWidth := 4.0
			for dx := -outlineWidth; dx <= outlineWidth; dx += 2.0 {
				for dy := -outlineWidth; dy <= outlineWidth; dy += 2.0 {
					dc.DrawStringAnchored(line, float64(StickerSize)/2+dx, ly+dy, 0.5, 0.5)
				}
			}
			// White fill
			dc.SetRGB(1, 1, 1)
			dc.DrawStringAnchored(line, float64(StickerSize)/2, ly, 0.5, 0.5)
		}
	}

	if zones.Top != "" {
		drawOutlinedText(zones.Top, 36, 0.0) // Top anchored
	}
	if zones.Center != "" {
		drawOutlinedText(zones.Center, float64(StickerSize)/2, 0.5) // Center anchored
	}
	if zones.Bottom != "" {
		drawOutlinedText(zones.Bottom, float64(StickerSize)-36, 1.0) // Bottom anchored
	}

	return dc.Image(), nil
}

// CreateStickerWebP composites the image/text and encodes to WebP <= 100KB using FFmpeg.
func CreateStickerWebP(ctx context.Context, imgData []byte, zones *TextZones, fontPath, ffmpegPath, workDir string) ([]byte, error) {
	var baseImg image.Image
	var err error

	if len(imgData) > 0 {
		baseImg, _, err = image.Decode(bytes.NewReader(imgData))
		if err != nil {
			return nil, fmt.Errorf("format gambar tidak dikenali: %w", err)
		}
	}

	finalImg, err := RenderMemeText(baseImg, zones, fontPath)
	if err != nil {
		return nil, err
	}

	// Save temporary PNG
	tempPngPath := filepath.Join(workDir, "temp_render.png")
	f, err := os.Create(tempPngPath)
	if err != nil {
		return nil, err
	}
	if err := png.Encode(f, finalImg); err != nil {
		f.Close()
		return nil, err
	}
	f.Close()

	// Try quality 80 then 50 if too big
	qualities := []int{80, 50, 25}
	for _, q := range qualities {
		tempWebpPath := filepath.Join(workDir, fmt.Sprintf("sticker_q%d.webp", q))

		args := []string{
			"-y",
			"-i", tempPngPath,
			"-vf", fmt.Sprintf("scale=%d:%d", StickerSize, StickerSize),
			"-c:v", "libwebp",
			"-quality", fmt.Sprintf("%d", q),
			"-preset", "picture",
			"-lossless", "0",
			tempWebpPath,
		}

		cmd := exec.CommandContext(ctx, ffmpegPath, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("ffmpeg webp error: %w (output: %s)", err, string(out))
		}

		webpData, err := os.ReadFile(tempWebpPath)
		if err != nil {
			continue
		}

		if len(webpData) <= MaxStickerSize {
			return webpData, nil
		}
	}

	// Return lowest quality if none <= 100KB, or last try
	lastPath := filepath.Join(workDir, "sticker_q25.webp")
	return os.ReadFile(lastPath)
}


 // CreateBratStickerWebP renders .t text in a minimal Brat-generator style:
 // white canvas, bold black lowercase text, centered with generous whitespace.
func CreateBratStickerWebP(ctx context.Context, zones *TextZones, fontPath, ffmpegPath, workDir string) ([]byte, error) {
	dc := gg.NewContext(StickerSize, StickerSize)
	dc.SetRGB(1, 1, 1)
	dc.Clear()
	if zones == nil || !zones.HasText() { return nil, fmt.Errorf("masukkan teks stiker") }
	if err := dc.LoadFontFace(fontPath, 62); err != nil { return nil, fmt.Errorf("gagal memuat font: %w", err) }
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{zones.Top, zones.Center, zones.Bottom}, " ")))
	maxWidth := float64(StickerSize - 76)
	lines := dc.WordWrap(text, maxWidth)
	if len(lines) == 0 { return nil, fmt.Errorf("masukkan teks stiker") }
	fontSize := 62.0
	for len(lines) > 5 || dc.FontHeight()*float64(len(lines))*1.12 > float64(StickerSize-100) {
		fontSize -= 2
		if fontSize < 24 { return nil, fmt.Errorf("teks terlalu panjang untuk stiker") }
		if err := dc.LoadFontFace(fontPath, fontSize); err != nil { return nil, err }
		lines = dc.WordWrap(text, maxWidth)
	}
	lineHeight := dc.FontHeight() * 1.12
	startY := (float64(StickerSize)-lineHeight*float64(len(lines)))/2 + lineHeight/2
	dc.SetRGB(0, 0, 0)
	for i, line := range lines { dc.DrawStringAnchored(line, float64(StickerSize)/2, startY+float64(i)*lineHeight, 0.5, 0.5) }
	pngPath := filepath.Join(workDir, "brat_render.png")
	f, err := os.Create(pngPath)
	if err != nil { return nil, err }
	if err := png.Encode(f, dc.Image()); err != nil { _ = f.Close(); return nil, err }
	if err := f.Close(); err != nil { return nil, err }
	webpPath := filepath.Join(workDir, "brat.webp")
	encode := func(quality int) ([]byte, error) {
		cmd := exec.CommandContext(ctx, ffmpegPath, "-y", "-i", pngPath, "-vf", "scale=512:512", "-c:v", "libwebp", "-quality", fmt.Sprint(quality), "-preset", "picture", webpPath)
		if out, err := cmd.CombinedOutput(); err != nil { return nil, fmt.Errorf("ffmpeg webp error: %w (output: %s)", err, string(out)) }
		return os.ReadFile(webpPath)
	}
	data, err := encode(85)
	if err != nil { return nil, err }
	if len(data) > MaxStickerSize { data, err = encode(60) }
	return data, err
}

// IsValidWebP checks the RIFF and WEBP magic bytes.
func IsValidWebP(data []byte) bool {
	if len(data) < 12 {
		return false
	}
	return string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP"
}

// EnsureAntonFont creates/checks Anton font presence.
func EnsureAntonFont(fontPath string) bool {
	if _, err := os.Stat(fontPath); err == nil {
		return true
	}
	// Try finding it relative to current working dir
	if strings.Contains(fontPath, "Anton-Regular.ttf") {
		alt := filepath.Join("assets", "fonts", "Anton-Regular.ttf")
		if _, err := os.Stat(alt); err == nil {
			return true
		}
	}
	return false
}
