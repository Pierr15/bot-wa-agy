package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"strconv"
	"sync"
	"time"

	"bot-wa/internal/bot/handlers"
	"bot-wa/internal/config"
	"bot-wa/internal/downloader"
	"bot-wa/internal/invite"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

type Router struct {
	cfg        *config.Config
	client     *whatsmeow.Client
	access     *AccessControl
	admin      *handlers.AdminHandler
	sticker    *handlers.StickerHandler
	downloader *handlers.DownloaderHandler
	invites *invite.Service

	processedMsg map[string]time.Time
	procMu       sync.Mutex
}

func NewRouter(cfg *config.Config, client *whatsmeow.Client, access *AccessControl, invites *invite.Service) *Router {
	r := &Router{
		cfg:          cfg,
		client:       client,
		access:       access,
		admin:        handlers.NewAdminHandler(cfg, client),
		sticker:      handlers.NewStickerHandler(cfg, client),
		downloader:   handlers.NewDownloaderHandler(cfg, client),
		invites:      invites,
		processedMsg: make(map[string]time.Time),
	}

	// Periodic cleanup of deduplication map
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			r.procMu.Lock()
			cutoff := time.Now().Add(-10 * time.Minute)
			for id, t := range r.processedMsg {
				if t.Before(cutoff) {
					delete(r.processedMsg, id)
				}
			}
			r.procMu.Unlock()
		}
	}()

	return r
}

func (r *Router) HandleMessage(evt *events.Message) {
	if evt.Info.IsFromMe {
		return
	}

	// Deduplication
	msgID := evt.Info.ID
	r.procMu.Lock()
	if _, seen := r.processedMsg[msgID]; seen {
		r.procMu.Unlock()
		return
	}
	r.processedMsg[msgID] = time.Now()
	r.procMu.Unlock()

	chat := evt.Info.Chat
	sender := evt.Info.Sender

	ctx := context.Background()
	msg := evt.Message
	if msg == nil {
		return
	}
	text := extractText(msg)
	cleanText := strings.TrimSpace(text)

	// Invitation redemption must be reachable by users not yet on the whitelist.
	if strings.HasPrefix(cleanText, ".") {
		cmd, args := parseCommand(cleanText)
		// Link-friendly form: .init_TOKEN, suitable for wa.me/?text=.init_TOKEN.
		if strings.HasPrefix(strings.ToLower(cleanText), ".init_") {
			cmd = "init"
			args = strings.TrimSpace(cleanText[len(".init_"):])
		}
		if cmd == "init" {
			if chat.Server != types.DefaultUserServer {
				r.reply(ctx, chat, "Pendaftaran token hanya bisa dilakukan lewat chat pribadi bot.")
				return
			}
			phone := r.access.ResolvePhone(sender)
			if phone == "" {
				r.reply(ctx, chat, "Nomor WhatsApp kamu belum bisa diverifikasi. Coba lagi lewat chat pribadi.")
				return
			}
			if r.access.IsOwner(sender) {
				r.reply(ctx, chat, "Akun owner sudah memiliki akses penuh.")
				return
			}
			allowed, _ := r.access.IsAllowed(sender, chat)
			if allowed {
				r.reply(ctx, chat, "Akses kamu sudah aktif. Token tidak digunakan.")
				return
			}
			if args == "" {
				r.reply(ctx, chat, "Format: .init TOKEN atau .init_TOKEN (untuk link WhatsApp)")
				return
			}
			if r.invites == nil {
				r.reply(ctx, chat, "Fitur pendaftaran sedang tidak tersedia.")
				return
			}
			if err := r.invites.Redeem(ctx, args, phone); err != nil {
				switch {
				case errors.Is(err, invite.ErrAlreadyRedeemed):
					r.reply(ctx, chat, "Nomor ini sudah pernah menggunakan token tersebut.")
				case errors.Is(err, invite.ErrAlreadyAllowed):
					r.reply(ctx, chat, "Nomor kamu sudah terdaftar di whitelist.")
				case errors.Is(err, invite.ErrInvalidToken):
					r.reply(ctx, chat, "Token tidak valid, kedaluwarsa, dicabut, atau kuotanya habis.")
				default:
					log.Error().Err(err).Msg("Gagal memproses token undangan")
					r.reply(ctx, chat, "Pendaftaran gagal karena masalah penyimpanan. Coba lagi nanti.")
				}
				return
			}
			r.access.AddAllowed(phone)
			r.reply(ctx, chat, "✅ Pendaftaran berhasil! Nomor kamu sudah masuk whitelist.")
			return
		}
	}

	// Whitelist & Access Check
	allowed, isOwner := r.access.IsAllowed(sender, chat)
	if !allowed {
		return
	}

	// 1. Direct Image Message with Caption
	if msg.ImageMessage != nil {
		if isStickerCmd(cleanText) {
			caption := extractCaptionArgs(cleanText)
			r.sticker.React(ctx, chat, sender, msgID, "⏳")
			if err := r.sticker.HandleImageSticker(ctx, evt, msg.ImageMessage, caption); err != nil {
				r.sticker.React(ctx, chat, sender, msgID, "❌")
				r.reply(ctx, chat, "Gagal membuat stiker: "+err.Error())
			} else {
				r.sticker.React(ctx, chat, sender, msgID, "✅")
			}
		} else {
			r.sticker.SendHint(ctx, chat)
		}
		return
	}

	// 2. Direct Video Message with Caption
	if msg.VideoMessage != nil {
		if isStickerCmd(cleanText) {
			r.sticker.React(ctx, chat, sender, msgID, "⏳")
			if err := r.sticker.HandleVideoSticker(ctx, evt, msg.VideoMessage); err != nil {
				r.sticker.React(ctx, chat, sender, msgID, "❌")
				r.reply(ctx, chat, "Gagal membuat stiker animasi: "+err.Error())
			} else {
				r.sticker.React(ctx, chat, sender, msgID, "✅")
			}
		} else {
			r.sticker.SendHint(ctx, chat)
		}
		return
	}

	// 3. Command Parsing (.command)
	if strings.HasPrefix(cleanText, ".") {
		cmd, args := parseCommand(cleanText)
		r.handleCommand(ctx, evt, cmd, args, isOwner)
		return
	}

	// 4. Downloader Links
	links := downloader.DetectLinks(cleanText)
	if len(links) > 0 {
		go func() {
			if err := r.downloader.Handle(ctx, evt, links[0]); err != nil {
				r.reply(ctx, chat, "Downloader error: "+err.Error())
			}
		}()
		return
	}

	// 5. Quoted media check
	quoted := getQuotedMessage(msg)
	if quoted != nil && (quoted.ImageMessage != nil || quoted.VideoMessage != nil) {
		r.sticker.SendHint(ctx, chat)
	}
}

