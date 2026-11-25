package handlers

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"bot-wa/internal/config"
	"bot-wa/internal/downloader"
	"bot-wa/internal/sticker"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

type StickerHandler struct {
	cfg    *config.Config
	client *whatsmeow.Client
	// Resolved FFmpeg path; empty until resolveFfmpeg succeeds.
	ffmpegPath string
	metadataMu sync.Mutex
	pendingMetadata map[string]StickerMetadata
}

func NewStickerHandler(cfg *config.Config, client *whatsmeow.Client) *StickerHandler {
	return &StickerHandler{
		cfg:    cfg,
		client: client,
	}
}

// FfmpegReady reports whether the FFmpeg binary can be located.
func (h *StickerHandler) FfmpegReady() bool {
	return h.withFfmpeg(func(path string) error {
		log.Info().Str("path", path).Msg("FFmpeg ditemukan; perintah stiker siap digunakan")
		return nil
	}) == nil
}



// withFfmpeg resolves the FFmpeg binary once and runs fn with it.
func (h *StickerHandler) withFfmpeg(fn func(path string) error) error {
	if h.ffmpegPath == "" {
		resolved, err := sticker.ResolveBinary(h.cfg.FfmpegPath, "ffmpeg")
		if err != nil {
			return err
		}
		h.ffmpegPath = resolved
	}
	return fn(h.ffmpegPath)
}

