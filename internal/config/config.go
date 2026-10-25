package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	// General
	BotName   string
	LogLevel  string
	Timezone  string
	LoginMode string // "qr" or "pairing"
	BotPhone  string // required if LoginMode == "pairing"

	// Whitelist & Access
	OwnerNumbers   []string
	AllowedNumbers []string
	AllowedGroups  []string

	// Paths
	DataDir      string
	TmpDir       string
	AuthDB       string
	CookiesDir   string
	FontPath     string
	IGCookiePath string

	// External Binaries
	YtDlpPath   string
	FfmpegPath  string
	FfprobePath string

	// Sticker Settings
	StickerDefaultPos string
	StickerMaxChars   int
	StickerAnimMaxSec float64
	StickerAnimMaxKB  int64
	MaxImageBytes     int64

	// Downloader Settings
	DownloadMaxBytes       int64
	DownloadSourceMaxBytes int64
	DownloadMaxDurationSec int
	DownloadConcurrency    int
	DownloadTimeoutSec     int
	MinFreeDiskBytes       uint64
	MaxQueue               int
	EnableTikwmFallback    bool
}

func LoadConfig() (*Config, error) {
	_ = godotenv.Load()

	dataDir := getEnv("DATA_DIR", "data")
	tmpDir := getEnv("TMP_DIR", filepath.Join(dataDir, "tmp"))
	authDB := getEnv("AUTH_DB", filepath.Join(dataDir, "auth", "whatsapp.db"))
	cookiesDir := getEnv("COOKIES_DIR", filepath.Join(dataDir, "cookies"))
	igCookiePath := getEnv("IG_COOKIES_PATH", filepath.Join(cookiesDir, "instagram.txt"))

	maxImageMB := getEnvInt("MAX_IMAGE_MB", 10)
	dlMaxMB := getEnvInt("DOWNLOAD_MAX_MB", 16)
	dlSourceMaxMB := getEnvInt("DOWNLOAD_SOURCE_MAX_MB", 100)
	minFreeDiskMB := getEnvInt("MIN_FREE_DISK_MB", 1024)

	cfg := &Config{
		BotName:   getEnv("BOT_NAME", "WaBot"),
		LogLevel:  getEnv("LOG_LEVEL", "info"),
		Timezone:  getEnv("TZ", "Asia/Jakarta"),
		LoginMode: strings.ToLower(getEnv("LOGIN_MODE", "qr")),
		BotPhone:  cleanPhone(getEnv("BOT_PHONE_NUMBER", "")),

		OwnerNumbers:   parseNumberList(os.Getenv("OWNER_NUMBERS")),
		AllowedNumbers: parseNumberList(os.Getenv("ALLOWED_NUMBERS")),
		AllowedGroups:  parseNumberList(os.Getenv("ALLOWED_GROUPS")),

		DataDir:      dataDir,
		TmpDir:       tmpDir,
		AuthDB:       authDB,
		CookiesDir:   cookiesDir,
		FontPath:     getEnv("STICKER_FONT_PATH", "assets/fonts/Anton-Regular.ttf"),
		IGCookiePath: igCookiePath,

		YtDlpPath:   getEnv("YTDLP_PATH", "yt-dlp"),
		FfmpegPath:  getEnv("FFMPEG_PATH", "ffmpeg"),
		FfprobePath: getEnv("FFPROBE_PATH", "ffprobe"),

		StickerDefaultPos: strings.ToLower(getEnv("STICKER_DEFAULT_POSITION", "bottom")),
		StickerMaxChars:   getEnvInt("STICKER_MAX_CHARS", 140),
		StickerAnimMaxSec: getEnvFloat("STICKER_ANIM_MAX_SEC", 6.0),
		StickerAnimMaxKB:  int64(getEnvInt("STICKER_ANIM_MAX_KB", 300)),
		MaxImageBytes:     int64(maxImageMB) * 1024 * 1024,

		DownloadMaxBytes:       int64(dlMaxMB) * 1024 * 1024,
		DownloadSourceMaxBytes: int64(dlSourceMaxMB) * 1024 * 1024,
		DownloadMaxDurationSec: getEnvInt("DOWNLOAD_MAX_DURATION_SEC", 300),
		DownloadConcurrency:    getEnvInt("DOWNLOAD_CONCURRENCY", 1),
		DownloadTimeoutSec:     getEnvInt("DOWNLOAD_TIMEOUT_SEC", 90),
		MinFreeDiskBytes:       uint64(minFreeDiskMB) * 1024 * 1024,
		MaxQueue:               getEnvInt("MAX_QUEUE", 20),
		EnableTikwmFallback:    getEnvBool("ENABLE_TIKWM_FALLBACK", true),
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if val, ok := os.LookupEnv(key); ok {
		if f, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil {
			return f
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		s := strings.ToLower(strings.TrimSpace(val))
		return s == "1" || s == "true" || s == "yes"
	}
	return fallback
}

func cleanPhone(raw string) string {
	var digits strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	s := digits.String()
	if strings.HasPrefix(s, "0") {
		s = "62" + s[1:]
	}
	return s
}

func parseNumberList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		c := cleanPhone(p)
		if c != "" {
			res = append(res, c)
		}
	}
	return res
}
