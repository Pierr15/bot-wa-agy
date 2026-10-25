package bot

import (
	"testing"
)

func TestCleanPhoneNumber(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "081234567890", want: "6281234567890"},
		{input: "+62 812-3456-7890", want: "6281234567890"},
		{input: "6281234567890", want: "6281234567890"},
		{input: "0857-1234-5678", want: "6285712345678"},
	}

	for _, tt := range tests {
		got := CleanPhoneNumber(tt.input)
		if got != tt.want {
			t.Errorf("CleanPhoneNumber(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
