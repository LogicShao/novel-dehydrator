package handlers

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
)

//go:embed static
var staticFiles embed.FS

func StaticHandler() http.Handler {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Printf("static files embed error: %v", err)
		return http.NotFoundHandler()
	}
	return http.FileServer(http.FS(sub))
}
