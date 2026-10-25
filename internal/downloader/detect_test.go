package downloader

import (
	"testing"
)

func TestDetectLinks(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		wantPlatform Platform
		wantCount    int
	}{
		{
			name:         "TikTok standard video",
			text:         "Coba cek video ini https://www.tiktok.com/@user/video/123456789 keren",
			wantPlatform: PlatformTikTok,
			wantCount:    1,
		},
		{
			name:         "TikTok short link",
			text:         "Link: https://vt.tiktok.com/ZS123456/",
			wantPlatform: PlatformTikTok,
			wantCount:    1,
		},
		{
			name:         "Instagram reel",
			text:         "Lihat reel ini https://www.instagram.com/reel/C123456789/?igsh=123",
			wantPlatform: PlatformInstagram,
			wantCount:    1,
		},
		{
			name:         "Instagram post",
			text:         "https://instagram.com/p/C_abcdef/",
			wantPlatform: PlatformInstagram,
			wantCount:    1,
		},
		{
			name:         "Non-matching link",
			text:         "Kunjungi https://google.com atau https://youtube.com/watch?v=123",
			wantPlatform: PlatformUnknown,
			wantCount:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links := DetectLinks(tt.text)
			if len(links) != tt.wantCount {
				t.Fatalf("got %d links, want %d", len(links), tt.wantCount)
			}
			if tt.wantCount > 0 {
				if links[0].Platform != tt.wantPlatform {
					t.Errorf("Platform = %s, want %s", links[0].Platform, tt.wantPlatform)
				}
			}
		})
	}
}
