package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
)

type MediaKind string

const (
	MediaKindVideo MediaKind = "video"
	MediaKindImage MediaKind = "image"
	MediaKindAudio MediaKind = "audio"
)

type MediaFile struct {
	Path      string
	Name      string
	Extension string
	Kind      MediaKind
	Size      int64
}

type VideoProbeInfo struct {
	Codec       string
	DurationSec float64
	HasAudio    bool
}

func ListMediaFiles(dir string) ([]MediaFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []MediaFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(entry.Name()), "."))
		var kind MediaKind

		switch ext {
		case "mp4", "mkv", "mov", "webm":
			kind = MediaKindVideo
		case "jpg", "jpeg", "png", "webp":
			kind = MediaKindImage
		case "mp3", "m4a", "aac", "ogg":
			kind = MediaKindAudio
		default:
			continue
		}

		files = append(files, MediaFile{
			Path:      filepath.Join(dir, entry.Name()),
			Name:      entry.Name(),
			Extension: ext,
			Kind:      kind,
			Size:      info.Size(),
		})
	}

	return files, nil
}

func ProbeVideo(ctx context.Context, ffprobePath, filePath string) (*VideoProbeInfo, error) {
	args := []string{
		"-v", "error",
		"-show_entries", "stream=codec_type,codec_name:format=duration",
		"-of", "json",
		filePath,
	}

	cmd := exec.CommandContext(ctx, ffprobePath, args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var data struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}

	if err := json.Unmarshal(out, &data); err != nil {
		return nil, err
	}

	res := &VideoProbeInfo{}
	if data.Format.Duration != "" {
		if d, err := strconv.ParseFloat(data.Format.Duration, 64); err == nil {
			res.DurationSec = d
		}
	}

	for _, s := range data.Streams {
		if s.CodecType == "video" && res.Codec == "" {
			res.Codec = s.CodecName
		}
		if s.CodecType == "audio" {
			res.HasAudio = true
		}
	}

	return res, nil
}

func CompressVideo(ctx context.Context, ffmpegPath, inputPath, outputPath string, targetBytes int64) error {
	// First probe to estimate duration
	probe, err := ProbeVideo(ctx, "ffprobe", inputPath)
	duration := 30.0
	if err == nil && probe.DurationSec > 0 {
		duration = probe.DurationSec
	}

	// Calculate target bitrate (bits per sec) minus 64k for audio
	targetBits := float64(targetBytes*8) * 0.90 // 10% safety margin
	totalBitrate := targetBits / duration
	videoBitrate := int(totalBitrate) - 64_000
	if videoBitrate < 150_000 {
		videoBitrate = 150_000
	}

	args := []string{
		"-y",
		"-i", inputPath,
		"-c:v", "libx264",
		"-b:v", fmt.Sprintf("%d", videoBitrate),
		"-maxrate", fmt.Sprintf("%d", int(float64(videoBitrate)*1.2)),
		"-bufsize", fmt.Sprintf("%d", videoBitrate*2),
		"-preset", "veryfast",
		"-vf", "scale='min(720,iw)':-2",
		"-c:a", "aac",
		"-b:a", "64k",
		"-movflags", "+faststart",
		outputPath,
	}

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Warn().Str("err", err.Error()).Str("output", string(out)).Msg("Kompresi video ffmpeg gagal")
		return err
	}

	return nil
}

func ExtractAudio(ctx context.Context, ffmpegPath, inputPath, outputPath string) error {
	args := []string{
		"-y",
		"-i", inputPath,
		"-vn",
		"-c:a", "aac",
		"-b:a", "128k",
		outputPath,
	}

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Warn().Str("err", err.Error()).Str("output", string(out)).Msg("Ekstrak audio ffmpeg gagal")
		return err
	}

	return nil
}

func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
