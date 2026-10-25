package main

import (
	"context"
	"os"
	"os/signal"
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

	// 5. Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	sig := <-sigChan
	log.Info().Str("signal", sig.String()).Msg("Menerima sinyal shutdown, menghentikan bot secara rapi...")
	client.Stop()
	log.Info().Msg("Bot berhasil dihentikan")
}
