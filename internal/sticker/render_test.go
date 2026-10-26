package sticker

import "testing"

func TestNormalizeBratText(t *testing.T) {
	tests := []struct {
		name string
		input string
		want string
	}{
		{name: "lowercase ASCII", input: "HELLO WORLD", want: "hello world"},
		{name: "trim whitespace", input: "  HELLO  ", want: "hello"},
		{name: "Unicode and emoji", input: "HÉLLO 世界 😺", want: "héllo 世界 😺"},
		{name: "empty", input: " \t\n ", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeBratText(tt.input); got != tt.want {
				t.Errorf("normalizeBratText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
