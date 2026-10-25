package handlers

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"bot-wa/internal/config"
	"bot-wa/internal/storage"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type AdminHandler struct {
	cfg       *config.Config
	client    *whatsmeow.Client
	startTime time.Time
}

func NewAdminHandler(cfg *config.Config, client *whatsmeow.Client) *AdminHandler {
	return &AdminHandler{
		cfg:       cfg,
		client:    client,
		startTime: time.Now(),
	}
}

func (h *AdminHandler) BuildMenu(isOwner bool) string {
	var sb strings.Builder
	sb.WriteString("🤖 *Menu " + h.cfg.BotName + "*\n\n")

	sb.WriteString("📌 *Stiker Meme*\n")
	sb.WriteString("• Kirim foto dengan caption `.s` → Stiker polos\n")
	sb.WriteString("• Kirim foto dengan caption `.s TEKS` → Teks bawah\n")
	sb.WriteString("• Kirim foto dengan caption `.s | TEKS` → Teks atas\n")
	sb.WriteString("• Kirim foto dengan caption `.s BAWAH | ATAS` → Dua zona\n")
	sb.WriteString("• Kirim video/GIF (maks " + fmt.Sprintf("%.0f", h.cfg.StickerAnimMaxSec) + "s) caption `.s` → Stiker animasi\n")
	sb.WriteString("• `.t TEKS` → Stiker teks di dasar putih\n")
	sb.WriteString("• Reply foto/video yang ada dengan `.s` untuk diproses\n\n")

	sb.WriteString("📥 *Downloader*\n")
	sb.WriteString("• Kirim link TikTok / Instagram (post/reel/carousel)\n")
	sb.WriteString("• Reply hasil download dengan:\n")
	sb.WriteString("  - `.v` : kirim video\n")
	sb.WriteString("  - `.a` : ekstrak audio musik (m4a)\n")
	sb.WriteString("  - `.f [N]` : kirim semua foto atau foto ke-N\n\n")

	sb.WriteString("⚡ *Utilitas*\n")
	sb.WriteString("• `.ping` : cek status respon bot\n")
	sb.WriteString("• `.menu` : tampilkan bantuan ini\n")

	if isOwner {
		sb.WriteString("\n👑 *Menu Khusus Owner*\n")
		sb.WriteString("• `.stat` : cek uptime, RAM, disk & sistem\n")
		sb.WriteString("• `.id` : lihat detail JID chat & pengirim\n")
		sb.WriteString("• `.bc <teks>` : siarkan pesan ke nomor whitelist\n")
	}

	return sb.String()
}

func (h *AdminHandler) Stat() string {
	uptime := time.Since(h.startTime).Round(time.Second)

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	freeDisk, err := storage.GetFreeDiskBytes(h.cfg.TmpDir)
	diskStr := "N/A"
	if err == nil {
		diskStr = fmt.Sprintf("%.2f GB", float64(freeDisk)/(1024*1024*1024))
	}

	return fmt.Sprintf("📊 *Status Sistem*\n"+
		"• Bot: %s\n"+
		"• Uptime: %s\n"+
		"• Runtime: Go %s (%s/%s)\n"+
		"• Goroutines: %d\n"+
		"• RAM Alokasi: %.2f MB\n"+
		"• RAM Sys: %.2f MB\n"+
		"• Sisa Disk: %s\n"+
		"• Whitelist: %d nomor\n",
		h.cfg.BotName,
		uptime,
		runtime.Version(), runtime.GOOS, runtime.GOARCH,
		runtime.NumGoroutine(),
		float64(m.Alloc)/(1024*1024),
		float64(m.Sys)/(1024*1024),
		diskStr,
		len(h.cfg.AllowedNumbers)+len(h.cfg.OwnerNumbers),
	)
}

func (h *AdminHandler) Broadcast(ctx context.Context, text string) (int, error) {
	targets := make(map[string]bool)
	for _, num := range h.cfg.OwnerNumbers {
		targets[num] = true
	}
	for _, num := range h.cfg.AllowedNumbers {
		targets[num] = true
	}

	count := 0
	msg := &waProto.Message{
		Conversation: proto.String("📢 *Siaran Pesan*\n\n" + text),
	}

	for num := range targets {
		jid := types.NewJID(num, types.DefaultUserServer)
		if _, err := h.client.SendMessage(ctx, jid, msg); err == nil {
			count++
			time.Sleep(500 * time.Millisecond) // avoid spamming WhatsApp server
		}
	}

	return count, nil
}
