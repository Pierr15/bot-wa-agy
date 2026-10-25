package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"bot-wa/internal/config"
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
}

func NewStickerHandler(cfg *config.Config, client *whatsmeow.Client) *StickerHandler {
	return &StickerHandler{
		cfg:    cfg,
		client: client,
	}
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

	webpData, err := sticker.CreateStickerWebP(ctx, data, zones, h.cfg.FontPath, h.cfg.FfmpegPath, workDir)
	if err != nil {
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

	webpData, err := sticker.CreateAnimatedSticker(ctx, h.cfg.FfmpegPath, srcPath, workDir, h.cfg.StickerAnimMaxSec, h.cfg.StickerAnimMaxKB)
	if err != nil {
		return err
	}

	return h.sendSticker(ctx, evt.Info.Chat, webpData, true)
}

func (h *StickerHandler) HandleTextSticker(ctx context.Context, evt *events.Message, text string) error {
	zones, err := sticker.ParseStickerText(text, h.cfg.StickerDefaultPos, h.cfg.StickerMaxChars)
	if err != nil {
		return err
	}
	if !zones.HasText() {
		return fmt.Errorf("masukkan teks stiker. Contoh: `.t KEREN` atau `.t BAWAH | ATAS`")
	}

	workDir, err := os.MkdirTemp(h.cfg.TmpDir, "stkt-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	webpData, err := sticker.CreateStickerWebP(ctx, nil, zones, h.cfg.FontPath, h.cfg.FfmpegPath, workDir)
	if err != nil {
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

	log.Info().Str("chat", chat.String()).Bool("animated", isAnimated).Int("bytes", len(webpData)).Msg("Stiker berhasil dikirim")
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
