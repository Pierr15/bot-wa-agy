#!/data/data/com.termux/files/usr/bin/bash
set -eu

APP_DIR="${APP_DIR:-$HOME/bot-wa-agy}"
PREFIX="${PREFIX:-/data/data/com.termux/files/usr}"
BOT_SERVICE="wa-bot"
BOT_SERVICE_DIR="$PREFIX/var/service/$BOT_SERVICE"
LOG_DIR="$HOME/.local/share/wa-bot/log"
BOOT_DIR="$HOME/.termux/boot"
BOOT_SCRIPT="$BOOT_DIR/start-wa-bot"
TZ_VALUE="${TZ:-Asia/Jakarta}"

if [ ! -f "$APP_DIR/bot-wa" ]; then
  if [ -f "$APP_DIR/bot-wa-arm64" ]; then
    cp "$APP_DIR/bot-wa-arm64" "$APP_DIR/bot-wa"
    chmod +x "$APP_DIR/bot-wa"
  else
    echo "Biner bot-wa tidak ditemukan. Kompilasi terlebih dahulu dengan:" >&2
    echo "  cd $APP_DIR && go build -o bot-wa ./cmd/bot" >&2
    exit 1
  fi
fi

if [ ! -f "$APP_DIR/.env" ]; then
  echo "Peringatan: $APP_DIR/.env belum ada. Salin dari .env.example terlebih dahulu." >&2
fi

if ! command -v sv >/dev/null 2>&1; then
  echo "termux-services belum terpasang. Jalankan: pkg install -y termux-services" >&2
  exit 1
fi

mkdir -p "$BOT_SERVICE_DIR/log" "$LOG_DIR" "$BOOT_DIR"

cat >"$BOT_SERVICE_DIR/run" <<EOF
#!/data/data/com.termux/files/usr/bin/sh
export PATH="$PREFIX/bin:\$PATH"
export TZ="$TZ_VALUE"
cd "$APP_DIR" || exit 1
exec 2>&1
exec ./bot-wa
EOF

cat >"$BOT_SERVICE_DIR/log/run" <<EOF
#!/data/data/com.termux/files/usr/bin/sh
export PATH="$PREFIX/bin:\$PATH"
mkdir -p "$LOG_DIR"
exec svlogd -tt "$LOG_DIR"
EOF

cat >"$BOOT_SCRIPT" <<EOF
#!/data/data/com.termux/files/usr/bin/sh
"$PREFIX/bin/termux-wake-lock" 2>/dev/null || true
if [ -r "$PREFIX/etc/profile.d/start-services.sh" ]; then
  . "$PREFIX/etc/profile.d/start-services.sh"
fi
EOF

chmod 755 \
  "$BOT_SERVICE_DIR/run" \
  "$BOT_SERVICE_DIR/log/run" \
  "$BOOT_SCRIPT"

if [ -f "$APP_DIR/.env" ]; then
  chmod 600 "$APP_DIR/.env"
fi

termux-wake-lock 2>/dev/null || true
sv-enable "$BOT_SERVICE" 2>/dev/null || true
sv up "$BOT_SERVICE" || true

echo
echo "=========================================================="
echo " Service wa-bot (Go Backend) berhasil dipasang!"
echo " Log bot : tail -f $LOG_DIR/current"
echo " Status  : sv status $BOT_SERVICE"
echo " Restart : sv restart $BOT_SERVICE"
echo " Stop    : sv down $BOT_SERVICE"
echo "=========================================================="
