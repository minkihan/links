package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func sanitizeText(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	return html.EscapeString(s)
}

func validateURL(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// GET /api/data
func (app *App) handleGetData(w http.ResponseWriter, r *http.Request) {
	tree, err := app.getTree()
	if err != nil {
		slog.Error("트리 조회 실패", "error", err)
		writeError(w, 500, "데이터 조회 실패")
		return
	}
	writeJSON(w, 200, tree)
}

// POST /api/links
func (app *App) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CategoryID int64  `json:"category_id"`
		Title      string `json:"title"`
		URL        string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "잘못된 요청")
		return
	}
	req.Title = sanitizeText(req.Title, 500)
	req.URL = strings.TrimSpace(req.URL)
	if req.Title == "" || req.URL == "" {
		writeError(w, 400, "제목과 URL은 필수입니다")
		return
	}
	if !validateURL(req.URL) {
		writeError(w, 400, "URL은 http:// 또는 https://로 시작해야 합니다")
		return
	}

	sort := app.maxSortOrder("links", "category_id = ?", req.CategoryID)
	res, err := app.db.Exec("INSERT INTO links (category_id, title, url, sort_order) VALUES (?, ?, ?, ?)",
		req.CategoryID, req.Title, req.URL, sort)
	if err != nil {
		slog.Error("링크 생성 실패", "error", err)
		writeError(w, 500, "링크 생성 실패")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]int64{"id": id})
}

// PUT /api/links/{id}
func (app *App) handleUpdateLink(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, 400, "잘못된 ID")
		return
	}
	var req struct {
		Title      string `json:"title"`
		URL        string `json:"url"`
		CategoryID *int64 `json:"category_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "잘못된 요청")
		return
	}
	req.Title = sanitizeText(req.Title, 500)
	req.URL = strings.TrimSpace(req.URL)
	if req.Title == "" || req.URL == "" {
		writeError(w, 400, "제목과 URL은 필수입니다")
		return
	}
	if !validateURL(req.URL) {
		writeError(w, 400, "URL은 http:// 또는 https://로 시작해야 합니다")
		return
	}

	if req.CategoryID != nil {
		_, err = app.db.Exec("UPDATE links SET title=?, url=?, category_id=? WHERE id=?",
			req.Title, req.URL, *req.CategoryID, id)
	} else {
		_, err = app.db.Exec("UPDATE links SET title=?, url=? WHERE id=?",
			req.Title, req.URL, id)
	}
	if err != nil {
		slog.Error("링크 수정 실패", "error", err)
		writeError(w, 500, "링크 수정 실패")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// DELETE /api/links/{id}
func (app *App) handleDeleteLink(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, 400, "잘못된 ID")
		return
	}
	_, err = app.db.Exec("DELETE FROM links WHERE id=?", id)
	if err != nil {
		slog.Error("링크 삭제 실패", "error", err)
		writeError(w, 500, "링크 삭제 실패")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// PUT /api/links/{id}/move
func (app *App) handleMoveLink(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, 400, "잘못된 ID")
		return
	}
	var req struct {
		CategoryID int64 `json:"category_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "잘못된 요청")
		return
	}
	sort := app.maxSortOrder("links", "category_id = ?", req.CategoryID)
	_, err = app.db.Exec("UPDATE links SET category_id=?, sort_order=? WHERE id=?",
		req.CategoryID, sort, id)
	if err != nil {
		slog.Error("링크 이동 실패", "error", err)
		writeError(w, 500, "링크 이동 실패")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// PUT /api/links/reorder
func (app *App) handleReorderLinks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CategoryID int64   `json:"category_id"`
		OrderedIDs []int64 `json:"ordered_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "잘못된 요청")
		return
	}

	app.reorder.Lock()
	defer app.reorder.Unlock()

	tx, err := app.db.Begin()
	if err != nil {
		writeError(w, 500, "트랜잭션 시작 실패")
		return
	}
	defer tx.Rollback()

	for i, id := range req.OrderedIDs {
		if _, err := tx.Exec("UPDATE links SET sort_order=? WHERE id=? AND category_id=?", i, id, req.CategoryID); err != nil {
			slog.Error("링크 순서 변경 실패", "error", err)
			writeError(w, 500, "순서 변경 실패")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 500, "커밋 실패")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// POST /api/categories
func (app *App) handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		ParentID *int64 `json:"parent_id"`
		Hidden   bool   `json:"hidden"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "잘못된 요청")
		return
	}
	req.Name = sanitizeText(req.Name, 500)
	if req.Name == "" {
		writeError(w, 400, "이름은 필수입니다")
		return
	}

	// 서브카드 검증: parent가 이미 서브카드면 거부
	if req.ParentID != nil {
		var parentParentID sql.NullInt64
		err := app.db.QueryRow("SELECT parent_id FROM categories WHERE id=?", *req.ParentID).Scan(&parentParentID)
		if err != nil {
			writeError(w, 400, "존재하지 않는 상위 카테고리")
			return
		}
		if parentParentID.Valid {
			writeError(w, 400, "2단계 중첩은 지원하지 않습니다")
			return
		}
	}

	var where string
	var args []any
	if req.ParentID != nil {
		where = "parent_id = ?"
		args = []any{*req.ParentID}
	} else {
		where = "parent_id IS NULL"
	}
	sort := app.maxSortOrder("categories", where, args...)

	var res sql.Result
	var err error
	hidden := 0
	if req.Hidden {
		hidden = 1
	}
	if req.ParentID != nil {
		res, err = app.db.Exec("INSERT INTO categories (name, parent_id, hidden, sort_order) VALUES (?, ?, ?, ?)",
			req.Name, *req.ParentID, hidden, sort)
	} else {
		res, err = app.db.Exec("INSERT INTO categories (name, hidden, sort_order) VALUES (?, ?, ?)",
			req.Name, hidden, sort)
	}
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, 409, "이미 존재하는 카테고리 이름입니다")
			return
		}
		slog.Error("카테고리 생성 실패", "error", err)
		writeError(w, 500, "카테고리 생성 실패")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, map[string]int64{"id": id})
}

