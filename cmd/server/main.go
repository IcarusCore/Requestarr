package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/IcarusCore/Requestarr/internal/cache"
	"github.com/IcarusCore/Requestarr/internal/handlers"
	"github.com/IcarusCore/Requestarr/internal/models"
	"github.com/IcarusCore/Requestarr/internal/services"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/rs/cors"
)

//go:embed frontend/static/*
var staticFiles embed.FS

func main() {
	// Get configuration from environment
	port := getEnv("PORT", "5000")
	dbPath := getEnv("DB_PATH", "/config/requestarrr.db")
	adminPassword := getEnv("ADMIN_PASSWORD", "admin")
	secretKey := getEnv("SECRET_KEY", "change-me-in-production-please")

	// Initialize database
	db, err := models.InitDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize default settings from environment
	initDefaultSettings(db)

	// Initialize cache with 10-minute TTL
	appCache := cache.NewCache(10 * time.Minute)

	// Initialize services
	tmdbService := services.NewTMDBService(db, appCache)
	sonarrService := services.NewSonarrService(db)
	radarrService := services.NewRadarrService(db)
	ratingsService := services.NewRatingsService(db, appCache)
	notificationService := services.NewNotificationService(db)

	// Initialize session store
	sessionStore := sessions.NewCookieStore([]byte(secretKey))
	sessionStore.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}

	// Initialize handlers
	h := handlers.NewHandler(db, sessionStore, adminPassword, tmdbService, sonarrService, radarrService, ratingsService, notificationService, appCache)

	// Setup router
	r := mux.NewRouter()

	// API routes
	api := r.PathPrefix("/api").Subrouter()

	// Health & Status
	api.HandleFunc("/health", h.HealthCheck).Methods("GET")
	api.HandleFunc("/services/status", h.ServicesStatus).Methods("GET")
	api.HandleFunc("/stats", h.GetStats).Methods("GET")

	// Discovery
	api.HandleFunc("/discover/series", h.DiscoverSeries).Methods("GET")
	api.HandleFunc("/discover/movies", h.DiscoverMovies).Methods("GET")

	// Search
	api.HandleFunc("/search/series", h.SearchSeries).Methods("GET")
	api.HandleFunc("/search/movies", h.SearchMovies).Methods("GET")
	api.HandleFunc("/search", h.SearchSeries).Methods("GET") // Alias

	// Ratings
	api.HandleFunc("/ratings", h.GetRatings).Methods("GET")

	// Requests
	api.HandleFunc("/request", h.CreateRequest).Methods("POST")
	api.HandleFunc("/requests", h.GetRequests).Methods("GET")
	api.HandleFunc("/requests/{id:[0-9]+}", h.GetRequest).Methods("GET")
	api.HandleFunc("/requests/{id:[0-9]+}/status", h.AdminRequired(h.UpdateRequestStatus)).Methods("PUT")
	api.HandleFunc("/requests/{id:[0-9]+}/approve", h.AdminRequired(h.ApproveRequest)).Methods("POST")

	// Admin
	api.HandleFunc("/admin/check", h.AdminCheck).Methods("GET")
	api.HandleFunc("/admin/login", h.AdminLogin).Methods("POST")
	api.HandleFunc("/admin/logout", h.AdminLogout).Methods("POST")
	api.HandleFunc("/admin/settings", h.AdminRequired(h.GetAdminSettings)).Methods("GET")
	api.HandleFunc("/admin/settings", h.AdminRequired(h.UpdateAdminSettings)).Methods("PUT")
	api.HandleFunc("/admin/test-connection", h.AdminRequired(h.TestConnection)).Methods("POST")
	api.HandleFunc("/admin/activity", h.AdminRequired(h.GetActivity)).Methods("GET")

	// Serve static files
	staticFS, err := fs.Sub(staticFiles, "frontend/static")
	if err != nil {
		log.Fatalf("Failed to get static files: %v", err)
	}
	
	// Serve index.html for root path
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, err := fs.ReadFile(staticFS, "index.html")
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write(data)
	})
	
	// Serve other static files
	r.PathPrefix("/").Handler(http.FileServer(http.FS(staticFS)))

	// Setup CORS
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})

	// Start background task for checking completed downloads
	go startBackgroundTasks(db, sonarrService, radarrService, notificationService)

	// Start server
	handler := c.Handler(r)
	addr := fmt.Sprintf(":%s", port)
	
	log.Printf("🚀 Requestarrr starting on http://0.0.0.0%s", addr)
	log.Printf("📁 Database: %s", dbPath)
	
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func initDefaultSettings(db *models.DB) {
	defaults := map[string]string{
		"sonarr_url":      os.Getenv("SONARR_URL"),
		"sonarr_api_key":  os.Getenv("SONARR_API_KEY"),
		"radarr_url":      os.Getenv("RADARR_URL"),
		"radarr_api_key":  os.Getenv("RADARR_API_KEY"),
		"discord_webhook": os.Getenv("DISCORD_WEBHOOK"),
		"ntfy_url":        os.Getenv("NTFY_URL"),
		"ntfy_topic":      os.Getenv("NTFY_TOPIC"),
		"tmdb_api_key":    os.Getenv("TMDB_API_KEY"),
		"mdblist_api_key": os.Getenv("MDBLIST_API_KEY"),
	}

	for key, value := range defaults {
		if value != "" {
			db.SetSettingIfNotExists(key, value)
		}
	}
}

func startBackgroundTasks(db *models.DB, sonarr *services.SonarrService, radarr *services.RadarrService, notify *services.NotificationService) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		checkCompletedDownloads(db, sonarr, radarr, notify)
	}
}

func checkCompletedDownloads(db *models.DB, sonarr *services.SonarrService, radarr *services.RadarrService, notify *services.NotificationService) {
	requests, err := db.GetApprovedRequests()
	if err != nil {
		log.Printf("Error getting approved requests: %v", err)
		return
	}

	for _, req := range requests {
		if req.ArrID == nil {
			continue
		}

		var completed bool
		if req.MediaType == "series" {
			series, err := sonarr.GetSeries(*req.ArrID)
			if err == nil && series != nil {
				if stats, ok := series["statistics"].(map[string]interface{}); ok {
					if count, ok := stats["episodeFileCount"].(float64); ok && count > 0 {
						completed = true
					}
				}
			}
		} else {
			movie, err := radarr.GetMovie(*req.ArrID)
			if err == nil && movie != nil {
				if hasFile, ok := movie["hasFile"].(bool); ok && hasFile {
					completed = true
				}
			}
		}

		if completed {
			db.UpdateRequestStatus(req.ID, "completed", "")
			db.LogActivity("request_completed", map[string]interface{}{
				"request_id": req.ID,
				"title":      req.Title,
			})
			
			emoji := "🎉"
			mediaWord := "Movie"
			if req.MediaType == "series" {
				mediaWord = "Series"
			}
			notify.Send(fmt.Sprintf("%s %s Ready", emoji, mediaWord), fmt.Sprintf("**%s** is now available to watch!", req.Title), "")
		}
	}
}
