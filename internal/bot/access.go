package bot

import (
	"context"
	"sync"

	"bot-wa/internal/config"

	"github.com/rs/zerolog/log"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type AccessControl struct {
	cfg        *config.Config
	client     *whatsmeow.Client
	owners     map[string]bool
	allowed    map[string]bool
	groups     map[string]bool
	lidToPhone map[string]string
	mu         sync.RWMutex
}

func NewAccessControl(cfg *config.Config, client *whatsmeow.Client) *AccessControl {
	owners := make(map[string]bool)
	for _, num := range cfg.OwnerNumbers {
		owners[CleanPhoneNumber(num)] = true
	}

	allowed := make(map[string]bool)
	for _, num := range cfg.AllowedNumbers {
		allowed[CleanPhoneNumber(num)] = true
	}

	groups := make(map[string]bool)
	for _, g := range cfg.AllowedGroups {
		groups[g] = true
	}

	return &AccessControl{
		cfg:        cfg,
		client:     client,
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
	phone = CleanPhoneNumber(phone)
	if phone == "" {
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
	if jid.Server != types.HiddenUserServer {
		return ""
	}

	// Prefer a mapping already learned by this process.
	ac.mu.RLock()
	phone, ok := ac.lidToPhone[jid.User]
	ac.mu.RUnlock()
	if ok && phone != "" {
		return CleanPhoneNumber(phone)
	}

	// whatsmeow persists LID <-> phone JID mappings in the device store.
	// Use that source so access checks work after a bot restart too.
	if ac.client == nil || ac.client.Store == nil || ac.client.Store.LIDs == nil {
		return ""
	}
	phoneJID, err := ac.client.Store.LIDs.GetPNForLID(context.Background(), jid)
	if err != nil {
		log.Debug().Str("sender", MaskJID(jid)).Err(err).Msg("Gagal menyelesaikan WhatsApp LID ke nomor telepon")
		return ""
	}
	if phoneJID.Server != types.DefaultUserServer || phoneJID.User == "" {
		return ""
	}
	return CleanPhoneNumber(phoneJID.User)
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
		log.Warn().Str("sender", MaskJID(sender)).Msg("Tidak dapat mengidentifikasi nomor pengirim; pesan diabaikan")
		return false, false
	}

	isOwner := ac.owners[phone]
	if isOwner || ac.allowed[phone] {
		return true, isOwner
	}

	return false, false
}
