#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"

echo "=== Links 테스트 ==="

# 빌드
echo "[1] 빌드..."
go build -o links .
echo "  OK"

# 테스트용 DB 생성
TEST_DB="test_links.db"
rm -f "$TEST_DB" "${TEST_DB}-wal" "${TEST_DB}-shm"

# 서버 시작 (테스트 DB)
echo "[2] 서버 시작..."
# main.go가 links.db를 하드코딩하므로 심볼릭 링크로 대체
ln -sf "$TEST_DB" links.db
./links &
SERVER_PID=$!
sleep 1

cleanup() {
  kill $SERVER_PID 2>/dev/null || true
  rm -f "$TEST_DB" "${TEST_DB}-wal" "${TEST_DB}-shm"
  rm -f links.db links.db-wal links.db-shm
}
trap cleanup EXIT

BASE="http://localhost:9900"
PASS=0
FAIL=0

check() {
  local desc="$1" expected="$2" actual="$3"
  if [ "$actual" = "$expected" ]; then
    echo "  PASS: $desc"
    PASS=$((PASS + 1))
  else
    echo "  FAIL: $desc (expected=$expected, got=$actual)"
    FAIL=$((FAIL + 1))
  fi
}

# 임포트
echo "[3] 임포트..."
RES=$(curl -s -X POST "$BASE/api/import")
CATS=$(echo "$RES" | python3 -c "import sys,json; print(json.load(sys.stdin)['categories'])")
LINKS=$(echo "$RES" | python3 -c "import sys,json; print(json.load(sys.stdin)['links'])")
check "카테고리 수 >= 9" "1" "$([ "$CATS" -ge 9 ] && echo 1 || echo 0)"
check "링크 수 >= 100" "1" "$([ "$LINKS" -ge 100 ] && echo 1 || echo 0)"

# 데이터 조회
echo "[4] 데이터 조회..."
DATA=$(curl -s "$BASE/api/data")
TOP_CATS=$(echo "$DATA" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))")
check "최상위 카테고리 수 = 9" "9" "$TOP_CATS"

# 링크 CRUD
echo "[5] 링크 CRUD..."
# 첫 카테고리 ID 가져오기
FIRST_CAT_ID=$(echo "$DATA" | python3 -c "import sys,json; print(json.load(sys.stdin)[0]['id'])")

# 링크 생성
CREATE_RES=$(curl -s -X POST "$BASE/api/links" -H "Content-Type: application/json" -d "{\"category_id\":$FIRST_CAT_ID,\"title\":\"Test Link\",\"url\":\"https://example.com\"}")
LINK_ID=$(echo "$CREATE_RES" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
check "링크 생성 (ID > 0)" "1" "$([ "$LINK_ID" -gt 0 ] && echo 1 || echo 0)"

# 링크 수정
UPDATE_RES=$(curl -s -X PUT "$BASE/api/links/$LINK_ID" -H "Content-Type: application/json" -d '{"title":"Updated Link","url":"https://updated.com"}')
check "링크 수정" "ok" "$(echo "$UPDATE_RES" | python3 -c "import sys,json; print(json.load(sys.stdin)['status'])")"

# 링크 삭제
DEL_RES=$(curl -s -X DELETE "$BASE/api/links/$LINK_ID")
check "링크 삭제" "ok" "$(echo "$DEL_RES" | python3 -c "import sys,json; print(json.load(sys.stdin)['status'])")"

# 카테고리 CRUD
echo "[6] 카테고리 CRUD..."
CAT_RES=$(curl -s -X POST "$BASE/api/categories" -H "Content-Type: application/json" -d '{"name":"test_cat"}')
NEW_CAT_ID=$(echo "$CAT_RES" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
check "카테고리 생성 (ID > 0)" "1" "$([ "$NEW_CAT_ID" -gt 0 ] && echo 1 || echo 0)"

CAT_DEL=$(curl -s -X DELETE "$BASE/api/categories/$NEW_CAT_ID")
check "카테고리 삭제" "ok" "$(echo "$CAT_DEL" | python3 -c "import sys,json; print(json.load(sys.stdin)['status'])")"

# URL 검증
echo "[7] 입력 검증..."
BAD_URL=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/api/links" -H "Content-Type: application/json" -d "{\"category_id\":$FIRST_CAT_ID,\"title\":\"Bad\",\"url\":\"javascript:alert(1)\"}")
check "잘못된 URL 거부" "400" "$BAD_URL"

# 내보내기
echo "[8] 내보내기..."
EXPORT_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/api/export")
check "내보내기 200" "200" "$EXPORT_STATUS"

# 정적 파일
echo "[9] 정적 파일..."
HTML_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$BASE/")
check "index.html 200" "200" "$HTML_STATUS"

echo ""
echo "=== 결과: PASS=$PASS, FAIL=$FAIL ==="
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
