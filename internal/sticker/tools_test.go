package sticker

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func chdir(t *testing.T, dir string) func() {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return func() { _ = os.Chdir(orig) }
}

func TestResolveBinaryFindsToolNextToExecutableDir(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "ffmpeg"
	if strings.Contains(runtime.GOOS, "windows") {
		name += ".exe"
	}
	if err := os.WriteFile(filepath.Join(binDir, name), []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("FFMPEG_RESOLVE_TEST_CWD", binDir) // penanda berbeda dari PATH sistem
	// ResolveBinary checks cwd/bin, so chdir into the fake app root.
	restore := chdir(t, dir)
	defer restore()

	got, err := ResolveBinary(name, "ffmpeg")
	if err != nil {
		t.Fatalf("ResolveBinary gagal: %v", err)
	}
	if got != filepath.Join(binDir, name) {
		t.Fatalf("path tidak sesuai: %s", got)
	}
}

func TestResolveBinaryMissingGivesHelpfulError(t *testing.T) {
	dir := t.TempDir()
	restore := chdir(t, dir)
	defer restore()

	_, err := ResolveBinary("", "ffmpeg-resolve-test-not-exists")
	if err == nil {
		t.Fatal("seharusnya gagal saat binary tidak ada")
	}
	if !strings.Contains(err.Error(), "tidak ditemukan") || !strings.Contains(err.Error(), "FFMPEG_PATH") {
		t.Fatalf("pesan error tidak membantu: %v", err)
	}
}
