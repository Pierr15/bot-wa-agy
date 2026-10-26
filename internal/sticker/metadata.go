package sticker

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
)

// EmbedWebPMetadata adds an EXIF chunk containing the sticker pack name and
// publisher. Existing RIFF chunks are preserved.
func EmbedWebPMetadata(data []byte, packName, author string) ([]byte, error) {
	if !IsValidWebP(data) {
		return nil, fmt.Errorf("data bukan WebP yang valid")
	}
	packName = strings.TrimSpace(packName)
	author = strings.TrimSpace(author)
	if packName == "" || author == "" {
		return nil, fmt.Errorf("nama paket dan author wajib diisi")
	}
	exif := buildTIFFMetadata(packName, author)
	chunk := make([]byte, 8+len(exif)+(len(exif)%2))
	copy(chunk[:4], "EXIF")
	binary.LittleEndian.PutUint32(chunk[4:8], uint32(len(exif)))
	copy(chunk[8:], exif)

	out := make([]byte, 0, len(data)+len(chunk))
	out = append(out, data[:len(data)]...)
	out = append(out, chunk...)
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	return out, nil
}

// buildTIFFMetadata writes the EXIF UserComment field used by WhatsApp sticker tooling.
func buildTIFFMetadata(packName, author string) []byte {
	// WhatsApp sticker tooling conventionally stores pack metadata in EXIF
	// UserComment as an ASCII-prefixed JSON object.
	payload, _ := json.Marshal(map[string]interface{}{
		"sticker-pack-id": "bot-wa-agy",
		"sticker-pack-name": packName,
		"sticker-pack-publisher": author,
		"emojis": []string{},
	})
	comment := append([]byte("ASCII\x00\x00\x00"), payload...)

	// TIFF header (8 bytes), one IFD entry (12), and next-IFD pointer (4).
	const dataOffset = 8 + 2 + 12 + 4
	out := make([]byte, dataOffset+len(comment))
	copy(out[:2], "II")
	binary.LittleEndian.PutUint16(out[2:4], 42)
	binary.LittleEndian.PutUint32(out[4:8], 8)
	binary.LittleEndian.PutUint16(out[8:10], 1)
	binary.LittleEndian.PutUint16(out[10:12], 0x9286) // UserComment
	binary.LittleEndian.PutUint16(out[12:14], 7)      // UNDEFINED
	binary.LittleEndian.PutUint32(out[14:18], uint32(len(comment)))
	binary.LittleEndian.PutUint32(out[18:22], uint32(dataOffset))
	copy(out[dataOffset:], comment)
	return append([]byte("Exif\x00\x00"), out...)
}
