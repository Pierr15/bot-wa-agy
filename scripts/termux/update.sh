#!/data/data/com.termux/files/usr/bin/bash
set -Eeuo pipefail

APP_DIR="${APP_DIR:-$HOME/bot-wa-agy}"
BRANCH="${UPDATE_BRANCH:-main}"
SERVICE="${BOT_SERVICE:-wa-bot}"
cd "$APP_DIR"

command -v git >/dev/null 2>&1 || { echo "Git tidak ditemukan." >&2; exit 1; }
command -v go >/dev/null 2>&1 || { echo "Go tidak ditemukan; biner lama tidak diubah." >&2; exit 1; }

if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "Update dibatalkan: ada perubahan lokal yang belum di-commit." >&2; exit 1
fi
if [ -n "$(git ls-files --others --exclude-standard)" ]; then
  echo "Update dibatalkan: ada file lokal yang belum terlacak." >&2; exit 1
fi

echo "Memeriksa pembaruan origin/$BRANCH..."
git fetch origin "$BRANCH"
LOCAL_HEAD="$(git rev-parse HEAD)"
REMOTE_HEAD="$(git rev-parse "origin/$BRANCH")"
if [ "$LOCAL_HEAD" = "$REMOTE_HEAD" ]; then
  echo "Bot sudah menggunakan commit terbaru ($LOCAL_HEAD)."; exit 0
fi
if ! git merge-base --is-ancestor "$LOCAL_HEAD" "$REMOTE_HEAD"; then
  echo "Update dibatalkan: origin/$BRANCH bukan fast-forward dari HEAD lokal." >&2; exit 1
fi

TMP_DIR="$(mktemp -d "$APP_DIR/.autoupdate.XXXXXX")"
TEMP_BINARY="$TMP_DIR/bot-wa.new"
BACKUP_BINARY="$TMP_DIR/bot-wa.previous"
WORKTREE="$TMP_DIR/source"
cleanup() {
  git worktree remove --force "$WORKTREE" >/dev/null 2>&1 || true
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

echo "Menyiapkan source commit baru di direktori sementara..."
git worktree add --detach "$WORKTREE" "$REMOTE_HEAD"
echo "Build bot-wa dari source terbaru..."
if ! (cd "$WORKTREE" && go build -o "$TEMP_BINARY" ./cmd/bot); then
  echo "Build gagal. Source dan biner aktif tidak diubah." >&2; exit 1
fi
chmod 755 "$TEMP_BINARY"

if [ -f "$APP_DIR/bot-wa" ]; then cp -p "$APP_DIR/bot-wa" "$BACKUP_BINARY"; fi

# Fast-forward only after the candidate binary builds successfully.
git merge --ff-only "$REMOTE_HEAD"
if ! mv -f "$TEMP_BINARY" "$APP_DIR/bot-wa"; then
  echo "Gagal mengganti biner; memulihkan source..." >&2
  git reset --hard "$LOCAL_HEAD" || true
  exit 1
fi

if [ "${BOT_AUTO_UPDATE:-0}" = "1" ]; then
  MARKER="${UPDATE_MARKER:-$APP_DIR/data/autoupdate-pending}"
  mkdir -p "$(dirname "$MARKER")"
  printf '%s\n' "$(git rev-parse HEAD)" > "$MARKER"
  echo "Auto-update siap; marker maintenance dibuat. Service manager akan memulai ulang proses."
elif command -v sv >/dev/null 2>&1; then
  echo "Merestart service $SERVICE..."
  if ! sv restart "$SERVICE"; then
    echo "Restart gagal. Memulihkan biner dan source..." >&2
    [ ! -f "$BACKUP_BINARY" ] || mv -f "$BACKUP_BINARY" "$APP_DIR/bot-wa"
    git reset --hard "$LOCAL_HEAD" || true
    sv restart "$SERVICE" || true
    exit 1
  fi
else
  echo "Perintah sv tidak ditemukan; restart service secara manual." >&2
fi

echo "Update berhasil: $(git rev-parse --short HEAD)"
