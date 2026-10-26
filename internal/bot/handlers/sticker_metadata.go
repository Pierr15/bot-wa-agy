package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"bot-wa/internal/sticker"
)

type StickerMetadata struct {
	PackName string `json:"pack_name"`
	Author   string `json:"author"`
}

var stickerMetadataMu sync.Mutex

func defaultStickerMetadata() StickerMetadata {
	return StickerMetadata{PackName: "✨ Stiker", Author: "StikerBot"}
}

func (h *StickerHandler) metadataPath() string {
	return filepath.Join(h.cfg.DataDir, "sticker-metadata.json")
}

func (h *StickerHandler) loadStickerDefaults() (StickerMetadata, error) {
	value := defaultStickerMetadata()
	data, err := os.ReadFile(h.metadataPath())
	if os.IsNotExist(err) {
		return value, nil
	}
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return defaultStickerMetadata(), fmt.Errorf("gagal membaca metadata default: %w", err)
	}
	if strings.TrimSpace(value.PackName) == "" || strings.TrimSpace(value.Author) == "" {
		return defaultStickerMetadata(), nil
	}
	return value, nil
}

// SetStickerMetadata sets a one-shot override for the next sticker sent in this chat.
func (h *StickerHandler) SetStickerMetadata(chat, raw string) error {
	value, err := parseStickerMetadata(raw)
	if err != nil {
		return err
	}
	h.metadataMu.Lock()
	defer h.metadataMu.Unlock()
	if h.pendingMetadata == nil {
		h.pendingMetadata = make(map[string]StickerMetadata)
	}
	h.pendingMetadata[chat] = value
	return nil
}

func (h *StickerHandler) SetDefaultStickerMetadata(raw string) error {
	value, err := parseStickerMetadata(raw)
	if err != nil {
		return err
	}
	stickerMetadataMu.Lock()
	defer stickerMetadataMu.Unlock()
	if err := os.MkdirAll(h.cfg.DataDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp := h.metadataPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, h.metadataPath()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (h *StickerHandler) StickerMetadataInfo(chat string) string {
	defaults, err := h.loadStickerDefaults()
	if err != nil {
		return "Gagal membaca metadata default: " + err.Error()
	}
	h.metadataMu.Lock()
	current, ok := h.pendingMetadata[chat]
	h.metadataMu.Unlock()
	if !ok {
		current = defaults
	}
	return fmt.Sprintf("🏷️ *Metadata Stiker*\n\nSaat ini untuk chat ini:\n• Paket: %s\n• Author: %s\n\nDefault tersimpan:\n• Paket: %s\n• Author: %s\n\nGunakan .wm Nama Paket|Author untuk stiker berikutnya.", current.PackName, current.Author, defaults.PackName, defaults.Author)
}

func parseStickerMetadata(raw string) (StickerMetadata, error) {
	parts := strings.SplitN(raw, "|", 2)
	if len(parts) != 2 {
		return StickerMetadata{}, fmt.Errorf("format: Nama Paket|Author\nContoh: ✨ Stiker|Javier")
	}
	value := StickerMetadata{PackName: strings.TrimSpace(parts[0]), Author: strings.TrimSpace(parts[1])}
	if value.PackName == "" || value.Author == "" {
		return StickerMetadata{}, fmt.Errorf("nama paket dan author tidak boleh kosong")
	}
	if len([]rune(value.PackName)) > 80 || len([]rune(value.Author)) > 80 {
		return StickerMetadata{}, fmt.Errorf("nama paket dan author maksimal 80 karakter")
	}
	return value, nil
}

func (h *StickerHandler) applyStickerMetadata(chat string, data []byte) ([]byte, StickerMetadata, error) {
	h.metadataMu.Lock()
	value, ok := h.pendingMetadata[chat]
	if ok {
		delete(h.pendingMetadata, chat)
	}
	h.metadataMu.Unlock()
	if !ok {
		var err error
		value, err = h.loadStickerDefaults()
		if err != nil {
			return nil, StickerMetadata{}, err
		}
	}
	embedded, err := sticker.EmbedWebPMetadata(data, value.PackName, value.Author)
	if err != nil {
		return nil, StickerMetadata{}, err
	}
	return embedded, value, nil
}