func (h *StickerHandler) HandleImageSticker(ctx context.Context, evt *events.Message, imgMsg *waProto.ImageMessage, caption string) error {
	zones, err := sticker.ParseStickerText(caption, h.cfg.StickerDefaultPos, h.cfg.StickerMaxChars)
	if err != nil {
		return err
	}

	// Download media buffer from WhatsApp
	data, err := h.client.Download(ctx, imgMsg)
	if err != nil {
		return fmt.Errorf("gagal mengunduh foto: %w", err)
	}

	workDir, err := os.MkdirTemp(h.cfg.TmpDir, "stk-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	var webpData []byte
	if err := h.withFfmpeg(func(path string) error {
		var encErr error
		webpData, encErr = sticker.CreateStickerWebP(ctx, data, zones, h.cfg.FontPath, path, workDir)
		return encErr
	}); err != nil {
		return fmt.Errorf("gagal merender stiker: %w", err)
	}

	return h.sendSticker(ctx, evt.Info.Chat, webpData, false)
}

func (h *StickerHandler) HandleVideoSticker(ctx context.Context, evt *events.Message, vidMsg *waProto.VideoMessage) error {
	data, err := h.client.Download(ctx, vidMsg)
	if err != nil {
		return fmt.Errorf("gagal mengunduh video: %w", err)
	}

	workDir, err := os.MkdirTemp(h.cfg.TmpDir, "anim-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	srcPath := filepath.Join(workDir, "source.mp4")
	if err := os.WriteFile(srcPath, data, 0644); err != nil {
		return err
	}

	var webpData []byte
	if err := h.withFfmpeg(func(path string) error {
		var encErr error
		webpData, encErr = sticker.CreateAnimatedSticker(ctx, path, srcPath, workDir, h.cfg.StickerAnimMaxSec, h.cfg.StickerAnimMaxKB)
		return encErr
	}); err != nil {
		return err
	}

	return h.sendSticker(ctx, evt.Info.Chat, webpData, true)
}

// HandleAudioSticker turns a video and its audio track into one WhatsApp
// sound-sticker message. The visual and audio tracks are uploaded separately.
func (h *StickerHandler) HandleAudioSticker(ctx context.Context, evt *events.Message, vidMsg *waProto.VideoMessage) error {
	data, err := h.client.Download(ctx, vidMsg)
	if err != nil {
		return fmt.Errorf("gagal mengunduh video: %w", err)
	}

	workDir, err := os.MkdirTemp(h.cfg.TmpDir, "saudio-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	srcPath := filepath.Join(workDir, "source.mp4")
	if err := os.WriteFile(srcPath, data, 0600); err != nil {
		return err
	}

	ffprobePath, err := sticker.ResolveBinary(h.cfg.FfprobePath, "ffprobe")
	if err != nil {
		return fmt.Errorf("ffprobe tidak tersedia: %w", err)
	}
	videoInfo, err := downloader.ProbeVideo(ctx, ffprobePath, srcPath)
	if err != nil {
		return fmt.Errorf("gagal memeriksa video: %w", err)
	}
	if !videoInfo.HasAudio {
		return fmt.Errorf("video tidak memiliki audio. Kirim video yang mempunyai suara")
	}

	maxSeconds := h.cfg.StickerAnimMaxSec
	if maxSeconds <= 0 {
		return fmt.Errorf("STICKER_ANIM_MAX_SEC harus lebih dari 0")
	}
	if maxSeconds > sticker.AudioStickerMaxSeconds {
		maxSeconds = sticker.AudioStickerMaxSeconds
	}

	audioPath := sticker.AudioStickerOutputPath(workDir)
	var audioData []byte
	if err := h.withFfmpeg(func(path string) error {
		var extractErr error
		audioData, extractErr = sticker.ExtractAudioSticker(ctx, path, srcPath, audioPath, maxSeconds)
		return extractErr
	}); err != nil {
		return err
	}

	audioInfo, err := downloader.ProbeVideo(ctx, ffprobePath, audioPath)
	if err != nil {
		return fmt.Errorf("gagal membaca durasi audio hasil konversi: %w", err)
	}
	if !audioInfo.HasAudio || audioInfo.DurationSec < 1 {
		return fmt.Errorf("audio video terlalu pendek untuk dijadikan stiker suara (minimal 1 detik)")
	}
	if audioInfo.DurationSec > sticker.AudioStickerMaxSeconds {
		return fmt.Errorf("durasi audio melewati batas %d detik", sticker.AudioStickerMaxSeconds)
	}

	var webpData []byte
	if err := h.withFfmpeg(func(path string) error {
		var encodeErr error
		webpData, encodeErr = sticker.CreateAnimatedSticker(
			ctx, path, srcPath, workDir, audioInfo.DurationSec, h.cfg.StickerAnimMaxKB,
		)
		return encodeErr
	}); err != nil {
		return fmt.Errorf("gagal membuat animasi stiker: %w", err)
	}
	if !sticker.IsValidAudioStickerWebP(webpData) {
		return fmt.Errorf("hasil WebP stiker suara melebihi batas ukuran WhatsApp (%d KB)", sticker.AudioStickerMaxWebPBytes/1024)
	}

	seconds := uint32(math.Ceil(audioInfo.DurationSec))
	if seconds < 1 {
		seconds = 1
	}
	if seconds > sticker.AudioStickerMaxSeconds {
		seconds = sticker.AudioStickerMaxSeconds
	}
	return h.sendAudioSticker(ctx, evt.Info.Chat, webpData, audioData, seconds)
}

func (h *StickerHandler) sendAudioSticker(ctx context.Context, chat types.JID, webpData, audioData []byte, seconds uint32) error {
	webpData, metadata, err := h.applyAudioStickerMetadata(chat.String(), webpData, audioData)
	if err != nil {
		return fmt.Errorf("gagal menyematkan metadata audio stiker: %w", err)
	}
	if !sticker.IsValidAudioStickerWebP(webpData) {
		return fmt.Errorf("ukuran WebP stiker suara setelah metadata melebihi batas WhatsApp")
	}
	if len(audioData) == 0 || len(audioData) > sticker.AudioStickerMaxAudioBytes {
		return fmt.Errorf("ukuran audio stiker tidak valid (maksimal %d KB)", sticker.AudioStickerMaxAudioBytes/1024)
	}

	stickerUpload, err := h.client.UploadSticker(ctx, webpData)
	if err != nil {
		return fmt.Errorf("gagal mengunggah gambar stiker: %w", err)
	}
	audioUpload, err := h.client.Upload(ctx, audioData, whatsmeow.MediaAudio)
	if err != nil {
		return fmt.Errorf("gagal mengunggah audio stiker: %w", err)
	}

	now := time.Now()
	audioMessage := &waProto.AudioMessage{
		URL:               proto.String(audioUpload.URL),
		DirectPath:        proto.String(audioUpload.DirectPath),
		MediaKey:          audioUpload.MediaKey,
		Mimetype:          proto.String("audio/ogg; codecs=opus"),
		FileEncSHA256:     audioUpload.FileEncSHA256,
		FileSHA256:        audioUpload.FileSHA256,
		FileLength:        proto.Uint64(audioUpload.FileLength),
		MediaKeyTimestamp: proto.Int64(now.Unix()),
		Seconds:           proto.Uint32(seconds),
		ContextInfo:       &waProto.ContextInfo{},
	}
	stickerMessage := &waProto.StickerMessage{
		URL:               proto.String(stickerUpload.URL),
		DirectPath:        proto.String(stickerUpload.DirectPath),
		MediaKey:          stickerUpload.MediaKey,
		Mimetype:          proto.String("image/webp"),
		FileEncSHA256:     stickerUpload.FileEncSHA256,
		FileSHA256:        stickerUpload.FileSHA256,
		FileLength:        proto.Uint64(stickerUpload.FileLength),
		MediaKeyTimestamp: proto.Int64(now.Unix()),
		IsAnimated:        proto.Bool(true),
		Width:             proto.Uint32(512),
		Height:            proto.Uint32(512),
		StickerSentTS:     proto.Int64(now.UnixMilli()),
		Audio: &waProto.StickerMessage_AudioMessage{
			AudioMessage: audioMessage,
		},
	}

	messageSecret := make([]byte, 32)
	if _, err := rand.Read(messageSecret); err != nil {
		return fmt.Errorf("gagal membuat message secret audio stiker: %w", err)
	}
	message := &waProto.Message{
		AudioStickerMessage: &waProto.FutureProofMessage{
			Message: &waProto.Message{
				StickerMessage: stickerMessage,
			},
		},
		MessageContextInfo: &waProto.MessageContextInfo{
			MessageSecret: messageSecret,
		},
	}
	if _, err := h.client.SendMessage(ctx, chat, message); err != nil {
		return fmt.Errorf("gagal mengirim audio stiker: %w", err)
	}

	log.Info().
		Str("chat", chat.String()).
		Str("pack", metadata.PackName).
		Str("author", metadata.Author).
		Uint32("seconds", seconds).
		Int("webp_bytes", len(webpData)).
		Int("audio_bytes", len(audioData)).
		Msg("Audio stiker berhasil dikirim")
	return nil
}

func (h *StickerHandler) HandleTextSticker(ctx context.Context, evt *events.Message, text string) error {
	zones, err := sticker.ParseStickerText(text, h.cfg.StickerDefaultPos, h.cfg.StickerMaxChars)
	if err != nil {
		return err
	}
	if !zones.HasText() {
		return fmt.Errorf("masukkan teks stiker. Contoh: `.st KEREN` atau `.st BAWAH | ATAS`")
	}

	workDir, err := os.MkdirTemp(h.cfg.TmpDir, "stkt-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	var webpData []byte
	if err := h.withFfmpeg(func(path string) error {
		var encErr error
		webpData, encErr = sticker.CreateBratStickerWebP(ctx, zones, h.cfg.FontPath, path, workDir)
		return encErr
	}); err != nil {
		return fmt.Errorf("gagal membuat stiker teks: %w", err)
	}

	return h.sendSticker(ctx, evt.Info.Chat, webpData, false)
}

func (h *StickerHandler) SendHint(ctx context.Context, chat types.JID) {
	hint := "💡 *Petunjuk Stiker*\n" +
		"Media ini belum diproses. Reply foto/video dengan `.s` untuk menjadikannya stiker:\n" +
		"• `.s` : stiker polos\n" +
		"• `.s TEKS` : stiker teks di bawah\n" +
		"• `.s | TEKS` : stiker teks di atas\n" +
		"• `.s BAWAH | ATAS` : teks dua zona\n" +
		"• Video/GIF + `.s` : stiker animasi (maks 6 detik)"

	_, _ = h.client.SendMessage(ctx, chat, &waProto.Message{
		Conversation: proto.String(hint),
	})
}

func (h *StickerHandler) sendSticker(ctx context.Context, chat types.JID, webpData []byte, isAnimated bool) error {
	var metadata StickerMetadata
	var err error
	webpData, metadata, err = h.applyStickerMetadata(chat.String(), webpData)
	if err != nil {
		return fmt.Errorf("gagal menyematkan metadata stiker: %w", err)
	}
	resp, err := h.client.Upload(ctx, webpData, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("gagal mengunggah stiker: %w", err)
	}

	stickerMsg := &waProto.StickerMessage{
		URL:           proto.String(resp.URL),
		DirectPath:    proto.String(resp.DirectPath),
		MediaKey:      resp.MediaKey,
		Mimetype:      proto.String("image/webp"),
		FileEncSHA256: resp.FileEncSHA256,
		FileSHA256:    resp.FileSHA256,
		FileLength:    proto.Uint64(uint64(len(webpData))),
		IsAnimated:    proto.Bool(isAnimated),
	}

	_, err = h.client.SendMessage(ctx, chat, &waProto.Message{
		StickerMessage: stickerMsg,
	})
	if err != nil {
		return fmt.Errorf("gagal mengirim stiker: %w", err)
	}

	log.Info().Str("chat", chat.String()).Str("pack", metadata.PackName).Str("author", metadata.Author).Bool("animated", isAnimated).Int("bytes", len(webpData)).Msg("Stiker berhasil dikirim")
	return nil
}

func (h *StickerHandler) React(ctx context.Context, chat, sender types.JID, messageID, emoji string) {
	react := h.client.BuildReaction(chat, sender, messageID, emoji)
	_, _ = h.client.SendMessage(ctx, chat, react)
}

func (h *StickerHandler) ReplyText(ctx context.Context, chat types.JID, text string) {
	_, _ = h.client.SendMessage(ctx, chat, &waProto.Message{
		Conversation: proto.String(text),
	})
}

func Delay(d time.Duration) {
	time.Sleep(d)
}
