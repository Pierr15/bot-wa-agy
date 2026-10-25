package storage

import (
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
)

func EnsureDirectories(dirs ...string) error {
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// CleanTempFiles removes entries in tmpDir older than maxAge.
func CleanTempFiles(tmpDir string, maxAge time.Duration) int {
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return 0
	}

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for _, entry := range entries {
		path := filepath.Join(tmpDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			if err := os.RemoveAll(path); err == nil {
				removed++
			}
		}
	}

	return removed
}

// StartJanitor launches a background goroutine that periodically cleans temporary files.
func StartJanitor(tmpDir string, interval time.Duration, maxAge time.Duration, stopCh <-chan struct{}) {
	go func() {
		// Run once on startup
		if n := CleanTempFiles(tmpDir, maxAge); n > 0 {
			log.Info().Int("removed", n).Msg("File sementara (temp) lama dibersihkan saat startup")
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				if n := CleanTempFiles(tmpDir, maxAge); n > 0 {
					log.Info().Int("removed", n).Msg("Pembersihan rutin file temp selesai")
				}
			}
		}
	}()
}
