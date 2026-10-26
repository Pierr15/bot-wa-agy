package bot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bot-wa/internal/config"
	"bot-wa/internal/invite"

	"github.com/mdp/qrterminal/v3"
	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	_ "modernc.org/sqlite"
)

type BotClient struct {
	cfg    *config.Config
	client *whatsmeow.Client
	router *Router
	access *AccessControl
	invites *invite.Service
	connected chan struct{}
	connectedOnce sync.Once
}

func whatsmeowLogLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug", "trace":
		return "DEBUG"
	case "warn", "warning":
		return "WARN"
	case "error", "fatal", "panic":
		return "ERROR"
	default:
		return "INFO"
	}
}

func NewBotClient(ctx context.Context, cfg *config.Config) (*BotClient, error) {
	waLevel := whatsmeowLogLevel(cfg.LogLevel)
	dbLog := waLog.Stdout("Database", waLevel, true)
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", cfg.AuthDB)

	container, err := sqlstore.New(ctx, "sqlite", dsn, dbLog)
	if err != nil {
		return nil, fmt.Errorf("gagal menginisialisasi SQLite auth store: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("gagal memuat device auth: %w", err)
	}

	clientLog := waLog.Stdout("WhatsApp", waLevel, true)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	// Retry transient network failures even when the first connection attempt fails.
	// whatsmeow already handles reconnects after unexpected disconnects by default.
	client.EnableAutoReconnect = true
	client.InitialAutoReconnect = true

	access := NewAccessControl(cfg, client)
	invites, err := invite.Open(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("gagal menginisialisasi database undangan: %w", err)
	}
	allowedNumbers, err := invites.AllowedNumbers(ctx)
	if err != nil {
		_ = invites.Close()
		return nil, fmt.Errorf("gagal memuat whitelist persisten: %w", err)
	}
	for _, number := range allowedNumbers {
		access.AddAllowed(number)
	}
	router := NewRouter(cfg, client, access, invites)

	bot := &BotClient{
		cfg:    cfg,
		client: client,
		router: router,
		access: access,
		invites: invites,
		connected: make(chan struct{}),
	}

	client.AddEventHandler(bot.EventHandler)
	return bot, nil
}

func (b *BotClient) Start(ctx context.Context) error {
	if b.client.Store.ID == nil {
		// Device not logged in yet
		if b.cfg.LoginMode == "pairing" {
			if b.cfg.BotPhone == "" {
				return fmt.Errorf("LOGIN_MODE=pairing tetapi BOT_PHONE_NUMBER belum diisi di .env")
			}

			go func() {
				time.Sleep(3 * time.Second)
				log.Info().Str("phone", b.cfg.BotPhone).Msg("Meminta kode pairing WhatsApp...")
				code, err := b.client.PairPhone(context.Background(), b.cfg.BotPhone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
				if err != nil {
					log.Error().Err(err).Msg("Gagal membuat kode pairing WhatsApp")
					return
				}

				formatted := code
				if len(code) == 8 {
					formatted = fmt.Sprintf("%s-%s", code[:4], code[4:])
				}

				fmt.Printf("\n========================================\n")
				fmt.Printf(" KODE PAIRING WHATSAPP: %s\n", formatted)
				fmt.Printf("========================================\n")
				fmt.Printf(" Buka WhatsApp di HP nomor bot:\n")
				fmt.Printf(" Perangkat Tertaut > Tautkan Perangkat > Tautkan dengan nomor telepon.\n\n")
			}()
		} else {
			// QR Code Mode
			qrChan, _ := b.client.GetQRChannel(ctx)
			go func() {
				qrFile := filepath.Join(b.cfg.DataDir, "qr.txt")
				for evt := range qrChan {
					if evt.Event == "code" {
						fmt.Println("\n--- SCAN QR CODE DI BAWAH INI DENGAN WHATSAPP ---")
						qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
						fmt.Println("-------------------------------------------------")
						_ = os.WriteFile(qrFile, []byte(evt.Code), 0644)
					} else {
						_ = os.Remove(qrFile)
						log.Info().Str("event", evt.Event).Msg("Status QR WhatsApp diperbarui")
					}
				}
			}()
		}
	}

	return b.client.Connect()
}

func (b *BotClient) Connected() <-chan struct{} { return b.connected }

func (b *BotClient) IsConnected() bool {
	return b.client != nil && b.client.IsConnected()
}

func (b *BotClient) Broadcast(ctx context.Context, text string) (int, error) {
	return b.router.Broadcast(ctx, text)
}

func (b *BotClient) Stop() {
	b.client.Disconnect()
	if b.invites != nil {
		_ = b.invites.Close()
	}
}

func (b *BotClient) EventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Connected:
		log.Info().Msg("WhatsApp terhubung!")
		b.connectedOnce.Do(func() { close(b.connected) })
		_ = os.Remove(filepath.Join(b.cfg.DataDir, "qr.txt"))
		marker := filepath.Join(b.cfg.DataDir, "autoupdate-pending")
		if _, err := os.Stat(marker); err == nil {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				defer cancel()
				sent, err := b.Broadcast(ctx, "✅ *MAINTENANCE SELESAI*\n\nUpdate berhasil dipasang dan bot sudah tersambung kembali. Silakan gunakan bot seperti biasa.")
				if err != nil || sent == 0 {
					log.Warn().Err(err).Int("sent", sent).Msg("Pengumuman maintenance selesai belum terkirim; marker dipertahankan")
					return
				}
				if err := os.Remove(marker); err != nil { log.Warn().Err(err).Msg("Gagal menghapus marker maintenance") }
			}()
		}

	case *events.Disconnected:
		log.Warn().Msg("Koneksi WhatsApp terputus; whatsmeow akan mencoba menyambungkan kembali jika memungkinkan.")

	case *events.LoggedOut:
		log.Error().Str("reason", v.Reason.String()).Msg("Sesi WhatsApp logout/dikeluarkan! Hapus data/auth/whatsapp.db untuk login baru.")
		// Exit process with non-zero so service manager (runit) knows it died
		os.Exit(1)

	case *events.Message:
		b.router.HandleMessage(v)
	}
}
