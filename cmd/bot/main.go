package main

import (
	"context"
	"os"
	"os/signal"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"bot-wa/internal/bot"
	"bot-wa/internal/config"
	"bot-wa/internal/sticker"
	"bot-wa/internal/storage"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// Pretty console output for logging
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Gagal membaca konfigurasi")
	}

	// Parse LOG_LEVEL instead of treating every non-debug value as info.
	levelName := strings.ToLower(strings.TrimSpace(cfg.LogLevel))
	if levelName == "warning" {
		levelName = "warn"
	}
	level, err := zerolog.ParseLevel(levelName)
	if err != nil {
		log.Warn().Str("level", cfg.LogLevel).Msg("LOG_LEVEL tidak dikenal; menggunakan info")
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	log.Info().
		Str("bot", cfg.BotName).
		Str("loginMode", cfg.LoginMode).
		Str("logLevel", levelName).
		Int("owners", len(cfg.OwnerNumbers)).
		Int("allowed", len(cfg.AllowedNumbers)).
		Msg("Memulai bot WhatsApp (Go Backend)")

	// 1. Ensure storage folders exist
	if err := storage.EnsureDirectories(cfg.DataDir, cfg.TmpDir, cfg.CookiesDir, "data/auth"); err != nil {
		log.Fatal().Err(err).Msg("Gagal membuat direktori data")
	}

	// 2. Verify Anton font
	if !sticker.EnsureAntonFont(cfg.FontPath) {
		log.Warn().Str("path", cfg.FontPath).Msg("Font Anton-Regular.ttf tidak ditemukan di path tersebut; pastikan file font ada di assets/fonts/")
	}

	// 3. Start background temp janitor
	stopJanitor := make(chan struct{})
	defer close(stopJanitor)
	storage.StartJanitor(cfg.TmpDir, 15*time.Minute, 6*time.Hour, stopJanitor)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 4. Initialize WhatsApp Bot Client
	client, err := bot.NewBotClient(ctx, cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Gagal menginisialisasi client bot")
	}

	if err := client.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("Gagal menyambungkan client WhatsApp")
	}

	// Watch main for updates only after WhatsApp is connected.
	go watchForUpdates(ctx, client, cfg.DataDir)

	// 5. Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	sig := <-sigChan
	log.Info().Str("signal", sig.String()).Msg("Menerima sinyal shutdown, menghentikan bot secara rapi...")

	// Broadcast before manual Ctrl+C or a manual runit stop. Auto-update has
	// already announced maintenance and leaves a marker, so avoid duplicate notices.
	marker := filepath.Join(cfg.DataDir, "autoupdate-pending")
	_, markerErr := os.Stat(marker)
	manualShutdown := sig == os.Interrupt || (sig == syscall.SIGTERM && os.IsNotExist(markerErr))
	if manualShutdown {
		if client.IsConnected() {
			bctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			sent, err := client.Broadcast(bctx, "🛑 *BOT AKAN DIHENTIKAN MANUAL*\n\nOwner menghentikan service untuk maintenance. Bot akan offline sementara dan tidak bisa memproses pesan sampai dijalankan kembali. Kami akan mengabari setelah layanan pulih.")
			cancel()
			if err != nil || sent == 0 {
				log.Warn().Err(err).Int("sent", sent).Msg("Broadcast shutdown tidak terkirim sepenuhnya")
			} else {
				log.Info().Int("sent", sent).Msg("Broadcast shutdown terkirim")
			}
		} else {
			log.Warn().Msg("WhatsApp belum tersambung; broadcast shutdown dilewati")
		}
	}

	client.Stop()
	log.Info().Msg("Bot berhasil dihentikan")
}

func watchForUpdates(ctx context.Context, client *bot.BotClient, dataDir string) {
	select {
	case <-client.Connected():
	case <-ctx.Done():
		return
	}
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		appDir, err := os.Getwd()
		if err != nil { log.Warn().Err(err).Msg("Auto-update: gagal membaca direktori aplikasi"); continue }
		fetch := exec.CommandContext(ctx, "git", "-C", appDir, "fetch", "origin", "main")
		if output, err := fetch.CombinedOutput(); err != nil {
			log.Warn().Err(err).Str("output", strings.TrimSpace(string(output))).Msg("Auto-update: gagal memeriksa origin/main"); continue
		}
		local := exec.CommandContext(ctx, "git", "-C", appDir, "rev-parse", "HEAD")
		localOut, err := local.Output()
		if err != nil { log.Warn().Err(err).Msg("Auto-update: gagal membaca commit lokal"); continue }
		remote := exec.CommandContext(ctx, "git", "-C", appDir, "rev-parse", "origin/main")
		remoteOut, err := remote.Output()
		if err != nil { log.Warn().Err(err).Msg("Auto-update: gagal membaca commit remote"); continue }
		if strings.TrimSpace(string(localOut)) == strings.TrimSpace(string(remoteOut)) { continue }

		log.Info().Msg("Auto-update: commit baru terdeteksi; mengumumkan maintenance")
		bctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		sent, err := client.Broadcast(bctx, "🛠️ *MAINTENANCE BERLANGSUNG*\n\nUpdate otomatis sedang dipasang. Bot akan tersambung kembali setelah proses selesai.")
		cancel()
		if err != nil || sent == 0 {
			log.Warn().Err(err).Int("sent", sent).Msg("Auto-update dibatalkan: pengumuman maintenance tidak terkirim")
			continue
		}

		marker := filepath.Join(dataDir, "autoupdate-pending")
		cmd := exec.CommandContext(ctx, "bash", filepath.Join(appDir, "scripts", "termux", "update.sh"))
		cmd.Dir = appDir
		cmd.Env = append(os.Environ(), "APP_DIR="+appDir, "BOT_AUTO_UPDATE=1", "UPDATE_MARKER="+marker)
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Error().Err(err).Str("output", strings.TrimSpace(string(output))).Msg("Auto-update gagal; versi aktif dipertahankan")
			bctx, cancel = context.WithTimeout(context.Background(), 45*time.Second)
			_, _ = client.Broadcast(bctx, "⚠️ *UPDATE DITUNDA*\n\nUpdate otomatis gagal atau dibatalkan. Bot tetap berjalan dengan versi sebelumnya.")
			cancel()
			continue
		}
		log.Info().Str("output", strings.TrimSpace(string(output))).Msg("Auto-update berhasil; meminta runit memulai ulang bot")
		if p, err := os.FindProcess(os.Getpid()); err == nil { _ = p.Signal(syscall.SIGTERM) }
		return
	}
}
