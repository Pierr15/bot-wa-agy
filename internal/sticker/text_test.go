package sticker

import (
	"testing"
)

func TestParseStickerText(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultPos string
		wantTop    string
		wantBottom string
		wantCenter string
	}{
		{
			name:       "Single bottom text (default)",
			input:      "halo dunia",
			defaultPos: "bottom",
			wantBottom: "HALO DUNIA",
		},
		{
			name:       "Single top text with pipe",
			input:      "| teks atas",
			defaultPos: "bottom",
			wantTop:    "TEKS ATAS",
		},
		{
			name:       "Two zones with pipe",
			input:      "teks bawah | teks atas",
			defaultPos: "bottom",
			wantBottom: "TEKS BAWAH",
			wantTop:    "TEKS ATAS",
		},
		{
			name:       "Prefix atas:",
			input:      "atas: mantap sekali",
			defaultPos: "bottom",
			wantTop:    "MANTAP SEKALI",
		},
		{
			name:       "Prefix tengah:",
			input:      "tengah: di tengah",
			defaultPos: "bottom",
			wantCenter: "DI TENGAH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zones, err := ParseStickerText(tt.input, tt.defaultPos, 140)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if zones.Top != tt.wantTop {
				t.Errorf("Top = %q, want %q", zones.Top, tt.wantTop)
			}
			if zones.Bottom != tt.wantBottom {
				t.Errorf("Bottom = %q, want %q", zones.Bottom, tt.wantBottom)
			}
			if zones.Center != tt.wantCenter {
				t.Errorf("Center = %q, want %q", zones.Center, tt.wantCenter)
			}
		})
	}
}
