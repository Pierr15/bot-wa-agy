package bot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"bot-wa/internal/bot/handlers"
	"bot-wa/internal/config"
	"bot-wa/internal/downloader"

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

	processedMsg map[string]time.Time
	procMu       sync.Mutex
}

func NewRouter(cfg *config.Config, client *whatsmeow.Client, access *AccessControl) *Router {
	r := &Router{
		cfg:          cfg,
		client:       client,
		access:       access,
		admin:        handlers.NewAdminHandler(cfg, client),
		sticker:      handlers.NewStickerHandler(cfg, client),
		downloader:   handlers.NewDownloaderHandler(cfg, client),
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

	// Whitelist & Access Check
	allowed, isOwner := r.access.IsAllowed(sender, chat)
	if !allowed {
		return
	}

	ctx := context.Background()
	msg := evt.Message
	if msg == nil {
		return
	}

	text := extractText(msg)
	cleanText := strings.TrimSpace(text)

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
		if args == "" {
			r.reply(ctx, chat, "Format: `.bc <pesan>`")
			return
		}
		go func() {
			n, _ := r.admin.Broadcast(ctx, args)
			r.reply(ctx, chat, fmt.Sprintf("Pesan telah disiarkan ke %d nomor.", n))
		}()

	default:
		r.reply(ctx, chat, fmt.Sprintf("Perintah `.%s` tidak dikenal. Ketik `.menu` untuk melihat daftar fitur.", cmd))
	}
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
