package downloader

import (
	"net/url"
	"regexp"
	"strings"
)

type Platform string

const (
	PlatformTikTok    Platform = "tiktok"
	PlatformInstagram Platform = "instagram"
	PlatformUnknown   Platform = "unknown"
)

type DetectedLink struct {
	URL      string
	Platform Platform
	Host     string
}

var (
	urlRegex = regexp.MustCompile(`https?://[^\s/$.?#].[^\s]*`)

	tiktokHosts = map[string]bool{
		"tiktok.com":    true,
		"www.tiktok.com": true,
		"vm.tiktok.com": true,
		"vt.tiktok.com": true,
		"m.tiktok.com":  true,
		"v.douyin.com":  true,
	}

	instagramHosts = map[string]bool{
		"instagram.com":     true,
		"www.instagram.com": true,
		"instagr.am":        true,
	}
)

func DetectLinks(text string) []DetectedLink {
	matches := urlRegex.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}

	var results []DetectedLink
	for _, raw := range matches {
		parsed, err := url.Parse(raw)
		if err != nil {
			continue
		}

		host := strings.ToLower(parsed.Hostname())
		// Strip leading 'www.' for lookup
		normalizedHost := strings.TrimPrefix(host, "www.")

		if tiktokHosts[host] || tiktokHosts[normalizedHost] {
			results = append(results, DetectedLink{
				URL:      raw,
				Platform: PlatformTikTok,
				Host:     host,
			})
		} else if instagramHosts[host] || instagramHosts[normalizedHost] {
			path := parsed.Path
			if strings.Contains(path, "/p/") ||
				strings.Contains(path, "/reel/") ||
				strings.Contains(path, "/reels/") ||
				strings.Contains(path, "/tv/") ||
				strings.Contains(path, "/share/") {
				results = append(results, DetectedLink{
					URL:      raw,
					Platform: PlatformInstagram,
					Host:     host,
				})
			}
		}
	}

	return results
}
