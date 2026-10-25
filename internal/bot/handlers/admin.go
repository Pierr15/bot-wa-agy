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

	sb.WriteString("🤖 *" + h.cfg.BotName + "*\n")
	if isOwner {
		sb.WriteString("👑 _Owner Control Center_\n")
	} else {
		sb.WriteString("✨ _Menu Fitur_\n")
	}
	sb.WriteString("━━━━━━━━━━━━━━━━━━\n\n")

	sb.WriteString("🎨 *STICKER STUDIO*\n")
	sb.WriteString("› Kirim foto + caption `.s` → stiker polos\n")
	sb.WriteString("› `.s TEKS` → teks bawah\n")
	sb.WriteString("› `.s | TEKS` → teks atas\n")
	sb.WriteString("› `.s BAWAH | ATAS` → teks dua zona\n")
	sb.WriteString("› Video/GIF + caption `.s` → stiker animasi (maks. " + fmt.Sprintf("%.0f", h.cfg.StickerAnimMaxSec) + " detik)\n")
	sb.WriteString("› `.t TEKS` → stiker teks berlatar putih\n")
	sb.WriteString("💡 _Bisa juga reply foto/video dengan `.s`._\n\n")

	sb.WriteString("📥 *MEDIA DOWNLOADER*\n")
	sb.WriteString("› Kirim link TikTok / Instagram (post, reel, atau carousel)\n")
	sb.WriteString("› Reply hasil download untuk memilih media:\n")
	sb.WriteString("  • `.v` → kirim video\n")
	sb.WriteString("  • `.a` → ekstrak audio (.m4a)\n")
	sb.WriteString("  • `.f` → kirim semua foto\n")
	sb.WriteString("  • `.f N` → kirim foto ke-N\n\n")

	sb.WriteString("⚡ *QUICK TOOLS*\n")
	sb.WriteString("› `.ping` → cek respons bot\n")
	sb.WriteString("› `.menu` → tampilkan menu ini\n")

	if isOwner {
		sb.WriteString("\n━━━━━━━━━━━━━━━━━━\n")
		sb.WriteString("🛡️ *OWNER TOOLS*\n")
		sb.WriteString("› `.stat` → uptime, RAM, disk, dan info sistem\n")
		sb.WriteString("› `.id` → detail JID chat dan pengirim\n")
		sb.WriteString("› `.bc <pesan>` → siarkan pesan custom\n")
		sb.WriteString("› `.bc upcoming` / `.bc maintenance` / `.bc ends` → template maintenance\n")
		sb.WriteString("› `.token create [max_use] [durasi]` → token undangan\n")
		sb.WriteString("  Contoh: `.token create 4 30h`\n")
		sb.WriteString("› `.token list` / `.token revoke ID` → kelola token\n")
		sb.WriteString("\n🔐 _Akses owner aktif_\n")
	} else {
		sb.WriteString("\n✨ _Simple • Fast • Easy to use_\n")
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
