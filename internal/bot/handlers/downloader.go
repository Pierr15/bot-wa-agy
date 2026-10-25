package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"bot-wa/internal/config"
	"bot-wa/internal/downloader"
	"bot-wa/internal/storage"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

type DownloadPlan struct {
	Platform  downloader.Platform
	Files     []downloader.MediaFile
	WorkDir   string
	ExpiresAt time.Time
}

type DownloaderHandler struct {
	cfg     *config.Config
	client  *whatsmeow.Client
	ytdlp   *downloader.YtDlp
	plans   map[string]*DownloadPlan
	plansMu sync.RWMutex
	queueMu sync.Mutex
}

func NewDownloaderHandler(cfg *config.Config, client *whatsmeow.Client) *DownloaderHandler {
	return &DownloaderHandler{
		cfg:    cfg,
		client: client,
		ytdlp:  downloader.NewYtDlp(cfg.YtDlpPath, cfg.IGCookiePath, cfg.DownloadSourceMaxBytes),
		plans:  make(map[string]*DownloadPlan),
	}
}

func (h *DownloaderHandler) Handle(ctx context.Context, evt *events.Message, link downloader.DetectedLink) error {
	// Disk space check
	freeBytes, err := storage.GetFreeDiskBytes(h.cfg.TmpDir)
	if err == nil && freeBytes < h.cfg.MinFreeDiskBytes {
		return fmt.Errorf("penyimpanan HP hampir penuh (sisa %s). Hapus beberapa file terlebih dahulu", downloader.FormatBytes(int64(freeBytes)))
	}

	h.React(ctx, evt.Info.Chat, evt.Info.Sender, evt.Info.ID, "⏳")

	workDir, err := os.MkdirTemp(h.cfg.TmpDir, "dl-")
	if err != nil {
		h.React(ctx, evt.Info.Chat, evt.Info.Sender, evt.Info.ID, "❌")
		return err
	}

	// Serialized queue
	h.queueMu.Lock()
	defer h.queueMu.Unlock()

	var files []downloader.MediaFile

	// 1. Try yt-dlp first
	dlCtx, cancel := context.WithTimeout(ctx, time.Duration(h.cfg.DownloadTimeoutSec)*time.Second)
	defer cancel()

	files, err = h.ytdlp.Download(dlCtx, link, workDir)
	if err != nil && link.Platform == downloader.PlatformTikTok && h.cfg.EnableTikwmFallback {
		log.Warn().Str("err", err.Error()).Msg("yt-dlp gagal; mencoba fallback TikWM")
		files, err = downloader.DownloadTikTokWithTikwm(dlCtx, link.URL, workDir)
	}

	if err != nil || len(files) == 0 {
		os.RemoveAll(workDir)
		h.React(ctx, evt.Info.Chat, evt.Info.Sender, evt.Info.ID, "❌")
		if err != nil {
			return err
		}
		return fmt.Errorf("tidak ada file yang berhasil diunduh")
	}

	// Send initial bundle
	sent := h.SendBundle(ctx, evt.Info.Chat, files, link.Platform, workDir)
	if sent > 0 {
		h.React(ctx, evt.Info.Chat, evt.Info.Sender, evt.Info.ID, "✅")

		// Save plan for 10 minutes for .v/.a/.f replies
		h.plansMu.Lock()
		// Clean existing old plan dir if any
		if oldPlan, exists := h.plans[evt.Info.Chat.String()]; exists {
			os.RemoveAll(oldPlan.WorkDir)
		}
		h.plans[evt.Info.Chat.String()] = &DownloadPlan{
			Platform:  link.Platform,
			Files:     files,
			WorkDir:   workDir,
			ExpiresAt: time.Now().Add(10 * time.Minute),
		}
		h.plansMu.Unlock()
	} else {
		os.RemoveAll(workDir)
		h.React(ctx, evt.Info.Chat, evt.Info.Sender, evt.Info.ID, "❌")
		return fmt.Errorf("gagal mengirim file yang telah diunduh")
	}

	return nil
}

