package sticker

import (
	"encoding/binary"
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

// buildTIFFMetadata writes a little-endian TIFF IFD with ImageDescription
// and Artist ASCII fields, which are commonly used for sticker EXIF metadata.
func buildTIFFMetadata(packName, author string) []byte {
	description := []byte(packName + "\x00")
	artist := []byte(author + "\x00")
	// TIFF header (8 bytes), IFD count (2), two entries (24), next IFD (4).
	const dataOffset = 8 + 2 + 2*12 + 4
	descOffset := uint32(dataOffset)
	artistOffset := descOffset + uint32(len(description))
	out := make([]byte, int(artistOffset)+len(artist))
	copy(out[:2], "II")
	binary.LittleEndian.PutUint16(out[2:4], 42)
	binary.LittleEndian.PutUint32(out[4:8], 8)
	binary.LittleEndian.PutUint16(out[8:10], 2)
	// ImageDescription, ASCII
	binary.LittleEndian.PutUint16(out[10:12], 0x010e)
	binary.LittleEndian.PutUint16(out[12:14], 2)
	binary.LittleEndian.PutUint32(out[14:18], uint32(len(description)))
	binary.LittleEndian.PutUint32(out[18:22], descOffset)
	// Artist, ASCII
	binary.LittleEndian.PutUint16(out[22:24], 0x013b)
	binary.LittleEndian.PutUint16(out[24:26], 2)
	binary.LittleEndian.PutUint32(out[26:30], uint32(len(artist)))
	binary.LittleEndian.PutUint32(out[30:34], artistOffset)
	copy(out[dataOffset:], description)
	copy(out[artistOffset:], artist)
	return append([]byte("Exif\x00\x00"), out...)
}
