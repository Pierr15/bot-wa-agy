#!/data/data/com.termux/files/usr/bin/bash
set -eu

APP_DIR="${APP_DIR:-$HOME/bot-wa-agy}"
cd "$APP_DIR" || exit 1

echo "Mengambil pembaruan dari repository..."
git pull origin main

if command -v go >/dev/null 2>&1; then
  echo "Mengompilasi ulang biner bot-wa..."
  go build -o bot-wa ./cmd/bot
  chmod +x bot-wa
fi

if command -v sv >/dev/null 2>&1; then
  echo "Merestart service wa-bot..."
  sv restart wa-bot || true
fi

echo "Pembaruan selesai!"