func (h *DownloaderHandler) HandleChoice(ctx context.Context, evt *events.Message, kind string, args string) error {
	chatKey := evt.Info.Chat.String()

	h.plansMu.RLock()
	plan, ok := h.plans[chatKey]
	h.plansMu.RUnlock()

	if !ok || time.Now().After(plan.ExpiresAt) {
		return fmt.Errorf("tidak ada download tersimpan atau sesi telah hangus. Kirim link TikTok/Instagram terlebih dahulu, lalu reply dengan `.v`, `.a`, atau `.f`")
	}

	h.React(ctx, evt.Info.Chat, evt.Info.Sender, evt.Info.ID, "⏳")

	switch kind {
	case "video":
		var videos []downloader.MediaFile
		for _, f := range plan.Files {
			if f.Kind == downloader.MediaKindVideo {
				videos = append(videos, f)
			}
		}
		if len(videos) == 0 {
			h.React(ctx, evt.Info.Chat, evt.Info.Sender, evt.Info.ID, "❌")
			return fmt.Errorf("download ini tidak memiliki video. Coba opsi foto `.f`")
		}
		h.SendBundle(ctx, evt.Info.Chat, videos, plan.Platform, plan.WorkDir)
		h.React(ctx, evt.Info.Chat, evt.Info.Sender, evt.Info.ID, "✅")

	case "audio":
		var videoFile *downloader.MediaFile
		for _, f := range plan.Files {
			if f.Kind == downloader.MediaKindVideo {
				videoFile = &f
				break
			}
		}
		if videoFile == nil {
			h.React(ctx, evt.Info.Chat, evt.Info.Sender, evt.Info.ID, "❌")
			return fmt.Errorf("tidak ada video untuk diekstrak audionya")
		}

		outAudio := filepath.Join(plan.WorkDir, fmt.Sprintf("audio-%d.m4a", time.Now().Unix()))
		if err := downloader.ExtractAudio(ctx, h.cfg.FfmpegPath, videoFile.Path, outAudio); err != nil {
			h.React(ctx, evt.Info.Chat, evt.Info.Sender, evt.Info.ID, "❌")
			return fmt.Errorf("gagal mengekstrak audio: %w", err)
		}

		fi, err := os.Stat(outAudio)
		if err != nil {
			h.React(ctx, evt.Info.Chat, evt.Info.Sender, evt.Info.ID, "❌")
			return err
		}

		audioMedia := downloader.MediaFile{
			Path:      outAudio,
			Name:      filepath.Base(outAudio),
			Extension: "m4a",
			Kind:      downloader.MediaKindAudio,
			Size:      fi.Size(),
		}

		if err := h.SendMediaFile(ctx, evt.Info.Chat, audioMedia, "🎵 Audio Musik", false); err != nil {
			h.React(ctx, evt.Info.Chat, evt.Info.Sender, evt.Info.ID, "❌")
			return err
		}
		h.React(ctx, evt.Info.Chat, evt.Info.Sender, evt.Info.ID, "✅")

	case "image":
		var images []downloader.MediaFile
		for _, f := range plan.Files {
			if f.Kind == downloader.MediaKindImage {
				images = append(images, f)
			}
		}
		if len(images) == 0 {
			h.React(ctx, evt.Info.Chat, evt.Info.Sender, evt.Info.ID, "❌")
			return fmt.Errorf("download ini tidak memiliki foto. Coba opsi video `.v`")
		}

		selected := images
		if trimmed := strings.TrimSpace(args); trimmed != "" {
			idx, err := strconv.Atoi(trimmed)
			if err != nil || idx < 1 || idx > len(images) {
				h.React(ctx, evt.Info.Chat, evt.Info.Sender, evt.Info.ID, "❌")
				return fmt.Errorf("nomor foto harus antara 1-%d. Contoh: `.f 1`", len(images))
			}
			selected = []downloader.MediaFile{images[idx-1]}
		}

		h.SendBundle(ctx, evt.Info.Chat, selected, plan.Platform, plan.WorkDir)
		h.React(ctx, evt.Info.Chat, evt.Info.Sender, evt.Info.ID, "✅")
	}

	return nil
}