// PUT /api/categories/{id}
func (app *App) handleUpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, 400, "잘못된 ID")
		return
	}
	var req struct {
		Name   *string `json:"name"`
		Hidden *bool   `json:"hidden"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "잘못된 요청")
		return
	}

	if req.Name != nil {
		name := sanitizeText(*req.Name, 500)
		if name == "" {
			writeError(w, 400, "이름은 비어있을 수 없습니다")
			return
		}
		if _, err := app.db.Exec("UPDATE categories SET name=? WHERE id=?", name, id); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				writeError(w, 409, "이미 존재하는 카테고리 이름입니다")
				return
			}
			writeError(w, 500, "카테고리 수정 실패")
			return
		}
	}
	if req.Hidden != nil {
		h := 0
		if *req.Hidden {
			h = 1
		}
		if _, err := app.db.Exec("UPDATE categories SET hidden=? WHERE id=?", h, id); err != nil {
			writeError(w, 500, "카테고리 수정 실패")
			return
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// DELETE /api/categories/{id}
func (app *App) handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, 400, "잘못된 ID")
		return
	}
	_, err = app.db.Exec("DELETE FROM categories WHERE id=?", id)
	if err != nil {
		slog.Error("카테고리 삭제 실패", "error", err)
		writeError(w, 500, "카테고리 삭제 실패")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// PUT /api/categories/{id}/move
func (app *App) handleMoveCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, 400, "잘못된 ID")
		return
	}
	var req struct {
		ParentID *int64 `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "잘못된 요청")
		return
	}

	// 순환 참조 방지
	if req.ParentID != nil {
		targetID := *req.ParentID
		if targetID == id {
			writeError(w, 400, "자기 자신에게 이동할 수 없습니다")
			return
		}
		// target이 이미 서브카드면 거부
		var targetParent sql.NullInt64
		err := app.db.QueryRow("SELECT parent_id FROM categories WHERE id=?", targetID).Scan(&targetParent)
		if err != nil {
			writeError(w, 400, "존재하지 않는 카테고리")
			return
		}
		if targetParent.Valid {
			writeError(w, 400, "서브카드 안에 넣을 수 없습니다 (1단계만 허용)")
			return
		}
		// source에 자식이 있으면 거부
		var childCount int
		app.db.QueryRow("SELECT COUNT(*) FROM categories WHERE parent_id=?", id).Scan(&childCount)
		if childCount > 0 {
			writeError(w, 400, "자식이 있는 카테고리는 서브카드로 넣을 수 없습니다")
			return
		}
	}

	if req.ParentID != nil {
		_, err = app.db.Exec("UPDATE categories SET parent_id=? WHERE id=?", *req.ParentID, id)
	} else {
		_, err = app.db.Exec("UPDATE categories SET parent_id=NULL WHERE id=?", id)
	}
	if err != nil {
		slog.Error("카테고리 이동 실패", "error", err)
		writeError(w, 500, "카테고리 이동 실패")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// PUT /api/categories/reorder
func (app *App) handleReorderCategories(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ParentID   *int64  `json:"parent_id"`
		OrderedIDs []int64 `json:"ordered_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "잘못된 요청")
		return
	}

	app.reorder.Lock()
	defer app.reorder.Unlock()

	tx, err := app.db.Begin()
	if err != nil {
		writeError(w, 500, "트랜잭션 시작 실패")
		return
	}
	defer tx.Rollback()

	for i, id := range req.OrderedIDs {
		if req.ParentID != nil {
			if _, err := tx.Exec("UPDATE categories SET sort_order=? WHERE id=? AND parent_id=?", i, id, *req.ParentID); err != nil {
				writeError(w, 500, "순서 변경 실패")
				return
			}
		} else {
			if _, err := tx.Exec("UPDATE categories SET sort_order=? WHERE id=? AND parent_id IS NULL", i, id); err != nil {
				writeError(w, 500, "순서 변경 실패")
				return
			}
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 500, "커밋 실패")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// POST /api/import
func (app *App) handleImport(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile("links.md")
	if err != nil {
		slog.Error("links.md 읽기 실패", "error", err)
		writeError(w, 500, "links.md 파일을 찾을 수 없습니다")
		return
	}

	stats, err := app.importLinksData(string(data))
	if err != nil {
		slog.Error("임포트 실패", "error", err)
		writeError(w, 500, fmt.Sprintf("임포트 실패: %v", err))
		return
	}
	writeJSON(w, 200, stats)
}

// GET /api/export
func (app *App) handleExport(w http.ResponseWriter, r *http.Request) {
	tree, err := app.getTree()
	if err != nil {
		slog.Error("내보내기 실패", "error", err)
		writeError(w, 500, "내보내기 실패")
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=links-export.json")
	writeJSON(w, 200, tree)
}

// GET /api/favicon
func (app *App) handleFavicon(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		writeError(w, 400, "domain 파라미터 필수")
		return
	}
	// 안전한 문자만 허용
	for _, c := range domain {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-') {
			writeError(w, 400, "잘못된 도메인")
			return
		}
	}

	dir := "favicons"
	os.MkdirAll(dir, 0755)
	cached := filepath.Join(dir, domain+".png")

	if _, err := os.Stat(cached); err == nil {
		http.ServeFile(w, r, cached)
		return
	}

	resp, err := http.Get(fmt.Sprintf("https://www.google.com/s2/favicons?domain=%s&sz=64", domain))
	if err != nil {
		writeError(w, 502, "favicon 가져오기 실패")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, 502, "favicon 읽기 실패")
		return
	}

	os.WriteFile(cached, body, 0644)
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", "public, max-age=604800")
	w.Write(body)
}
