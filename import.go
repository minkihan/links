package main

import (
	"fmt"
	"regexp"
	"strings"
)

var linkRegex = regexp.MustCompile(`\*\s+\[([^\]]+)\]\(([^)]+)\)`)

type ImportStats struct {
	Categories int `json:"categories"`
	Links      int `json:"links"`
}

func (app *App) importLinksData(content string) (*ImportStats, error) {
	hiddenCats := map[string]bool{"comics": true, "xxx": true}
	subCatParent := map[string]string{"work_old": "work"}

	type parsedLink struct {
		title string
		url   string
	}
	type parsedCat struct {
		name  string
		links []parsedLink
	}

	var categories []parsedCat
	var current *parsedCat

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "### ") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "### "))
			categories = append(categories, parsedCat{name: name})
			current = &categories[len(categories)-1]
			continue
		}
		if current == nil {
			continue
		}
		matches := linkRegex.FindStringSubmatch(line)
		if len(matches) == 3 {
			current.links = append(current.links, parsedLink{title: matches[1], url: matches[2]})
		}
	}

	tx, err := app.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("트랜잭션 시작 실패: %w", err)
	}
	defer tx.Rollback()

	// 기존 데이터 삭제
	tx.Exec("DELETE FROM links")
	tx.Exec("DELETE FROM categories")

	catIDs := make(map[string]int64)
	stats := &ImportStats{}

	// 1차: 모든 카테고리 생성 (서브카드 제외)
	catOrder := 0
	for _, cat := range categories {
		if _, isSub := subCatParent[cat.name]; isSub {
			continue
		}
		hidden := 0
		if hiddenCats[cat.name] {
			hidden = 1
		}
		res, err := tx.Exec("INSERT INTO categories (name, hidden, sort_order) VALUES (?, ?, ?)",
			cat.name, hidden, catOrder*1000)
		if err != nil {
			return nil, fmt.Errorf("카테고리 '%s' 생성 실패: %w", cat.name, err)
		}
		id, _ := res.LastInsertId()
		catIDs[cat.name] = id
		catOrder++
		stats.Categories++
	}

	// 2차: 서브카드 생성
	for _, cat := range categories {
		parentName, isSub := subCatParent[cat.name]
		if !isSub {
			continue
		}
		parentID, ok := catIDs[parentName]
		if !ok {
			continue
		}
		res, err := tx.Exec("INSERT INTO categories (name, parent_id, sort_order) VALUES (?, ?, ?)",
			cat.name, parentID, 0)
		if err != nil {
			return nil, fmt.Errorf("서브카테고리 '%s' 생성 실패: %w", cat.name, err)
		}
		id, _ := res.LastInsertId()
		catIDs[cat.name] = id
		stats.Categories++
	}

	// 3차: 링크 생성
	for _, cat := range categories {
		catID, ok := catIDs[cat.name]
		if !ok {
			continue
		}
		for i, link := range cat.links {
			_, err := tx.Exec("INSERT INTO links (category_id, title, url, sort_order) VALUES (?, ?, ?, ?)",
				catID, link.title, link.url, i*1000)
			if err != nil {
				return nil, fmt.Errorf("링크 '%s' 생성 실패: %w", link.title, err)
			}
			stats.Links++
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("커밋 실패: %w", err)
	}
	return stats, nil
}