func (h *DownloaderHandler) SendBundle(ctx context.Context, chat types.JID, files []downloader.MediaFile, platform downloader.Platform, workDir string) int {
	label := "TikTok"
	if platform == downloader.PlatformInstagram {
		label = "Instagram"
	}

	sent := 0
	for i, file := range files {
		current := file
		asDoc := false

		if current.Kind == downloader.MediaKindVideo {
			// Check duration
			if h.cfg.DownloadMaxDurationSec > 0 {
				probe, _ := downloader.ProbeVideo(ctx, h.cfg.FfprobePath, current.Path)
				if probe != nil && probe.DurationSec > float64(h.cfg.DownloadMaxDurationSec) {
					log.Warn().Float64("duration", probe.DurationSec).Msg("Video melebihi batas durasi")
					continue
				}
			}

			// Compress if size exceeds limit
			if current.Size > h.cfg.DownloadMaxBytes {
				compPath := filepath.Join(workDir, fmt.Sprintf("compressed-%d.mp4", i+1))
				if err := downloader.CompressVideo(ctx, h.cfg.FfmpegPath, current.Path, compPath, h.cfg.DownloadMaxBytes); err == nil {
					fi, _ := os.Stat(compPath)
					if fi != nil {
						current.Path = compPath
						current.Size = fi.Size()
					}
				} else {
					asDoc = true
				}
			}

			if current.Size > h.cfg.DownloadMaxBytes {
				asDoc = true
			}
		}

		caption := fmt.Sprintf("📥 *%s*", label)
		if len(files) > 1 {
			caption = fmt.Sprintf("📥 *%s* • (%d/%d)", label, i+1, len(files))
		}

		if err := h.SendMediaFile(ctx, chat, current, caption, asDoc); err == nil {
			sent++
		} else {
			log.Warn().Str("err", err.Error()).Str("file", current.Name).Msg("Gagal mengirim salah satu media")
		}

		time.Sleep(1 * time.Second) // pause between sends
	}

	return sent
}

func (h *DownloaderHandler) SendMediaFile(ctx context.Context, chat types.JID, file downloader.MediaFile, caption string, asDoc bool) error {
	data, err := os.ReadFile(file.Path)
	if err != nil {
		return err
	}

	if asDoc {
		resp, err := h.client.Upload(ctx, data, whatsmeow.MediaDocument)
		if err != nil {
			return err
		}
		_, err = h.client.SendMessage(ctx, chat, &waProto.Message{
			DocumentMessage: &waProto.DocumentMessage{
				URL:           proto.String(resp.URL),
				DirectPath:    proto.String(resp.DirectPath),
				MediaKey:      resp.MediaKey,
				Mimetype:      proto.String("application/octet-stream"),
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    proto.Uint64(uint64(len(data))),
				FileName:      proto.String(file.Name),
				Caption:       proto.String(caption),
			},
		})
		return err
	}

	switch file.Kind {
	case downloader.MediaKindVideo:
		resp, err := h.client.Upload(ctx, data, whatsmeow.MediaVideo)
		if err != nil {
			return err
		}
		_, err = h.client.SendMessage(ctx, chat, &waProto.Message{
			VideoMessage: &waProto.VideoMessage{
				URL:           proto.String(resp.URL),
				DirectPath:    proto.String(resp.DirectPath),
				MediaKey:      resp.MediaKey,
				Mimetype:      proto.String("video/mp4"),
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    proto.Uint64(uint64(len(data))),
				Caption:       proto.String(caption),
			},
		})
		return err

	case downloader.MediaKindImage:
		resp, err := h.client.Upload(ctx, data, whatsmeow.MediaImage)
		if err != nil {
			return err
		}
		_, err = h.client.SendMessage(ctx, chat, &waProto.Message{
			ImageMessage: &waProto.ImageMessage{
				URL:           proto.String(resp.URL),
				DirectPath:    proto.String(resp.DirectPath),
				MediaKey:      resp.MediaKey,
				Mimetype:      proto.String("image/jpeg"),
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    proto.Uint64(uint64(len(data))),
				Caption:       proto.String(caption),
			},
		})
		return err

	case downloader.MediaKindAudio:
		resp, err := h.client.Upload(ctx, data, whatsmeow.MediaAudio)
		if err != nil {
			return err
		}
		_, err = h.client.SendMessage(ctx, chat, &waProto.Message{
			AudioMessage: &waProto.AudioMessage{
				URL:           proto.String(resp.URL),
				DirectPath:    proto.String(resp.DirectPath),
				MediaKey:      resp.MediaKey,
				Mimetype:      proto.String("audio/mp4"),
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    proto.Uint64(uint64(len(data))),
			},
		})
		return err
	}

	return nil
}

func (h *DownloaderHandler) React(ctx context.Context, chat, sender types.JID, messageID, emoji string) {
	react := h.client.BuildReaction(chat, sender, messageID, emoji)
	_, _ = h.client.SendMessage(ctx, chat, react)
}
