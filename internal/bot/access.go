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
	cfg *config.Config
	client *whatsmeow.Client
	owners map[string]bool
	allowed map[string]bool
	groups map[string]bool
	lidToPhone map[string]string
	mu sync.RWMutex
}

func NewAccessControl(cfg *config.Config, client *whatsmeow.Client) *AccessControl {
	owners := make(map[string]bool)
	for _, num := range cfg.OwnerNumbers { owners[CleanPhoneNumber(num)] = true }
	allowed := make(map[string]bool)
	for _, num := range cfg.AllowedNumbers { allowed[CleanPhoneNumber(num)] = true }
	groups := make(map[string]bool)
	for _, g := range cfg.AllowedGroups { groups[g] = true }
	return &AccessControl{cfg:cfg, client:client, owners:owners, allowed:allowed, groups:groups, lidToPhone:make(map[string]string)}
}

func (ac *AccessControl) AddAllowed(phone string) {
	phone = CleanPhoneNumber(phone)
	if phone == "" { return }
	ac.mu.Lock()
	ac.allowed[phone] = true
	ac.mu.Unlock()
}

func (ac *AccessControl) RegisterLID(lid, phone string) {
	if lid == "" || phone == "" { return }
	phone = CleanPhoneNumber(phone)
	if phone == "" { return }
	ac.mu.Lock()
	ac.lidToPhone[lid] = phone
	ac.mu.Unlock()
}

func (ac *AccessControl) ResolvePhone(jid types.JID) string {
	if jid.Server == types.DefaultUserServer { return CleanPhoneNumber(jid.User) }
	if jid.Server != types.HiddenUserServer { return "" }
	ac.mu.RLock()
	phone, ok := ac.lidToPhone[jid.User]
	ac.mu.RUnlock()
	if ok && phone != "" { return CleanPhoneNumber(phone) }
	if ac.client == nil || ac.client.Store == nil || ac.client.Store.LIDs == nil { return "" }
	phoneJID, err := ac.client.Store.LIDs.GetPNForLID(context.Background(), jid)
	if err != nil {
		log.Debug().Str("sender", MaskJID(jid)).Err(err).Msg("Gagal menyelesaikan WhatsApp LID ke nomor telepon")
		return ""
	}
	if phoneJID.Server != types.DefaultUserServer || phoneJID.User == "" { return "" }
	return CleanPhoneNumber(phoneJID.User)
}

func (ac *AccessControl) IsOwner(sender types.JID) bool {
	phone := ac.ResolvePhone(sender)
	if phone == "" { return false }
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.owners[phone]
}

func (ac *AccessControl) IsAllowed(sender, chat types.JID) (bool, bool) {
	if chat.Server == types.GroupServer {
		ac.mu.RLock()
		groupAllowed := ac.groups[chat.User]
		ac.mu.RUnlock()
		if !groupAllowed {
			log.Debug().Str("group", chat.String()).Msg("Pesan grup diabaikan (belum masuk whitelist)")
			return false, false
		}
	}
	phone := ac.ResolvePhone(sender)
	if phone == "" {
		log.Warn().Str("sender", MaskJID(sender)).Msg("Tidak dapat mengidentifikasi nomor pengirim; pesan diabaikan")
		return false, false
	}
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	isOwner := ac.owners[phone]
	if isOwner || ac.allowed[phone] { return true, isOwner }
	return false, false
}
