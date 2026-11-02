package sticker

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	vp8xFlagAlpha = 0x10
	vp8xFlagEXIF  = 0x08
)

type riffChunk struct {
	id      string
	payload []byte
}

// EmbedWebPMetadata menyematkan nama paket dan author stiker ke dalam chunk EXIF.
//
// Container WebP harus tetap valid supaya WhatsApp mau merender gambarnya:
//   - chunk EXIF hanya sah di format extended, jadi header VP8X dibuat jika
//     belum ada, atau flag EXIF-nya dinyalakan jika sudah ada (hasil ffmpeg
//     untuk gambar ber-alpha / animasi);
//   - VP8X harus menjadi chunk pertama dan EXIF diletakkan setelah data gambar;
//   - ukuran RIFF dihitung ulang.
func EmbedWebPMetadata(data []byte, packName, author string) ([]byte, error) {
	if !IsValidWebP(data) {
		return nil, fmt.Errorf("data bukan WebP yang valid")
	}
	packName = strings.TrimSpace(packName)
	author = strings.TrimSpace(author)
	if packName == "" || author == "" {
		return nil, fmt.Errorf("nama paket dan author wajib diisi")
	}

	chunks, err := parseRIFFChunks(data)
	if err != nil {
		return nil, err
	}

	// Buang EXIF lama (jika ada) agar tidak ganda.
	kept := chunks[:0:0]
	for _, c := range chunks {
		if c.id != "EXIF" {
			kept = append(kept, c)
		}
	}
	chunks = kept

	if len(chunks) == 0 {
		return nil, fmt.Errorf("WebP tidak memiliki data gambar")
	}

	if chunks[0].id == "VP8X" {
		if len(chunks[0].payload) < 10 {
			return nil, fmt.Errorf("chunk VP8X rusak")
		}
		payload := append([]byte(nil), chunks[0].payload...)
		payload[0] |= vp8xFlagEXIF
		chunks[0].payload = payload
	} else {
		vp8x, err := buildVP8X(chunks)
		if err != nil {
			return nil, err
		}
		chunks = append([]riffChunk{vp8x}, chunks...)
	}

	chunks = append(chunks, riffChunk{id: "EXIF", payload: buildTIFFMetadata(packName, author)})

	out := make([]byte, 12, len(data)+256)
	copy(out[0:4], "RIFF")
	copy(out[8:12], "WEBP")
	for _, c := range chunks {
		var hdr [8]byte
		copy(hdr[0:4], c.id)
		binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(c.payload)))
		out = append(out, hdr[:]...)
		out = append(out, c.payload...)
		if len(c.payload)%2 == 1 {
			out = append(out, 0)
		}
	}
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	return out, nil
}

// parseRIFFChunks memecah isi container WebP menjadi daftar chunk.
func parseRIFFChunks(data []byte) ([]riffChunk, error) {
	end := 8 + int(binary.LittleEndian.Uint32(data[4:8]))
	if end > len(data) || end < 12 {
		end = len(data)
	}
	var chunks []riffChunk
	pos := 12
	for pos+8 <= end {
		id := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		start := pos + 8
		if size < 0 || start+size > end {
			return nil, fmt.Errorf("chunk WebP %q terpotong/rusak", id)
		}
		chunks = append(chunks, riffChunk{id: id, payload: data[start : start+size]})
		pos = start + size + size%2
	}
	return chunks, nil
}

// buildVP8X membuat header VP8X dari ukuran bitstream VP8/VP8L pertama.
func buildVP8X(chunks []riffChunk) (riffChunk, error) {
	var w, h int
	var flags byte
	found := false
	for _, c := range chunks {
		p := c.payload
		switch c.id {
		case "VP8 ":
			if len(p) < 10 || p[3] != 0x9d || p[4] != 0x01 || p[5] != 0x2a {
				return riffChunk{}, fmt.Errorf("bitstream VP8 tidak valid")
			}
			w = int(binary.LittleEndian.Uint16(p[6:8]) & 0x3fff)
			h = int(binary.LittleEndian.Uint16(p[8:10]) & 0x3fff)
			found = true
		case "VP8L":
			if len(p) < 5 || p[0] != 0x2f {
				return riffChunk{}, fmt.Errorf("bitstream VP8L tidak valid")
			}
			bits := binary.LittleEndian.Uint32(p[1:5])
			w = int(bits&0x3fff) + 1
			h = int((bits>>14)&0x3fff) + 1
			if (bits>>28)&1 == 1 {
				flags |= vp8xFlagAlpha
			}
			found = true
		}
		if found {
			break
		}
	}
	if !found || w == 0 || h == 0 {
		return riffChunk{}, fmt.Errorf("WebP tidak memiliki data gambar")
	}
	flags |= vp8xFlagEXIF

	payload := make([]byte, 10)
	payload[0] = flags
	putUint24(payload[4:7], uint32(w-1))
	putUint24(payload[7:10], uint32(h-1))
	return riffChunk{id: "VP8X", payload: payload}, nil
}

func putUint24(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
}

// buildTIFFMetadata menulis metadata paket stiker dengan format yang dipakai
// WhatsApp: header TIFF little-endian, satu entri IFD bertag 0x5741 (tipe
// UNDEFINED) yang berisi JSON. Tidak ada prefix "Exif\0\0" karena itu hanya
// untuk JPEG; di dalam chunk EXIF WebP payload langsung diawali header TIFF.
func buildTIFFMetadata(packName, author string) []byte {
	payload, _ := json.Marshal(map[string]interface{}{
		"sticker-pack-id":        "bot-wa-agy",
		"sticker-pack-name":      packName,
		"sticker-pack-publisher": author,
		"emojis":                 []string{},
	})

	// Header TIFF (8) + jumlah entri (2) + satu entri (12) = 22 byte.
	const dataOffset = 8 + 2 + 12
	out := make([]byte, dataOffset+len(payload))
	copy(out[0:2], "II")
	binary.LittleEndian.PutUint16(out[2:4], 42)
	binary.LittleEndian.PutUint32(out[4:8], 8)
	binary.LittleEndian.PutUint16(out[8:10], 1)
	binary.LittleEndian.PutUint16(out[10:12], 0x5741) // tag sticker-pack WhatsApp
	binary.LittleEndian.PutUint16(out[12:14], 7)      // UNDEFINED
	binary.LittleEndian.PutUint32(out[14:18], uint32(len(payload)))
	binary.LittleEndian.PutUint32(out[18:22], dataOffset)
	copy(out[dataOffset:], payload)
	return out
}
