package bot

import (
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// CleanPhoneNumber strips non-numeric characters and converts 08xx to 628xx.
func CleanPhoneNumber(raw string) string {
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

// ExtractPhoneFromJID retrieves the numerical phone prefix from a standard WhatsApp user JID.
func ExtractPhoneFromJID(jid types.JID) string {
	if jid.Server == types.DefaultUserServer {
		return CleanPhoneNumber(jid.User)
	}
	return ""
}

// MaskJID creates a privacy-friendly representation of a JID for logging.
func MaskJID(jid types.JID) string {
	user := jid.User
	if len(user) <= 4 {
		return "***@" + jid.Server
	}
	return user[:2] + "****" + user[len(user)-2:] + "@" + jid.Server
}
