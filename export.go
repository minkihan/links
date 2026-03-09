package main

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const exportDebounce = 1 * time.Minute

type Exporter struct {
	app   *App
	path  string // docs/index.html 경로
	mu    sync.Mutex
	timer *time.Timer
}

func newExporter(app *App, path string) *Exporter {
	return &Exporter{app: app, path: path}
}

// schedule은 디바운스 타이머를 리셋한다.
// 마지막 호출로부터 1분 후에 export를 실행한다.
func (e *Exporter) schedule() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.timer != nil {
		e.timer.Stop()
	}
	e.timer = time.AfterFunc(exportDebounce, func() {
		if err := e.export(); err != nil {
			slog.Error("export 실패", "error", err)
			return
		}
		if err := e.gitPush(); err != nil {
			slog.Error("git push 실패", "error", err)
		}
	})
}

func (e *Exporter) export() error {
	tree, err := e.app.getTree()
	if err != nil {
		return fmt.Errorf("트리 조회: %w", err)
	}

	// 숨김 카테고리 제외
	var visible []*Category
	for _, c := range tree {
		if !c.Hidden {
			visible = append(visible, c)
		}
	}

	now := time.Now().Format("2006-01-02 15:04")
	html := renderHTML(visible, now)

	if err := os.WriteFile(e.path, []byte(html), 0644); err != nil {
		return fmt.Errorf("파일 쓰기: %w", err)
	}
	slog.Info("export 완료", "path", e.path)
	return nil
}

func (e *Exporter) gitPush() error {
	cmds := [][]string{
		{"git", "add", e.path},
		{"git", "commit", "-m", "docs: update links page"},
		{"git", "push", "origin", "main"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			// commit이 실패하면 변경사항 없음 (정상)
			if args[0] == "git" && args[1] == "commit" {
				slog.Info("export 변경사항 없음, push 생략")
				return nil
			}
			return fmt.Errorf("%s: %w", strings.Join(args, " "), err)
		}
	}
	slog.Info("git push 완료")
	return nil
}

func domain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func renderHTML(cats []*Category, updated string) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="ko">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0, user-scalable=no">
<meta name="apple-mobile-web-app-capable" content="yes">
<meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
<title>Links</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  background: #0d1117;
  color: #c9d1d9;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
  padding: 16px;
  padding-top: env(safe-area-inset-top, 16px);
  padding-bottom: calc(env(safe-area-inset-bottom, 0px) + 16px);
  -webkit-text-size-adjust: 100%;
}
.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid #21262d;
}
.header h1 { font-size: 18px; color: #e6b422; font-weight: 600; }
.updated { font-size: 11px; color: #484f58; }
.card {
  background: #161b22;
  border: 1px solid #21262d;
  border-radius: 12px;
  margin-bottom: 12px;
  overflow: hidden;
}
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px;
  background: #1c2128;
  border-bottom: 1px solid #21262d;
}
.card-name { font-size: 14px; font-weight: 600; color: #e6b422; }
.card-count {
  font-size: 11px;
  color: #484f58;
  background: #21262d;
  padding: 2px 8px;
  border-radius: 10px;
}
.links-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 1px;
  background: #21262d;
}
.link-chip {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 14px 6px;
  background: #161b22;
  text-decoration: none;
  min-height: 80px;
  -webkit-tap-highlight-color: rgba(230, 180, 34, 0.1);
  transition: background 0.15s;
}
.link-chip:active { background: #1c2128; }
.link-chip img { width: 20px; height: 20px; border-radius: 4px; }
.link-chip span {
  font-size: 11px;
  color: #8b949e;
  text-align: center;
  line-height: 1.3;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  word-break: break-all;
  max-width: 100%;
  padding: 0 2px;
}
.subcard {
  margin: 8px;
  background: #1c2128;
  border: 1px solid #30363d;
  border-radius: 8px;
  overflow: hidden;
}
.subcard .card-header { background: #21262d; padding: 10px 12px; }
.subcard .card-name { font-size: 13px; color: #bc8cff; }
.subcard .links-grid { background: #30363d; }
.subcard .link-chip { background: #1c2128; }
.subcard .link-chip:active { background: #21262d; }
</style>
</head>
<body>
<div class="header">
  <h1>Links</h1>
  <span class="updated">`)
	b.WriteString(updated)
	b.WriteString(`</span>
</div>
`)

	for _, cat := range cats {
		totalLinks := len(cat.Links)
		for _, child := range cat.Children {
			totalLinks += len(child.Links)
		}

		b.WriteString(`<div class="card">
<div class="card-header">
  <span class="card-name">`)
		b.WriteString(htmlEsc(cat.Name))
		b.WriteString(`</span>
  <span class="card-count">`)
		b.WriteString(fmt.Sprintf("%d", totalLinks))
		b.WriteString(`</span>
</div>
`)

		// 서브카드
		for _, child := range cat.Children {
			b.WriteString(`<div class="subcard">
<div class="card-header">
  <span class="card-name">`)
			b.WriteString(htmlEsc(child.Name))
			b.WriteString(`</span>
  <span class="card-count">`)
			b.WriteString(fmt.Sprintf("%d", len(child.Links)))
			b.WriteString(`</span>
</div>
`)
			writeLinksGrid(&b, child.Links)
			b.WriteString("</div>\n")
		}

		// 카드 본체 링크
		if len(cat.Links) > 0 {
			writeLinksGrid(&b, cat.Links)
		}

		b.WriteString("</div>\n\n")
	}

	b.WriteString("</body>\n</html>")
	return b.String()
}

func writeLinksGrid(b *strings.Builder, links []*Link) {
	b.WriteString(`<div class="links-grid">
`)
	for _, l := range links {
		d := domain(l.URL)
		b.WriteString(`<a class="link-chip" href="`)
		b.WriteString(htmlEsc(l.URL))
		b.WriteString(`">
  <img src="https://www.google.com/s2/favicons?sz=32&domain=`)
		b.WriteString(htmlEsc(d))
		b.WriteString(`" alt="">
  <span>`)
		b.WriteString(htmlEsc(l.Title))
		b.WriteString(`</span>
</a>
`)
	}
	b.WriteString("</div>\n")
}

func htmlEsc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}
