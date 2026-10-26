# Bot WhatsApp — Stiker & Downloader (Go Backend)

Backend Bot WhatsApp privat berkinerja tinggi yang ditulis ulang dalam **Go (Golang)** menggunakan [whatsmeow](https://github.com/tulir/whatsmeow) dan database SQLite murni (`modernc.org/sqlite` tanpa CGO). 

Dirancang khusus untuk berjalan stabil 24/7 di **HP Android (Termux)** dengan konsumsi RAM sangat minim (~25–40 MB), tahan terhadap crash/LMK, serta mudah di-debug di laptop (Windows/Linux/macOS).

---

## 🚀 Fitur Unggulan

- **Stiker Meme Indonesia**:
  - Foto + caption `.s` → Stiker WebP 512×512 (≤100 KB) dengan gaya meme teks putih outline hitam tebal.
  - Teks atas/bawah/tengah via pipe (`.s TEKS BAWAH | TEKS ATAS`) atau prefix (`.s atas: ...`, `.s bawah: ...`).
  - `.st TEKS` → Stiker teks minimalis bergaya Brat: teks hitam lowercase, rata tengah, latar putih.
  - Video/GIF + `.s` → Stiker animasi WebP (maks 6 detik, ≤300 KB) dengan pipeline FFmpeg 2-pass cepat (hemat CPU/baterai).
  - Reply foto/video yang sudah terkirim dengan `.s` untuk memprosesnya.
- **Downloader TikTok & Instagram**:
  - Download video/foto tanpa watermark via `yt-dlp` + fallback `TikWM`.
  - Carousel foto Instagram / slideshow foto TikTok otomatis dikirim berurutan.
  - Pemilihan media lanjutan (reply hasil download):
    - `.v` : kirim video.
    - `.a` : ekstrak audio ke format AAC (`.m4a`).
    - `.f [N]` : kirim semua foto atau foto ke-N.
  - Kompresi video otomatis via FFmpeg jika melebihi batas WhatsApp (16 MB).
- **Keandalan & Manajemen Resource**:
  - **Zero Node.js dependency di HP**: Tidak ada proses kompilasi `tsc` atau ratusan MB `node_modules` di Termux.
  - **SQLite Single Database**: Multi-device auth tersimpan di `data/auth/whatsapp.db` dengan mode WAL (tahan mati lampu/reboot mendadak).
  - **Memory-Safe**: File dialirkan via disk streaming tanpa lonjakan V8 Heap RAM.
  - **Auto-clean**: Pembersihan berkala otomatis untuk file sementara di `data/tmp`.

---

## 📋 Kebutuhan Sistem

- **Di Laptop (Windows/Mac/Linux)**:
  - Go 1.22+
  - `ffmpeg` & `yt-dlp` (opsional jika ingin debug fitur media di laptop)
- **Di HP Android (Termux)**:
  - Termux (dari F-Droid) dengan arsitektur `aarch64`
  - Paket: `ffmpeg`, `python` (`pip install yt-dlp`), `termux-services`

---

## 🛠️ Panduan Penggunaan & Debug di Laptop (Windows)

1. **Salin file konfigurasi**:
   ```powershell
   Copy-Item .env.example .env
   ```
2. **Edit `.env`**:
   - Atur `OWNER_NUMBERS` dan `ALLOWED_NUMBERS` dengan nomor WhatsApp Anda (format: `0812...` atau `62812...`).
   - Pilih `LOGIN_MODE=qr` (tampil QR di terminal) atau `LOGIN_MODE=pairing` (kode pairing 8 digit).
3. **Jalankan bot secara langsung**:
   ```powershell
   go run ./cmd/bot
   ```
   *Atau kompilasi executable:*
   ```powershell
   go build -o bot-wa.exe ./cmd/bot
   .\bot-wa.exe
   ```

---

## 📲 Deploy ke HP Android (Termux)

### Cara 1: Cross-Compile dari Laptop (Sangat Direkomendasikan)
Anda dapat mengompilasi biner langsung dari laptop Windows ke biner Android ARM64:

```powershell
$env:GOOS="android"; $env:GOARCH="arm64"; $env:CGO_ENABLED="0"; go build -o bot-wa-arm64 ./cmd/bot
```

Salin file `bot-wa-arm64`, folder `assets/`, dan file `.env` ke HP (via SSH, Syncthing, atau kabel data ke folder Termux `$HOME/bot-wa-agy`).

### Cara 2: Setup di Termux
Di dalam Termux HP:

```bash
pkg update && pkg upgrade -y
pkg install -y ffmpeg python termux-services
pip install -U yt-dlp

cd ~/bot-wa-agy
cp .env.example .env
# Edit .env dengan nomor Anda: nano .env

# Jalankan installer service 24/7
bash scripts/termux/install-service.sh
```

### Operasional Service Termux
```bash
sv status wa-bot      # Cek status
sv restart wa-bot     # Restart bot
sv down wa-bot        # Hentikan bot
tail -f ~/.local/share/wa-bot/log/current   # Pantau log secara realtime
```

---

## ⚙️ Variabel Konfigurasi (`.env`)

| Variabel | Default | Keterangan |
| :--- | :--- | :--- |
| `LOGIN_MODE` | `qr` | `qr` (scan QR) atau `pairing` (kode 8-digit) |
| `BOT_PHONE_NUMBER` | kosong | Wajib jika `LOGIN_MODE=pairing` |
| `OWNER_NUMBERS` | kosong | Nomor owner, pisahkan dengan koma |
| `ALLOWED_NUMBERS` | kosong | Whitelist nomor pengguna yang diizinkan |
| `DOWNLOAD_MAX_MB` | `16` | Batas ukuran media terkirim |
| `DOWNLOAD_SOURCE_MAX_MB` | `100` | Batas file sumber download `yt-dlp` |
| `DOWNLOAD_MAX_DURATION_SEC`| `300` | Batas durasi video (5 menit) |
| `MIN_FREE_DISK_MB` | `1024` | Batas ruang sisa disk sebelum tolak download |
| `ENABLE_TIKWM_FALLBACK` | `true` | Fallback API TikTok jika `yt-dlp` gagal |
| `IG_COOKIES_PATH` | `data/cookies/instagram.txt` | File cookies Netscape untuk konten IG login-wall |

---

## 📜 Daftar Perintah Bot

| Perintah | Fungsi |
| :--- | :--- |
| `.s [TEKS]` | Buat stiker dari foto/video (bawah / `\|` atas / dua zona) |
| `.st <TEKS>` | Buat stiker teks minimalis gaya Brat berlatar putih |
| `.wm Nama Paket|Author` | Atur metadata untuk stiker berikutnya di chat ini |\n| `.wm info` | Tampilkan metadata aktif dan default |\n| `.wmd Nama Paket|Author` | Ubah metadata default (*khusus owner*) |\n| `.v` | Kirim video dari download terakhir |
| `.a` | Ekstrak audio dari download terakhir |
| `.f [N]` | Kirim semua foto atau foto ke-N dari download terakhir |
| `.menu` | Tampilkan menu bantuan |
| `.ping` | Cek status aktif bot |
| `.stat` | Cek uptime, RAM, disk, & spek sistem (*khusus owner*) |
| `.id` | Cek JID chat & pengirim (*khusus owner*) |
| `.bc <teks>` | Siarkan pesan custom (*khusus owner*) |\n| `.bc upcoming [detail]` | Template pengumuman maintenance mendatang (*khusus owner*) |\n| `.bc maintenance [detail]` | Template maintenance berlangsung (*khusus owner*) |\n| `.bc ends [detail]` | Template maintenance selesai (*khusus owner*) |\n| `.token create [max_use] [durasi]` | Buat token undangan, contoh `.token create 4 30h` (*khusus owner*) |\n| `.init_TOKEN` | Redeem token dalam format yang bisa dipakai di link WhatsApp |