func (r *Router) handleCommand(ctx context.Context, evt *events.Message, cmd, args string, isOwner bool) {
	chat := evt.Info.Chat
	sender := evt.Info.Sender
	msgID := evt.Info.ID

	switch cmd {
	case "token":
		if !isOwner {
			r.reply(ctx, chat, "Perintah ini khusus owner.")
			return
		}
		if r.invites == nil {
			r.reply(ctx, chat, "Database token tidak tersedia.")
			return
		}
		switch {
		case args == "" || args == "help":
			r.reply(ctx, chat, "🔐 *Manajemen Token*\n.token create [max_use] [durasi] - default 5 pendaftaran, 24 jam\nContoh: .token create 4 30h\n.token list - lihat token\n.token revoke ID - cabut token")
		case strings.HasPrefix(args, "create"):
			fields := strings.Fields(args)
			if len(fields) == 0 || fields[0] != "create" || len(fields) > 3 {
				r.reply(ctx, chat, "Format: .token create [max_use] [durasi]\nContoh: .token create 4 30h")
				return
			}
			maxUses := 5
			ttl := 24 * time.Hour
			var parseErr error
			if len(fields) >= 2 {
				maxUses, parseErr = strconv.Atoi(fields[1])
				if parseErr != nil || maxUses < 1 || maxUses > 10000 {
					r.reply(ctx, chat, "Max use harus berupa angka 1–10000.")
					return
				}
			}
			if len(fields) == 3 {
				ttl, parseErr = time.ParseDuration(fields[2])
				if parseErr != nil || ttl < time.Minute || ttl > 8760*time.Hour {
					r.reply(ctx, chat, "Durasi tidak valid. Gunakan format seperti 30h, minimal 1m dan maksimal 8760h.")
					return
				}
			}
			token, t, err := r.invites.Create(ctx, maxUses, ttl)
			if err != nil {
				r.reply(ctx, chat, "Gagal membuat token.")
				log.Error().Err(err).Msg("Gagal membuat token undangan")
				return
			}
			r.reply(ctx, chat, fmt.Sprintf("🔐 *Token Undangan*\nToken: %s\nLink command: .init_%s\nID: %d\nKuota: %d nomor\nKedaluwarsa: %s UTC", token, token, t.ID, t.MaxUses, t.ExpiresAt.Format("2006-01-02 15:04:05")))
		case args == "list":
			tokens, err := r.invites.List(ctx)
			if err != nil {
				r.reply(ctx, chat, "Gagal membaca daftar token.")
				return
			}
			if len(tokens) == 0 {
				r.reply(ctx, chat, "Belum ada token undangan.")
				return
			}
			var sb strings.Builder
			sb.WriteString("🔐 *Daftar Token*\n")
			for _, t := range tokens {
				status := "aktif"
				if t.Revoked { status = "dicabut" } else if time.Now().UTC().After(t.ExpiresAt) { status = "kedaluwarsa" } else if t.Uses >= t.MaxUses { status = "habis" }
				fmt.Fprintf(&sb, "#%d · %s · %d/%d · sampai %s UTC\n", t.ID, status, t.Uses, t.MaxUses, t.ExpiresAt.Format("2006-01-02 15:04"))
			}
			r.reply(ctx, chat, sb.String())
		case strings.HasPrefix(args, "revoke "):
			id, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(args, "revoke ")), 10, 64)
			if err != nil || id < 1 {
				r.reply(ctx, chat, "Format: .token revoke ID")
				return
			}
			ok, err := r.invites.Revoke(ctx, id)
			if err != nil { r.reply(ctx, chat, "Gagal mencabut token."); return }
			if !ok { r.reply(ctx, chat, "ID token tidak ditemukan."); return }
			r.reply(ctx, chat, fmt.Sprintf("✅ Token #%d berhasil dicabut.", id))
		default:
			r.reply(ctx, chat, "Format: .token create | .token list | .token revoke ID")
		}

	case "s", "stiker", "sticker":
		quoted := getQuotedMessage(evt.Message)
		if quoted != nil && quoted.ImageMessage != nil {
			r.sticker.React(ctx, chat, sender, msgID, "⏳")
			if err := r.sticker.HandleImageSticker(ctx, evt, quoted.ImageMessage, args); err != nil {
				r.sticker.React(ctx, chat, sender, msgID, "❌")
				r.reply(ctx, chat, "Gagal membuat stiker: "+err.Error())
			} else {
				r.sticker.React(ctx, chat, sender, msgID, "✅")
			}
			return
		}
		if quoted != nil && quoted.VideoMessage != nil {
			r.sticker.React(ctx, chat, sender, msgID, "⏳")
			if err := r.sticker.HandleVideoSticker(ctx, evt, quoted.VideoMessage); err != nil {
				r.sticker.React(ctx, chat, sender, msgID, "❌")
				r.reply(ctx, chat, "Gagal membuat stiker animasi: "+err.Error())
			} else {
				r.sticker.React(ctx, chat, sender, msgID, "✅")
			}
			return
		}
		r.reply(ctx, chat, "Kirim foto/video dengan caption `.s` atau reply foto/video Anda dengan `.s`.")

	case "t", "teks":
		r.sticker.React(ctx, chat, sender, msgID, "⏳")
		if err := r.sticker.HandleTextSticker(ctx, evt, args); err != nil {
			r.sticker.React(ctx, chat, sender, msgID, "❌")
			r.reply(ctx, chat, err.Error())
		} else {
			r.sticker.React(ctx, chat, sender, msgID, "✅")
		}

	case "v":
		go func() {
			if err := r.downloader.HandleChoice(ctx, evt, "video", args); err != nil {
				r.reply(ctx, chat, err.Error())
			}
		}()

	case "a":
		go func() {
			if err := r.downloader.HandleChoice(ctx, evt, "audio", args); err != nil {
				r.reply(ctx, chat, err.Error())
			}
		}()

	case "f", "foto":
		go func() {
			if err := r.downloader.HandleChoice(ctx, evt, "image", args); err != nil {
				r.reply(ctx, chat, err.Error())
			}
		}()

	case "menu", "help", "start":
		r.reply(ctx, chat, r.admin.BuildMenu(isOwner))

	case "ping":
		r.reply(ctx, chat, "🏓 pong!")

	case "id":
		if !isOwner {
			r.reply(ctx, chat, "Perintah ini khusus owner.")
			return
		}
		r.reply(ctx, chat, fmt.Sprintf("Chat JID: `%s`\nSender: `%s`", chat.String(), sender.String()))

	case "stat", "status":
		if !isOwner {
			r.reply(ctx, chat, "Perintah ini khusus owner.")
			return
		}
		r.reply(ctx, chat, r.admin.Stat())

	case "broadcast", "bc":
		if !isOwner {
			r.reply(ctx, chat, "Perintah ini khusus owner.")
			return
		}
		broadcastText, ok := maintenanceBroadcast(args)
		if !ok {
			r.reply(ctx, chat, "Format: .bc <pesan>\nTemplate: .bc upcoming [detail] | .bc maintenance [detail] | .bc ends [detail]")
			return
		}
		go func() {
			n, _ := r.admin.Broadcast(ctx, broadcastText)
			r.reply(ctx, chat, fmt.Sprintf("Pesan telah disiarkan ke %d nomor.", n))
		}()

	default:
		r.reply(ctx, chat, fmt.Sprintf("Perintah `.%s` tidak dikenal. Ketik `.menu` untuk melihat daftar fitur.", cmd))
	}
}


