package sticker

import (
	"encoding/binary"
	"testing"
)

func TestEmbedWebPMetadataAppendsExifChunk(t *testing.T) {
	webp := make([]byte, 12)
	copy(webp[:4], "RIFF")
	copy(webp[8:12], "WEBP")
	binary.LittleEndian.PutUint32(webp[4:8], uint32(len(webp)-8))

	got, err := EmbedWebPMetadata(webp, "✨ Stiker", "Javier")
	if err != nil {
		t.Fatalf("EmbedWebPMetadata returned error: %v", err)
	}
	if !IsValidWebP(got) {
		t.Fatal("result is not a valid RIFF/WEBP container")
	}
	if string(got[12:16]) != "EXIF" {
		t.Fatalf("expected EXIF chunk, got %q", got[12:16])
	}
	if binary.LittleEndian.Uint32(got[4:8]) != uint32(len(got)-8) {
		t.Fatal("RIFF size was not updated")
	}
	if string(got[len(got)-len("Javier")-1:len(got)-1]) != "Javier" {
		t.Fatal("author metadata was not embedded")
	}
}

func TestEmbedWebPMetadataRejectsInvalidInput(t *testing.T) {
	if _, err := EmbedWebPMetadata([]byte("not webp"), "pack", "author"); err == nil {
		t.Fatal("expected invalid WebP error")
	}
	if _, err := EmbedWebPMetadata([]byte("RIFFxxxxWEBP"), "", "author"); err == nil {
		t.Fatal("expected empty metadata error")
	}
}
