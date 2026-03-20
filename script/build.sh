#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")/.."

echo "=== Links 빌드 ==="
go build -o links .
echo "  -> $(ls -lh links | awk '{print $5}') 빌드 완료"
