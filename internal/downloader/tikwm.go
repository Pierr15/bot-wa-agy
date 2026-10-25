package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
)

type TikwmResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		ID     string   `json:"id"`
		Title  string   `json:"title"`
		Play   string   `json:"play"`
		WMPlay string   `json:"wmplay"`
		Music  string   `json:"music"`
		Images []string `json:"images"`
	} `json:"data"`
}

func DownloadTikTokWithTikwm(ctx context.Context, rawURL, tempDir string) ([]MediaFile, error) {
	apiURL := fmt.Sprintf("https://www.tikwm.com/api/?url=%s&hd=1", rawURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data TikwmResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if data.Code != 0 {
		return nil, fmt.Errorf("tikwm error: %s", data.Msg)
	}

	// 1. Photos carousel
	if len(data.Data.Images) > 0 {
		var downloaded []MediaFile
		for i, imgURL := range data.Data.Images {
			if i >= 10 { // Max 10 images
				break
			}
			fileName := fmt.Sprintf("%02d-%s.jpg", i+1, data.Data.ID)
			filePath := filepath.Join(tempDir, fileName)

			if err := downloadFile(ctx, imgURL, filePath); err != nil {
				log.Warn().Str("err", err.Error()).Msg("Gagal mengunduh salah satu slide foto TikTok")
				continue
			}

			fi, err := os.Stat(filePath)
			if err == nil {
				downloaded = append(downloaded, MediaFile{
					Path:      filePath,
					Name:      fileName,
					Extension: "jpg",
					Kind:      MediaKindImage,
					Size:      fi.Size(),
				})
			}
		}
		if len(downloaded) > 0 {
			return downloaded, nil
		}
	}

	// 2. Video no watermark
	targetVideo := data.Data.Play
	if targetVideo == "" {
		targetVideo = data.Data.WMPlay
	}

	if targetVideo != "" {
		fileName := fmt.Sprintf("01-%s.mp4", data.Data.ID)
		filePath := filepath.Join(tempDir, fileName)
		if err := downloadFile(ctx, targetVideo, filePath); err != nil {
			return nil, err
		}

		fi, err := os.Stat(filePath)
		if err != nil {
			return nil, err
		}

		return []MediaFile{{
			Path:      filePath,
			Name:      fileName,
			Extension: "mp4",
			Kind:      MediaKindVideo,
			Size:      fi.Size(),
		}}, nil
	}

	return nil, fmt.Errorf("tidak menemukan konten video/foto dari TikWM")
}

func downloadFile(ctx context.Context, fileURL, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status response %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
