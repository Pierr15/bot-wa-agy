package sticker

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type TextZones struct {
	Top    string
	Bottom string
	Center string
}

func (tz TextZones) HasText() bool {
	return tz.Top != "" || tz.Bottom != "" || tz.Center != ""
}

// ParseStickerText parses commands like:
// - "HALO" -> Bottom: "HALO" (if defaultPos is bottom)
// - "| HALO" -> Top: "HALO"
// - "BAWAH | ATAS" -> Bottom: "BAWAH", Top: "ATAS"
// - "atas: TEKS 1 | bawah: TEKS 2"
func ParseStickerText(raw, defaultPos string, maxChars int) (*TextZones, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return &TextZones{}, nil
	}

	if utf8.RuneCountInString(text) > maxChars {
		return nil, fmt.Errorf("teks terlalu panjang (maksimal %d karakter)", maxChars)
	}

	zones := &TextZones{}

	// Check if pipe syntax is used: LEFT (Bottom) | RIGHT (Top)
	if strings.Contains(text, "|") {
		parts := strings.SplitN(text, "|", 2)
		bottom := strings.TrimSpace(parts[0])
		top := strings.TrimSpace(parts[1])

		zones.Bottom = strings.ToUpper(bottom)
		zones.Top = strings.ToUpper(top)
		return zones, nil
	}

	// Check for explicit prefix
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "atas:") {
		zones.Top = strings.ToUpper(strings.TrimSpace(text[5:]))
		return zones, nil
	}
	if strings.HasPrefix(lower, "bawah:") {
		zones.Bottom = strings.ToUpper(strings.TrimSpace(text[6:]))
		return zones, nil
	}
	if strings.HasPrefix(lower, "tengah:") {
		zones.Center = strings.ToUpper(strings.TrimSpace(text[7:]))
		return zones, nil
	}

	// Default positioning
	upper := strings.ToUpper(text)
	switch defaultPos {
	case "top":
		zones.Top = upper
	case "center":
		zones.Center = upper
	default:
		zones.Bottom = upper
	}

	return zones, nil
}
