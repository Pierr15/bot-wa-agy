package sticker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

func AnimateEncoderAvailable(ffmpegPath string) bool {
	cmd := exec.Command(ffmpegPath, "-hide_banner", "-encoders")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "libwebp_anim")
}

func CreateAnimatedSticker(ctx context.Context, ffmpegPath, videoPath, workDir string, maxSec float64, maxKB int64) ([]byte, error) {
	if !AnimateEncoderAvailable(ffmpegPath) {
		return nil, fmt.Errorf("ffmpeg di perangkat belum mendukung encoder 'libwebp_anim'")
	}

	maxBytes := maxKB * 1024

	// Pass 1: FPS 10, Quality 50
	// Pass 2: FPS 8, Quality 25
	passes := []struct {
		fps     int
		quality int
	}{
		{fps: 10, quality: 50},
		{fps: 8, quality: 25},
	}

	scaleFilter := "scale=512:512:force_original_aspect_ratio=increase,crop=512:512"

	for i, p := range passes {
		targetWebp := filepath.Join(workDir, fmt.Sprintf("anim_pass%d.webp", i+1))

		args := []string{
			"-hide_banner",
			"-loglevel", "error",
			"-y",
			"-i", videoPath,
			"-t", fmt.Sprintf("%.2f", maxSec),
			"-vf", fmt.Sprintf("fps=%d,%s", p.fps, scaleFilter),
			"-c:v", "libwebp_anim",
			"-lossless", "0",
			"-quality", fmt.Sprintf("%d", p.quality),
			"-compression_level", "4",
			"-an",
			"-loop", "0",
			targetWebp,
		}

		cmd := exec.CommandContext(ctx, ffmpegPath, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Warn().Str("err", err.Error()).Str("output", string(out)).Msg("FFmpeg pass animasi gagal")
			continue
		}

		data, err := os.ReadFile(targetWebp)
		if err != nil {
			continue
		}

		if len(data) <= int(maxBytes) && IsValidWebP(data) {
			log.Debug().Int("bytes", len(data)).Int("pass", i+1).Msg("Stiker animasi berhasil dibuat")
			return data, nil
		}
	}

	// If slightly over limit on pass 2, still return it if valid WebP
	lastPath := filepath.Join(workDir, "anim_pass2.webp")
	data, err := os.ReadFile(lastPath)
	if err == nil && IsValidWebP(data) {
		return data, nil
	}

	return nil, fmt.Errorf("video terlalu panjang atau kompleks untuk dijadikan stiker animasi (batas %d KB)", maxKB)
}
