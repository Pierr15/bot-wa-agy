#!/data/data/com.termux/files/usr/bin/bash
set -Eeuo pipefail

APP_DIR="${APP_DIR:-$HOME/bot-wa-agy}"
BRANCH="${UPDATE_BRANCH:-main}"
SERVICE="${BOT_SERVICE:-wa-bot}"
cd "$APP_DIR"

command -v git >/dev/null 2>&1 || { echo "Git tidak ditemukan." >&2; exit 1; }
command -v go >/dev/null 2>&1 || { echo "Go tidak ditemukan; biner lama tidak diubah." >&2; exit 1; }

# Jangan menimpa perubahan lokal atau melakukan update saat repo belum bersih.
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "Update dibatalkan: ada perubahan lokal yang belum di-commit." >&2
  exit 1
fi
if [ -n "$(git ls-files --others --exclude-standard)" ]; then
  echo "Update dibatalkan: ada file lokal yang belum terlacak." >&2
  exit 1
fi

echo "Memeriksa pembaruan origin/$BRANCH..."
git fetch origin "$BRANCH"
LOCAL_HEAD="$(git rev-parse HEAD)"
REMOTE_HEAD="$(git rev-parse "origin/$BRANCH")"

if [ "$LOCAL_HEAD" = "$REMOTE_HEAD" ]; then
  echo "Bot sudah menggunakan commit terbaru ($LOCAL_HEAD)."
  exit 0
fi

if ! git merge-base --is-ancestor "$LOCAL_HEAD" "$REMOTE_HEAD"; then
  echo "Update dibatalkan: origin/$BRANCH bukan fast-forward dari HEAD lokal." >&2
  exit 1
fi

OLD_BINARY="$APP_DIR/bot-wa"
TEMP_BINARY="$APP_DIR/.bot-wa.new.$$"
BACKUP_BINARY="$APP_DIR/.bot-wa.previous.$$"
cleanup() { rm -f "$TEMP_BINARY"; }
trap cleanup EXIT

echo "Mengompilasi commit baru ke file sementara..."
if ! go build -o "$TEMP_BINARY" ./cmd/bot; then
  echo "Build gagal. Biner aktif tidak diubah." >&2
  exit 1
fi
chmod 755 "$TEMP_BINARY"

# Update source only after successful build. Preserve binary for rollback.
if [ -f "$OLD_BINARY" ]; then cp -p "$OLD_BINARY" "$BACKUP_BINARY"; fi
git merge --ff-only "$REMOTE_HEAD"

echo "Mengaktifkan biner baru..."
if ! mv -f "$TEMP_BINARY" "$OLD_BINARY"; then
  echo "Gagal mengganti biner; mencoba memulihkan source ke commit sebelumnya." >&2
  git reset --hard "$LOCAL_HEAD" || true
  exit 1
fi

if command -v sv >/dev/null 2>&1; then
  echo "Merestart service $SERVICE..."
  if ! sv restart "$SERVICE"; then
    echo "Restart service gagal. Mencoba rollback biner dan source..." >&2
    [ ! -f "$BACKUP_BINARY" ] || mv -f "$BACKUP_BINARY" "$OLD_BINARY"
    git reset --hard "$LOCAL_HEAD" || true
    sv restart "$SERVICE" || true
    exit 1
  fi
else
  echo "Perintah sv tidak ditemukan. Source dan biner sudah diperbarui; restart service secara manual." >&2
fi

rm -f "$BACKUP_BINARY"
echo "Update berhasil: $(git rev-parse --short HEAD)"
