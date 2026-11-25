package sticker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	// AudioStickerMaxSeconds is the maximum duration accepted by the current
	// WhatsApp sound-sticker implementation.
	AudioStickerMaxSeconds = 7

	// AudioStickerMaxAudioBytes keeps the Opus track comfortably below the
	// current WhatsApp Web upload limit.
	AudioStickerMaxAudioBytes = 256 * 1024

	// AudioStickerMaxWebPBytes is the current upper bound for the visual track.
	AudioStickerMaxWebPBytes = 1024 * 1024
)

// ExtractAudioSticker converts the first audio stream in a source video to a
// mono Ogg/Opus track suitable for a WhatsApp audio sticker.
func ExtractAudioSticker(ctx context.Context, ffmpegPath, inputPath, outputPath string, maxSeconds float64) ([]byte, error) {
	if maxSeconds <= 0 {
		return nil, fmt.Errorf("durasi audio stiker harus lebih dari 0 detik")
	}
	if maxSeconds > AudioStickerMaxSeconds {
		maxSeconds = AudioStickerMaxSeconds
	}

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-nostdin",
		"-y",
		"-i", inputPath,
		"-map", "0:a:0",
		"-vn",
		"-t", fmt.Sprintf("%.2f", maxSeconds),
		"-ac", "1",
		"-ar", "48000",
		"-c:a", "libopus",
		"-b:a", "32k",
		"-application", "audio",
		"-f", "ogg",
		outputPath,
	}

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("gagal mengonversi audio ke Ogg/Opus: %w (%s)", err, string(output))
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca hasil konversi audio: %w", err)
	}
	if len(data) < 64 || !bytes.HasPrefix(data, []byte("OggS")) ||
		!bytes.Contains(data[:min(len(data), 128)], []byte("OpusHead")) {
		return nil, fmt.Errorf("hasil audio bukan Ogg/Opus yang valid")
	}
	if len(data) > AudioStickerMaxAudioBytes {
		return nil, fmt.Errorf("audio hasil konversi terlalu besar (%d KB; batas %d KB)",
			len(data)/1024, AudioStickerMaxAudioBytes/1024)
	}

	// Avoid retaining a potentially large backing array from an intermediate
	// read buffer after the temporary directory is removed.
	return append([]byte(nil), data...), nil
}

// IsValidAudioStickerWebP performs basic RIFF/WebP validation and checks the
// sound-sticker size limit.
func IsValidAudioStickerWebP(data []byte) bool {
	return len(data) >= 12 &&
		len(data) <= AudioStickerMaxWebPBytes &&
		string(data[:4]) == "RIFF" &&
		string(data[8:12]) == "WEBP"
}

// Keep filepath imported here so callers can build a predictable temporary
// output path without duplicating naming conventions.
func AudioStickerOutputPath(workDir string) string {
	return filepath.Join(workDir, "sound.ogg")
}
