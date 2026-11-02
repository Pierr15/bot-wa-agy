package sticker

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func testChunk(id string, payload []byte) []byte {
	b := make([]byte, 8, 8+len(payload)+1)
	copy(b[:4], id)
	binary.LittleEndian.PutUint32(b[4:8], uint32(len(payload)))
	b = append(b, payload...)
	if len(payload)%2 == 1 {
		b = append(b, 0)
	}
	return b
}

func testWebP(chunks ...[]byte) []byte {
	out := []byte("RIFF\x00\x00\x00\x00WEBP")
	for _, c := range chunks {
		out = append(out, c...)
	}
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	return out
}

// Header VP8 lossy minimal 512x512 (hanya header frame, cukup untuk parser).
func testVP8() []byte {
	p := make([]byte, 10)
	copy(p[3:6], []byte{0x9d, 0x01, 0x2a})
	binary.LittleEndian.PutUint16(p[6:8], 512)
	binary.LittleEndian.PutUint16(p[8:10], 512)
	return testChunk("VP8 ", p)
}

func chunkIDs(t *testing.T, data []byte) []string {
	t.Helper()
	chunks, err := parseRIFFChunks(data)
	if err != nil {
		t.Fatalf("parseRIFFChunks: %v", err)
	}
	ids := make([]string, len(chunks))
	for i, c := range chunks {
		ids[i] = c.id
	}
	return ids
}

func TestEmbedWebPMetadataAddsVP8XForSimpleWebP(t *testing.T) {
	got, err := EmbedWebPMetadata(testWebP(testVP8()), "✨ Stiker", "Javier")
	if err != nil {
		t.Fatalf("EmbedWebPMetadata returned error: %v", err)
	}
	if !IsValidWebP(got) {
		t.Fatal("result is not a valid RIFF/WEBP container")
	}
	if binary.LittleEndian.Uint32(got[4:8]) != uint32(len(got)-8) {
		t.Fatal("RIFF size was not updated")
	}
	ids := chunkIDs(t, got)
	if len(ids) != 3 || ids[0] != "VP8X" || ids[1] != "VP8 " || ids[2] != "EXIF" {
		t.Fatalf("unexpected chunk order: %v", ids)
	}
	chunks, _ := parseRIFFChunks(got)
	vp8x := chunks[0].payload
	if vp8x[0]&vp8xFlagEXIF == 0 {
		t.Fatal("VP8X EXIF flag not set")
	}
	w := int(vp8x[4]) | int(vp8x[5])<<8 | int(vp8x[6])<<16
	h := int(vp8x[7]) | int(vp8x[8])<<8 | int(vp8x[9])<<16
	if w+1 != 512 || h+1 != 512 {
		t.Fatalf("canvas size = %dx%d, want 512x512", w+1, h+1)
	}
	if !bytes.Contains(got, []byte(`"sticker-pack-publisher":"Javier"`)) {
		t.Fatal("author metadata was not embedded")
	}
}

func TestEmbedWebPMetadataKeepsExistingVP8XAndSetsFlag(t *testing.T) {
	vp8x := make([]byte, 10)
	vp8x[0] = vp8xFlagAlpha
	binary.LittleEndian.PutUint16(vp8x[4:6], 511)
	binary.LittleEndian.PutUint16(vp8x[7:9], 511)
	in := testWebP(testChunk("VP8X", vp8x), testChunk("ALPH", []byte{1, 2, 3}), testVP8())

	got, err := EmbedWebPMetadata(in, "pack", "author")
	if err != nil {
		t.Fatal(err)
	}
	ids := chunkIDs(t, got)
	want := []string{"VP8X", "ALPH", "VP8 ", "EXIF"}
	if len(ids) != len(want) {
		t.Fatalf("chunks = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("chunks = %v, want %v", ids, want)
		}
	}
	chunks, _ := parseRIFFChunks(got)
	if chunks[0].payload[0] != vp8xFlagAlpha|vp8xFlagEXIF {
		t.Fatalf("VP8X flags = %#x, want alpha|exif", chunks[0].payload[0])
	}
	// Input tidak boleh termodifikasi.
	if in[20] != vp8xFlagAlpha {
		t.Fatal("input buffer was mutated")
	}
}

func TestEmbedWebPMetadataReplacesOldExif(t *testing.T) {
	first, err := EmbedWebPMetadata(testWebP(testVP8()), "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	second, err := EmbedWebPMetadata(first, "c", "d")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, id := range chunkIDs(t, second) {
		if id == "EXIF" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("EXIF chunks = %d, want 1", n)
	}
	if bytes.Contains(second, []byte(`"sticker-pack-name":"a"`)) {
		t.Fatal("old metadata still present")
	}
}

func TestEmbedWebPMetadataPadsOddLengthExif(t *testing.T) {
	for _, author := range []string{"x", "xy"} {
		got, err := EmbedWebPMetadata(testWebP(testVP8()), "p", author)
		if err != nil {
			t.Fatal(err)
		}
		if len(got)%2 != 0 {
			t.Fatalf("file length %d is odd (author %q)", len(got), author)
		}
	}
}

func TestEmbedWebPMetadataRejectsInvalidInput(t *testing.T) {
	if _, err := EmbedWebPMetadata([]byte("not webp"), "pack", "author"); err == nil {
		t.Fatal("expected invalid WebP error")
	}
	if _, err := EmbedWebPMetadata([]byte("RIFFxxxxWEBP"), "", "author"); err == nil {
		t.Fatal("expected empty metadata error")
	}
	if _, err := EmbedWebPMetadata(testWebP(), "pack", "author"); err == nil {
		t.Fatal("expected error for WebP without image data")
	}
}
