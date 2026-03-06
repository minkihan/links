#!/bin/bash
set -euo pipefail

PROJECT_DIR="/Users/minkihan/Documents/coda/__sub/2026-03-links"
PLIST_NAME="com.coda.links"
PLIST_PATH="$HOME/Library/LaunchAgents/${PLIST_NAME}.plist"
BINARY="$PROJECT_DIR/links"

echo "=== Links 빌드 + 설치 ==="

# 빌드
cd "$PROJECT_DIR"
echo "[1/3] 빌드 중..."
go build -o links .
echo "  -> $(ls -lh links | awk '{print $5}') 빌드 완료"

# plist 생성
echo "[2/3] LaunchAgent plist 생성..."
cat > "$PLIST_PATH" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>${PLIST_NAME}</string>
    <key>ProgramArguments</key>
    <array>
        <string>${BINARY}</string>
    </array>
    <key>WorkingDirectory</key>
    <string>${PROJECT_DIR}</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/links.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/links.log</string>
</dict>
</plist>
PLIST

# 서비스 등록
echo "[3/3] LaunchAgent 등록..."
DOMAIN="gui/$(id -u)"

# 기존 서비스 제거 (있으면)
launchctl bootout "$DOMAIN/$PLIST_NAME" 2>/dev/null || true

# 새로 등록
launchctl bootstrap "$DOMAIN" "$PLIST_PATH"

sleep 1
if launchctl list | grep -q "$PLIST_NAME"; then
    echo ""
    echo "=== 설치 완료 ==="
    echo "  서비스: $PLIST_NAME"
    echo "  주소:   http://localhost:9900"
    echo "  로그:   /tmp/links.log"
else
    echo "경고: 서비스 등록 실패. 로그를 확인하세요."
    echo "  tail -f /tmp/links.log"
    exit 1
fi
