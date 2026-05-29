package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/LogicShao/novel-dehydrator/internal/config"
	"github.com/LogicShao/novel-dehydrator/internal/handlers"
	"github.com/LogicShao/novel-dehydrator/internal/services/deepseek"
	"github.com/LogicShao/novel-dehydrator/internal/services/jobmanager"

	customMiddleware "github.com/LogicShao/novel-dehydrator/internal/middleware"
)

// New creates a chi.Mux with all routes registered, middleware chained,
// and dependencies wired to handlers.
func New(pool *pgxpool.Pool, cfg *config.Config, deepseekClient *deepseek.Client, manager *jobmanager.Manager) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware chain
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(customMiddleware.AuthMiddleware(cfg.AuthPassword))

	booksHandler := handlers.NewBooksHandler(pool, cfg)
	foldersHandler := handlers.NewFoldersHandler(pool)

	// --- Static files ---
	r.Get("/static/*", http.StripPrefix("/static", handlers.StaticHandler()).ServeHTTP)

	// --- Page routes ---
	r.Get("/", handlers.HandleIndex())
	r.Get("/login", handlers.HandleLoginPage())
	r.Get("/book/{bookID}", handlers.HandleBookPage())
	r.Get("/book/{bookID}/reader", handlers.HandleReaderPage())

	// --- Auth API ---
	r.Route("/api/auth", func(r chi.Router) {
		r.Post("/login", handlers.HandleLogin(cfg))
		r.Post("/logout", handlers.HandleLogout())
		r.Get("/status", handlers.HandleStatus(cfg))
	})

	// --- Books API ---
	r.Route("/api/books", func(r chi.Router) {
		r.Get("/", booksHandler.HandleListBooks)
		r.Post("/upload", booksHandler.HandleUploadBooks)
		r.Post("/batch-delete", booksHandler.HandleBatchDeleteBooks)
		r.Get("/{bookID}", booksHandler.HandleGetBook)
		r.Delete("/{bookID}", booksHandler.HandleDeleteBook)
		r.Get("/{bookID}/structure", handlers.HandleGetStructure(pool))
		r.Post("/{bookID}/estimate", handlers.HandleEstimate(pool))
		r.Post("/{bookID}/jobs", handlers.HandleCreateJob(pool, cfg, manager))
		r.Get("/{bookID}/jobs/latest", handlers.HandleLatestJob(pool))
		r.Get("/{bookID}/export", handlers.HandleExport(pool, cfg.DataDir))
		r.Get("/{bookID}/chapters/{chapterID}/content", handlers.HandleChapterContent(pool))
		r.Post("/{bookID}/chat", handlers.HandleChapterChat(pool, deepseekClient))
	})

	// --- Jobs API ---
	r.Route("/api/jobs", func(r chi.Router) {
		r.Get("/{jobID}", handlers.HandleGetJob(pool))
		r.Post("/{jobID}/pause", handlers.HandlePauseJob(manager))
		r.Post("/{jobID}/resume", handlers.HandleResumeJob(manager))
		r.Post("/{jobID}/cancel", handlers.HandleCancelJob(pool, manager))
		r.Get("/{jobID}/stream", handlers.HandleJobProgressStream(pool, manager))
	})

	// --- Prompts API ---
	r.Get("/api/prompts/defaults", handlers.HandleGetDefaultPrompts())

	// --- Settings API ---
	r.Route("/api/settings", func(r chi.Router) {
		r.Get("/", handlers.HandleGetSettings(pool, cfg))
		r.Put("/", handlers.HandleUpdateSettings(pool, cfg))
	})

	// --- Folders API ---
	r.Route("/api/folders", func(r chi.Router) {
		r.Get("/", foldersHandler.HandleListFolders)
		r.Post("/", foldersHandler.HandleCreateFolder)
		r.Put("/{folderID}", foldersHandler.HandleRenameFolder)
		r.Delete("/{folderID}", foldersHandler.HandleDeleteFolder)
		r.Post("/{folderID}/books", foldersHandler.HandleAddBooks)
		r.Delete("/{folderID}/books/{bookID}", foldersHandler.HandleRemoveBook)
		r.Get("/{folderID}/books", foldersHandler.HandleListFolderBooks)
	})

	return r
}