func maintenanceBroadcast(args string) (string, bool) {
	args = strings.TrimSpace(args)
	fields := strings.Fields(args)
	if len(fields) == 0 { return "", false }
	kind := strings.ToLower(fields[0])
	detail := strings.TrimSpace(strings.TrimPrefix(args, fields[0]))
	var message string
	switch kind {
	case "upcoming", "upcoming-maintenance":
		message = "📢 *PENGUMUMAN MAINTENANCE*\n\nBot akan menjalani maintenance dalam waktu dekat. Selama proses persiapan, layanan mungkin mengalami gangguan. Terima kasih atas pengertiannya."
	case "maintenance", "start":
		message = "🛠️ *MAINTENANCE BERLANGSUNG*\n\nBot sedang dalam proses maintenance. Beberapa fitur mungkin tidak dapat digunakan sementara waktu. Kami akan mengabari kembali setelah layanan pulih."
	case "ends", "ended", "selesai":
		message = "✅ *MAINTENANCE SELESAI*\n\nMaintenance telah selesai dan layanan bot kembali tersedia. Silakan gunakan bot seperti biasa. Terima kasih sudah menunggu!"
	default:
		return args, true
	}
	if detail != "" { message += "\n\n📝 " + detail }
	return message, true
}

func (r *Router) reply(ctx context.Context, chat types.JID, text string) {
	_, err := r.client.SendMessage(ctx, chat, &waProto.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		log.Warn().Str("err", err.Error()).Msg("Gagal mengirim balasan teks")
	}
}

