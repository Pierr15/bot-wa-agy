package sticker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ResolveBinary locates a required external binary (e.g. ffmpeg or ffprobe).
// It checks the configured path first (with and without .exe on Windows), then
// common install locations next to the running application so the bot keeps
// working when the binary is simply dropped next to bot-wa.exe.
func ResolveBinary(cfgPath, name string) (string, error) {
	if cfgPath != "" {
		if _, err := os.Stat(cfgPath); err == nil {
			return cfgPath, nil
		}
		if matched, err := exec.LookPath(cfgPath); err == nil {
			return matched, nil
		}
	}

	candidates := searchCandidates(name)
	exts := []string{"", ".exe", ".cmd", ".bat"}
	if runtime.GOOS != "windows" {
		exts = exts[:1]
	}
	for _, dir := range candidates {
		for _, ext := range exts {
			full := filepath.Join(dir, name+ext)
			if info, err := os.Stat(full); err == nil && !info.IsDir() {
				return full, nil
			}
		}
	}

	hint := "install " + name + " terlebih dahulu"
	if runtime.GOOS == "windows" {
		hint = fmt.Sprintf("install %s (mis. `winget install ffmpeg`) atau unduh %s.exe lalu taruh di folder aplikasi", name, name)
	}
	return "", fmt.Errorf(
		"binary %q tidak ditemukan (path terkonfigurasi: %q). %s, atau atur FFMPEG_PATH di .env",
		name, cfgPath, hint,
	)
}

// searchCandidates returns directories, in priority order, that commonly hold
// the ffmpeg/ffprobe binaries for this setup.
func searchCandidates(name string) []string {
	var dirs []string
	add := func(dir string) {
		if dir == "" {
			return
		}
		for _, existing := range dirs {
			if strings.EqualFold(existing, dir) {
				return
			}
		}
		dirs = append(dirs, dir)
	}

	// Directory of the running executable and its ./bin and ./assets/bin.
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		add(exeDir)
		add(filepath.Join(exeDir, "bin"))
		add(filepath.Join(exeDir, "assets", "bin"))
	}

	// Relative to the working directory and its ancestors (covers `go run`,
	// tests running from package dirs, and Termux).
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for i := 0; i < 4 && dir != ""; i++ {
			add(dir)
			add(filepath.Join(dir, "bin"))
			add(filepath.Join(dir, "tools"))
			add(filepath.Join(dir, "assets", "bin"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	if runtime.GOOS == "windows" {
		add(`C:\ffmpeg\bin`)
		add(filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WinGet", "Links"))
		add(filepath.Join(os.Getenv("ProgramFiles"), "ffmpeg", "bin"))
	} else {
		add("/data/data/com.termux/files/usr/bin")
		add("/usr/local/bin")
	}

	return dirs
}
