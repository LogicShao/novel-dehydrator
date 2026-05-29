package handlers

import (
	"embed"
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"
)

//go:embed templates
var templateFiles embed.FS

var templates = template.Must(template.New("").ParseFS(templateFiles, "templates/*.html"))

func HandleIndex() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderTemplate(w, "index.html", nil)
	}
}

func HandleLoginPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderTemplate(w, "login.html", nil)
	}
}

func HandleBookPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bookID := chi.URLParam(r, "bookID")
		renderTemplate(w, "book.html", map[string]string{
			"BookID": bookID,
		})
	}
}

func HandleReaderPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bookID := chi.URLParam(r, "bookID")
		renderTemplate(w, "reader.html", map[string]string{
			"BookID": bookID,
		})
	}
}

func renderTemplate(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template render error", http.StatusInternalServerError)
	}
}

