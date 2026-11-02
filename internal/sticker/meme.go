package sticker

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/fogleman/gg"
)

type textRun struct {
	text string
	face string
}

func isEmojiRune(r rune) bool {
	return (r >= 0x1F000 && r <= 0x1FAFF) ||
		(r >= 0x2600 && r <= 0x27BF) ||
		(r >= 0x2300 && r <= 0x23FF)
}

func emojiFontCandidates(primary string) []string {
	dirs := []string{
		filepath.Dir(primary),
		"/system/fonts",
		"/usr/share/fonts/truetype/noto",
		"/usr/share/fonts/truetype/dejavu",
		"/usr/local/share/fonts",
		filepath.Join(os.Getenv("WINDIR"), "Fonts"),
	}
	names := []string{
		"NotoSansSymbols2-Regular.ttf",
		"NotoSansSymbols-Regular.ttf",
		"NotoEmoji-Regular.ttf",
		"SegoeUIEmoji.ttf",
		"seguisym.ttf",
		"DejaVuSans.ttf",
	}
	var found []string
	for _, dir := range dirs {
		if dir == "." || dir == "" {
			continue
		}
		for _, name := range names {
			path := filepath.Join(dir, name)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				found = append(found, path)
			}
		}
	}
	return found
}

// CreateMemeTextStickerWebP creates a white meme-style text sticker. Text is
// uppercase, bold white, outlined in black, and anchored near the bottom.
// Emoji glyphs use a system symbol/emoji font when one is available.
func CreateMemeTextStickerWebP(ctx context.Context, zones *TextZones, fontPath, ffmpegPath, workDir string) ([]byte, error) {
	if zones == nil || !zones.HasText() {
		return nil, fmt.Errorf("masukkan teks stiker")
	}

	dc := gg.NewContext(StickerSize, StickerSize)
	dc.SetRGBA(0, 0, 0, 0)
	dc.Clear()

	fontSize := 54.0
	if err := dc.LoadFontFace(fontPath, fontSize); err != nil {
		return nil, fmt.Errorf("gagal memuat font: %w", err)
	}

	emojiFont := ""
	for _, candidate := range emojiFontCandidates(fontPath) {
		if err := dc.LoadFontFace(candidate, fontSize); err == nil {
			emojiFont = candidate
			break
		}
	}
	joined := strings.TrimSpace(strings.Join([]string{zones.Top, zones.Center, zones.Bottom}, " "))
	joined = strings.ToUpper(joined)
	if emojiFont == "" {
		for _, r := range joined {
			if isEmojiRune(r) {
				return nil, fmt.Errorf("font emoji tidak ditemukan. Pasang NotoSansSymbols2-Regular.ttf atau NotoEmoji-Regular.ttf di folder assets/fonts")
			}
		}
	}

	maxWidth := float64(StickerSize - 36)
	lines := dc.WordWrap(joined, maxWidth)
	if len(lines) == 0 {
		return nil, fmt.Errorf("masukkan teks stiker")
	}
	for len(lines) > 4 || dc.FontHeight()*float64(len(lines))*1.12 > 190 {
		fontSize -= 2
		if fontSize < 24 {
			return nil, fmt.Errorf("teks terlalu panjang untuk stiker")
		}
		_ = dc.LoadFontFace(fontPath, fontSize)
		lines = dc.WordWrap(joined, maxWidth)
	}

	lineHeight := dc.FontHeight() * 1.08
	startY := float64(StickerSize) - 38 - lineHeight*float64(len(lines)) + lineHeight/2
	for i, line := range lines {
		drawOutlinedMemeLine(dc, line, float64(StickerSize)/2, startY+float64(i)*lineHeight, fontPath, emojiFont, fontSize)
	}

	pngPath := filepath.Join(workDir, "meme_text_render.png")
	f, err := os.Create(pngPath)
	if err != nil {
		return nil, err
	}
	if err := dc.SavePNG(pngPath); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}

	webpPath := filepath.Join(workDir, "meme_text.webp")
	for _, quality := range []int{88, 70, 50, 30} {
		cmd := exec.CommandContext(ctx, ffmpegPath,
			"-y", "-v", "error", "-i", pngPath,
			"-frames:v", "1", "-vf", "scale=512:512:flags=lanczos,format=rgba",
			"-c:v", "libwebp", "-quality", fmt.Sprint(quality),
			"-preset", "picture", "-lossless", "0", "-f", "webp", webpPath,
		)
		if output, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("ffmpeg webp error: %w (output: %s)", err, string(output))
		}
		data, err := os.ReadFile(webpPath)
		if err != nil {
			return nil, err
		}
		if IsValidWebP(data) && len(data) <= MaxStickerSize {
			return data, nil
		}
	}
	return nil, fmt.Errorf("hasil stiker melebihi batas %d KB", MaxStickerSize/1024)
}

func drawOutlinedMemeLine(dc *gg.Context, line string, centerX, centerY float64, primaryFont, emojiFont string, size float64) {
	type glyph struct {
		value string
		face string
		width float64
	}
	glyphs := make([]glyph, 0, len([]rune(line)))
	totalWidth := 0.0
	for _, r := range line {
		if r == '\uFE0F' || r == '\u200D' || unicode.Is(unicode.Mn, r) {
			continue
		}
		font := primaryFont
		if isEmojiRune(r) && emojiFont != "" {
			font = emojiFont
		}
		_ = dc.LoadFontFace(font, size)
		value := string(r)
		width, _ := dc.MeasureString(value)
		glyphs = append(glyphs, glyph{value: value, face: font, width: width})
		totalWidth += width
	}

	x := centerX - totalWidth/2
	for _, g := range glyphs {
		_ = dc.LoadFontFace(g.face, size)
		// Draw each glyph with a thick black stroke, then the white foreground.
		for dx := -3.0; dx <= 3.0; dx += 1.5 {
			for dy := -3.0; dy <= 3.0; dy += 1.5 {
				dc.SetColor(color.Black)
				dc.DrawStringAnchored(g.value, x+dx, centerY+dy, 0, 0.5)
			}
		}
		dc.SetColor(color.White)
		dc.DrawStringAnchored(g.value, x, centerY, 0, 0.5)
		x += g.width
	}
}
