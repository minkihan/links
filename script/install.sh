#!/bin/bash
set -euo pipefail

PROJECT_DIR="/Users/minkihan/Documents/coda/__sub/2026-03-links"
PLIST_NAME="com.coda.links"
PLIST_PATH="$HOME/Library/LaunchAgents/${PLIST_NAME}.plist"
BINARY="$PROJECT_DIR/links"
DOMAIN="gui/$(id -u)"

cd "$PROJECT_DIR"

# 1. 빌드
echo "=== Links 빌드 + 설치 ==="
./script/build.sh

# 2. plist 생성 (없으면)
if [ ! -f "$PLIST_PATH" ]; then
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
else
    echo "[2/3] plist 이미 존재, 스킵"
fi

# 3. 서비스 재시작
echo "[3/3] 서비스 재시작..."
# KeepAlive=true이므로 프로세스만 kill하면 새 바이너리로 자동 재시작
PID=$(launchctl list | awk "/$PLIST_NAME/"'{print $1}')
if [ -n "$PID" ] && [ "$PID" != "-" ]; then
    kill "$PID" 2>/dev/null || true
else
    # 서비스가 등록되지 않은 경우
    launchctl bootstrap "$DOMAIN" "$PLIST_PATH"
fi

sleep 1
if launchctl list | grep -q "$PLIST_NAME"; then
    echo ""
    echo "=== 완료 ==="
    echo "  http://localhost:9900"
else
    echo "서비스 등록 실패. tail -f /tmp/links.log"
    exit 1
fi
