package main

import (
	"database/sql"
	"fmt"
	"sync"
)

type App struct {
	db      *sql.DB
	reorder sync.Mutex
}

type Category struct {
	ID        int64       `json:"id"`
	Name      string      `json:"name"`
	ParentID  *int64      `json:"parent_id"`
	Hidden    bool        `json:"hidden"`
	SortOrder int         `json:"sort_order"`
	Children  []*Category `json:"children"`
	Links     []*Link     `json:"links"`
}

type Link struct {
	ID         int64  `json:"id"`
	CategoryID int64  `json:"category_id"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	SortOrder  int    `json:"sort_order"`
}

func initDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("DB 열기 실패: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return nil, fmt.Errorf("PRAGMA 실행 실패 (%s): %w", p, err)
		}
	}

	schema := `
	CREATE TABLE IF NOT EXISTS categories (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		parent_id INTEGER REFERENCES categories(id) ON DELETE CASCADE,
		hidden INTEGER NOT NULL DEFAULT 0 CHECK(hidden IN (0, 1)),
		sort_order INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);

	CREATE TABLE IF NOT EXISTS links (
		id INTEGER PRIMARY KEY,
		category_id INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
		title TEXT NOT NULL,
		url TEXT NOT NULL,
		sort_order INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);

	CREATE INDEX IF NOT EXISTS idx_links_category ON links(category_id, sort_order);
	CREATE INDEX IF NOT EXISTS idx_categories_parent ON categories(parent_id);
	`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("스키마 생성 실패: %w", err)
	}

	if _, err := db.Exec("PRAGMA user_version = 1"); err != nil {
		return nil, fmt.Errorf("user_version 설정 실패: %w", err)
	}

	return db, nil
}

func (app *App) getTree() ([]*Category, error) {
	catRows, err := app.db.Query("SELECT id, name, parent_id, hidden, sort_order FROM categories ORDER BY sort_order")
	if err != nil {
		return nil, err
	}
	defer catRows.Close()

	catMap := make(map[int64]*Category)
	var topLevel []*Category

	for catRows.Next() {
		c := &Category{Children: []*Category{}, Links: []*Link{}}
		var parentID sql.NullInt64
		var hidden int
		if err := catRows.Scan(&c.ID, &c.Name, &parentID, &hidden, &c.SortOrder); err != nil {
			return nil, err
		}
		c.Hidden = hidden == 1
		if parentID.Valid {
			pid := parentID.Int64
			c.ParentID = &pid
		}
		catMap[c.ID] = c
	}

	// 링크 로드
	linkRows, err := app.db.Query("SELECT id, category_id, title, url, sort_order FROM links ORDER BY sort_order")
	if err != nil {
		return nil, err
	}
	defer linkRows.Close()

	for linkRows.Next() {
		l := &Link{}
		if err := linkRows.Scan(&l.ID, &l.CategoryID, &l.Title, &l.URL, &l.SortOrder); err != nil {
			return nil, err
		}
		if cat, ok := catMap[l.CategoryID]; ok {
			cat.Links = append(cat.Links, l)
		}
	}

	// 트리 구성
	for _, c := range catMap {
		if c.ParentID != nil {
			if parent, ok := catMap[*c.ParentID]; ok {
				parent.Children = append(parent.Children, c)
				continue
			}
		}
		topLevel = append(topLevel, c)
	}

	// sort_order 기준 정렬
	sortCategories(topLevel)
	for _, c := range catMap {
		sortCategories(c.Children)
	}

	return topLevel, nil
}

func sortCategories(cats []*Category) {
	for i := 0; i < len(cats); i++ {
		for j := i + 1; j < len(cats); j++ {
			if cats[i].SortOrder > cats[j].SortOrder {
				cats[i], cats[j] = cats[j], cats[i]
			}
		}
	}
}

func (app *App) maxSortOrder(table, where string, args ...any) int {
	var maxVal sql.NullInt64
	q := fmt.Sprintf("SELECT MAX(sort_order) FROM %s WHERE %s", table, where)
	app.db.QueryRow(q, args...).Scan(&maxVal)
	if maxVal.Valid {
		return int(maxVal.Int64) + 1000
	}
	return 0
}
