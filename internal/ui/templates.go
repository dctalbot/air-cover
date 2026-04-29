package ui

import (
	"embed"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
)

//go:embed html/*.html
var htmlFiles embed.FS

var (
	unauthenticatedTmpl *template.Template
	authenticatedTmpl   *template.Template
)

func init() {
	mustInitTemplates(htmlFiles)
}

func mustInitTemplates(fsys fs.FS) {
	var err error
	unauthenticatedTmpl, err = template.ParseFS(fsys, "html/unauthenticated.html")
	if err != nil {
		slog.Error("Failed to parse unauthenticated template", "error", err)
		panic(err)
	}

	authenticatedTmpl, err = template.ParseFS(fsys, "html/authenticated.html")
	if err != nil {
		slog.Error("Failed to parse authenticated template", "error", err)
		panic(err)
	}
}

func RenderUnauthenticated(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := unauthenticatedTmpl.Execute(w, data); err != nil {
		slog.Error("Failed to write response", "error", err)
	}
}

func RenderAuthenticated(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := authenticatedTmpl.Execute(w, data); err != nil {
		slog.Error("Failed to write response", "error", err)
	}
}
