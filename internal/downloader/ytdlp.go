package downloader

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
)

const TikTokFormat = "best[format_id!^=download_addr][format_note!*=watermark][vcodec^=h264]/best[format_id!^=download_addr][format_note!*=watermark]/bestvideo*[format_id!^=download_addr][format_note!*=watermark]+bestaudio/best"

type YtDlp struct {
	binaryPath     string
	cookiesPath    string
	maxSourceBytes int64
}

func NewYtDlp(binaryPath, cookiesPath string, maxSourceBytes int64) *YtDlp {
	return &YtDlp{
		binaryPath:     binaryPath,
		cookiesPath:    cookiesPath,
		maxSourceBytes: maxSourceBytes,
	}
}

func (y *YtDlp) Check() bool {
	cmd := exec.Command(y.binaryPath, "--version")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	log.Info().Str("version", strings.TrimSpace(string(out))).Msg("yt-dlp siap digunakan")
	return true
}

func (y *YtDlp) Download(ctx context.Context, link DetectedLink, tempDir string) ([]MediaFile, error) {
	outputTemplate := filepath.Join(tempDir, "%(autonumber)02d-%(id)s.%(ext)s")
	maxMB := y.maxSourceBytes / (1024 * 1024)
	if maxMB < 1 {
		maxMB = 100
	}

	args := []string{
		"--no-warnings",
		"--no-progress",
		"--newline",
		"--no-cache-dir",
		"--no-write-info-json",
		"--no-write-thumbnail",
		"--no-write-description",
		"--restrict-filenames",
		"--yes-playlist",
		"--playlist-items", "1-10",
		"--socket-timeout", "20",
		"--retries", "2",
		"--merge-output-format", "mp4",
		"--remux-video", "mp4",
		"--max-filesize", fmt.Sprintf("%dM", maxMB),
		"--output", outputTemplate,
	}

	if link.Platform == PlatformTikTok && !strings.Contains(link.URL, "/photo/") {
		args = append(args, "--format", TikTokFormat)
	}

	if link.Platform == PlatformInstagram && y.cookiesPath != "" {
		if _, err := os.Stat(y.cookiesPath); err == nil {
			args = append(args, "--cookies", y.cookiesPath)
		}
	}

	args = append(args, "--", link.URL)

	cmd := exec.CommandContext(ctx, y.binaryPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		stdErr := string(out)
		log.Warn().Str("err", err.Error()).Str("output", stdErr).Msg("yt-dlp download gagal")
		return nil, fmt.Errorf("%s", DescribeYtDlpFailure(stdErr))
	}

	files, err := ListMediaFiles(tempDir)
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("tidak ada file yang berhasil diunduh")
	}

	return files, nil
}

func DescribeYtDlpFailure(stderr string) string {
	lower := strings.ToLower(stderr)

	if matched, _ := regexp.MatchString(`login|sign in|cookies|private|not available|age-restricted|429|checkpoint`, lower); matched {
		return "Konten ini bersifat privat atau membutuhkan login Instagram/TikTok. Perbarui cookies jika diperlukan."
	}
	if strings.Contains(lower, "unsupported url") || strings.Contains(lower, "no formats found") {
		return "Format link ini belum didukung oleh bot."
	}
	if strings.Contains(lower, "larger than") || strings.Contains(lower, "max-filesize") {
		return "Ukuran media melebihi batas maksimal server."
	}
	if strings.Contains(lower, "404") || strings.Contains(lower, "not found") || strings.Contains(lower, "does not exist") {
		return "Konten tidak ditemukan atau telah dihapus."
	}
	if strings.Contains(lower, "timeout") || strings.Contains(lower, "timed out") {
		return "Waktu unduh habis (timeout). Silakan coba lagi."
	}

	return "Gagal mengunduh media dari link tersebut. Coba beberapa saat lagi."
}
