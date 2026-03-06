package main

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed static/*
var staticFiles embed.FS

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	db, err := initDB("links.db")
	if err != nil {
		slog.Error("DB 초기화 실패", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	app := &App{db: db}

	mux := http.NewServeMux()

	// API
	mux.HandleFunc("GET /api/data", app.handleGetData)
	mux.HandleFunc("POST /api/links", app.handleCreateLink)
	mux.HandleFunc("PUT /api/links/{id}", app.handleUpdateLink)
	mux.HandleFunc("DELETE /api/links/{id}", app.handleDeleteLink)
	mux.HandleFunc("PUT /api/links/{id}/move", app.handleMoveLink)
	mux.HandleFunc("PUT /api/links/reorder", app.handleReorderLinks)
	mux.HandleFunc("POST /api/categories", app.handleCreateCategory)
	mux.HandleFunc("PUT /api/categories/{id}", app.handleUpdateCategory)
	mux.HandleFunc("DELETE /api/categories/{id}", app.handleDeleteCategory)
	mux.HandleFunc("PUT /api/categories/{id}/move", app.handleMoveCategory)
	mux.HandleFunc("PUT /api/categories/reorder", app.handleReorderCategories)
	mux.HandleFunc("POST /api/import", app.handleImport)
	mux.HandleFunc("GET /api/export", app.handleExport)
	mux.HandleFunc("GET /api/favicon", app.handleFavicon)

	// Static files
	subFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	srv := &http.Server{
		Addr:    "127.0.0.1:9900",
		Handler: mux,
	}

	go func() {
		slog.Info("서버 시작", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("서버 에러", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("서버 종료 중...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	slog.Info("서버 종료 완료")
}
