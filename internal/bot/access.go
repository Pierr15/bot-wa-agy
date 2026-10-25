package bot

import (
	"bot-wa/internal/config"
	"sync"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow/types"
)

type AccessControl struct {
	cfg        *config.Config
	owners     map[string]bool
	allowed    map[string]bool
	groups     map[string]bool
	lidToPhone map[string]string
	mu         sync.RWMutex
}

func NewAccessControl(cfg *config.Config) *AccessControl {
	owners := make(map[string]bool)
	for _, num := range cfg.OwnerNumbers {
		owners[num] = true
	}

	allowed := make(map[string]bool)
	for _, num := range cfg.AllowedNumbers {
		allowed[num] = true
	}

	groups := make(map[string]bool)
	for _, g := range cfg.AllowedGroups {
		groups[g] = true
	}

	return &AccessControl{
		cfg:        cfg,
		owners:     owners,
		allowed:    allowed,
		groups:     groups,
		lidToPhone: make(map[string]string),
	}
}

// RegisterLID maps a WhatsApp LID to a verified phone number.
func (ac *AccessControl) RegisterLID(lid, phone string) {
	if lid == "" || phone == "" {
		return
	}
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.lidToPhone[lid] = phone
}

func (ac *AccessControl) ResolvePhone(jid types.JID) string {
	if jid.Server == types.DefaultUserServer {
		return CleanPhoneNumber(jid.User)
	}

	ac.mu.RLock()
	defer ac.mu.RUnlock()
	if phone, ok := ac.lidToPhone[jid.User]; ok {
		return phone
	}
	return ""
}

func (ac *AccessControl) IsOwner(sender types.JID) bool {
	phone := ac.ResolvePhone(sender)
	if phone == "" {
		return false
	}
	return ac.owners[phone]
}

func (ac *AccessControl) IsAllowed(sender, chat types.JID) (bool, bool) {
	// Group check
	if chat.Server == types.GroupServer {
		if !ac.groups[chat.User] {
			log.Debug().Str("group", chat.String()).Msg("Pesan grup diabaikan (belum masuk whitelist)")
			return false, false
		}
	}

	// If no whitelist is specified, deny all for security
	if len(ac.owners) == 0 && len(ac.allowed) == 0 {
		return false, false
	}

	phone := ac.ResolvePhone(sender)
	if phone == "" {
		// If it's a direct user server, phone is directly sender.User
		if sender.Server == types.DefaultUserServer {
			phone = CleanPhoneNumber(sender.User)
		}
	}

	if phone == "" {
		log.Warn().Str("sender", MaskJID(sender)).Msg("Tidak dapat mengidentifikasi nomor pengirim; pesan diabaikan")
		return false, false
	}

	isOwner := ac.owners[phone]
	if isOwner || ac.allowed[phone] {
		return true, isOwner
	}

	return false, false
}