func isStickerCmd(caption string) bool {
	if !strings.HasPrefix(caption, ".") {
		return false
	}
	cmd, _ := parseCommand(caption)
	return cmd == "s" || cmd == "stiker" || cmd == "sticker"
}

func extractCaptionArgs(caption string) string {
	_, args := parseCommand(caption)
	return args
}

func parseCommand(raw string) (string, string) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(raw), ".")
	parts := strings.SplitN(trimmed, " ", 2)
	cmd := strings.ToLower(parts[0])
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}
	return cmd, args
}

func extractText(msg *waProto.Message) string {
	if msg.Conversation != nil {
		return *msg.Conversation
	}
	if msg.ExtendedTextMessage != nil && msg.ExtendedTextMessage.Text != nil {
		return *msg.ExtendedTextMessage.Text
	}
	if msg.ImageMessage != nil && msg.ImageMessage.Caption != nil {
		return *msg.ImageMessage.Caption
	}
	if msg.VideoMessage != nil && msg.VideoMessage.Caption != nil {
		return *msg.VideoMessage.Caption
	}
	return ""
}

func getQuotedMessage(msg *waProto.Message) *waProto.Message {
	if msg.ExtendedTextMessage != nil && msg.ExtendedTextMessage.ContextInfo != nil {
		return msg.ExtendedTextMessage.ContextInfo.QuotedMessage
	}
	if msg.ImageMessage != nil && msg.ImageMessage.ContextInfo != nil {
		return msg.ImageMessage.ContextInfo.QuotedMessage
	}
	if msg.VideoMessage != nil && msg.VideoMessage.ContextInfo != nil {
		return msg.VideoMessage.ContextInfo.QuotedMessage
	}
	return nil
}
