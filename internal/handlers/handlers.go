package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/IcarusCore/Requestarr/internal/cache"
	"github.com/IcarusCore/Requestarr/internal/models"
	"github.com/IcarusCore/Requestarr/internal/services"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

type Handler struct {
	db            *models.DB
	store         *sessions.CookieStore
	adminPassword string
	tmdb          *services.TMDBService
	sonarr        *services.SonarrService
	radarr        *services.RadarrService
	ratings       *services.RatingsService
	notify        *services.NotificationService
	cache         *cache.Cache
}

func NewHandler(db *models.DB, store *sessions.CookieStore, adminPassword string, tmdb *services.TMDBService, sonarr *services.SonarrService, radarr *services.RadarrService, ratings *services.RatingsService, notify *services.NotificationService, cache *cache.Cache) *Handler {
	return &Handler{
		db:            db,
		store:         store,
		adminPassword: adminPassword,
		tmdb:          tmdb,
		sonarr:        sonarr,
		radarr:        radarr,
		ratings:       ratings,
		notify:        notify,
		cache:         cache,
	}
}

func (h *Handler) jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) errorResponse(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// Middleware
func (h *Handler) AdminRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, _ := h.store.Get(r, "session")
		if session.Values["is_admin"] != true {
			h.errorResponse(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// Health & Status
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	sonarrStatus := "not configured"
	radarrStatus := "not configured"

	if h.db.GetSetting("sonarr_url") != "" && h.db.GetSetting("sonarr_api_key") != "" {
		if _, err := h.sonarr.GetStatus(); err == nil {
			sonarrStatus = "connected"
		} else {
			sonarrStatus = "error: " + err.Error()
		}
	}

	if h.db.GetSetting("radarr_url") != "" && h.db.GetSetting("radarr_api_key") != "" {
		if _, err := h.radarr.GetStatus(); err == nil {
			radarrStatus = "connected"
		} else {
			radarrStatus = "error: " + err.Error()
		}
	}

	h.jsonResponse(w, map[string]interface{}{
		"status": "ok",
		"sonarr": sonarrStatus,
		"radarr": radarrStatus,
	})
}

func (h *Handler) ServicesStatus(w http.ResponseWriter, r *http.Request) {
	sonarrURL := h.db.GetSetting("sonarr_url")
	sonarrKey := h.db.GetSetting("sonarr_api_key")
	radarrURL := h.db.GetSetting("radarr_url")
	radarrKey := h.db.GetSetting("radarr_api_key")

	sonarrConfigured := sonarrURL != "" && sonarrKey != ""
	radarrConfigured := radarrURL != "" && radarrKey != ""

	sonarrConnected := false
	radarrConnected := false

	if sonarrConfigured {
		if _, err := h.sonarr.GetStatus(); err == nil {
			sonarrConnected = true
		}
	}

	if radarrConfigured {
		if _, err := h.radarr.GetStatus(); err == nil {
			radarrConnected = true
		}
	}

	h.jsonResponse(w, map[string]interface{}{
		"sonarr": map[string]bool{"configured": sonarrConfigured, "connected": sonarrConnected},
		"radarr": map[string]bool{"configured": radarrConfigured, "connected": radarrConnected},
	})
}

func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.db.GetStats()
	if err != nil {
		h.errorResponse(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.jsonResponse(w, stats)
}

// Discovery
func (h *Handler) DiscoverSeries(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "popularity.desc"
	}
	year := r.URL.Query().Get("year")

	items, totalPages, err := h.tmdb.DiscoverTV(page, sort, year)
	if err != nil {
		h.errorResponse(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]interface{}{
		"results":    items,
		"page":       page,
		"totalPages": totalPages,
	})
}

func (h *Handler) DiscoverMovies(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "popularity.desc"
	}
	year := r.URL.Query().Get("year")

	items, totalPages, err := h.tmdb.DiscoverMovies(page, sort, year)
	if err != nil {
		h.errorResponse(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]interface{}{
		"results":    items,
		"page":       page,
		"totalPages": totalPages,
	})
}

// Search
func (h *Handler) SearchSeries(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	if len(term) < 2 {
		h.errorResponse(w, "Search term too short", http.StatusBadRequest)
		return
	}

	results, err := h.sonarr.Search(term)
	if err != nil {
		h.errorResponse(w, err.Error(), http.StatusInternalServerError)
		return
	}

	existing, _ := h.sonarr.GetExisting()
	existingIDs := make(map[int]bool)
	for _, s := range existing {
		if id, ok := s["tvdbId"].(float64); ok {
			existingIDs[int(id)] = true
		}
	}

	requestedIDs, _ := h.db.GetRequestedIDs("series")

	enhancedResults := make([]map[string]interface{}, 0, len(results))
	for _, series := range results {
		tvdbID := 0
		if id, ok := series["tvdbId"].(float64); ok {
			tvdbID = int(id)
		}

		status := "available"
		if existingIDs[tvdbID] {
			status = "exists"
		} else if requestedIDs[tvdbID] {
			status = "requested"
		}

		rating := 0.0
		if ratings, ok := series["ratings"].(map[string]interface{}); ok {
			if v, ok := ratings["value"].(float64); ok {
				rating = v
			}
		}

		poster := ""
		fanart := ""
		if images, ok := series["images"].([]interface{}); ok {
			for _, img := range images {
				if imgMap, ok := img.(map[string]interface{}); ok {
					coverType, _ := imgMap["coverType"].(string)
					remoteUrl, _ := imgMap["remoteUrl"].(string)
					if coverType == "poster" && poster == "" {
						poster = remoteUrl
					} else if coverType == "fanart" && fanart == "" {
						fanart = remoteUrl
					}
				}
			}
		}

		enhanced := map[string]interface{}{
			"tvdbId":        tvdbID,
			"title":         series["title"],
			"year":          series["year"],
			"overview":      series["overview"],
			"network":       series["network"],
			"status":        series["status"],
			"rating":        rating,
			"poster":        poster,
			"fanart":        fanart,
			"requestStatus": status,
		}
		enhancedResults = append(enhancedResults, enhanced)
	}

	h.jsonResponse(w, enhancedResults)
}

func (h *Handler) SearchMovies(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	if len(term) < 2 {
		h.errorResponse(w, "Search term too short", http.StatusBadRequest)
		return
	}

	results, err := h.radarr.Search(term)
	if err != nil {
		h.errorResponse(w, err.Error(), http.StatusInternalServerError)
		return
	}

	existing, _ := h.radarr.GetExisting()
	existingIDs := make(map[int]bool)
	for _, m := range existing {
		if id, ok := m["tmdbId"].(float64); ok {
			existingIDs[int(id)] = true
		}
	}

	requestedIDs, _ := h.db.GetRequestedIDs("movie")

	enhancedResults := make([]map[string]interface{}, 0, len(results))
	for _, movie := range results {
		tmdbID := 0
		if id, ok := movie["tmdbId"].(float64); ok {
			tmdbID = int(id)
		}

		status := "available"
		if existingIDs[tmdbID] {
			status = "exists"
		} else if requestedIDs[tmdbID] {
			status = "requested"
		}

		rating := 0.0
		if ratings, ok := movie["ratings"].(map[string]interface{}); ok {
			if tmdbRating, ok := ratings["tmdb"].(map[string]interface{}); ok {
				if v, ok := tmdbRating["value"].(float64); ok {
					rating = v
				}
			} else if v, ok := ratings["value"].(float64); ok {
				rating = v
			}
		}

		poster := ""
		fanart := ""
		if images, ok := movie["images"].([]interface{}); ok {
			for _, img := range images {
				if imgMap, ok := img.(map[string]interface{}); ok {
					coverType, _ := imgMap["coverType"].(string)
					remoteUrl, _ := imgMap["remoteUrl"].(string)
					if coverType == "poster" && poster == "" {
						poster = remoteUrl
					} else if coverType == "fanart" && fanart == "" {
						fanart = remoteUrl
					}
				}
			}
		}

		enhanced := map[string]interface{}{
			"tmdbId":        tmdbID,
			"imdbId":        movie["imdbId"],
			"title":         movie["title"],
			"year":          movie["year"],
			"overview":      movie["overview"],
			"studio":        movie["studio"],
			"runtime":       movie["runtime"],
			"rating":        rating,
			"poster":        poster,
			"fanart":        fanart,
			"requestStatus": status,
		}
		enhancedResults = append(enhancedResults, enhanced)
	}

	h.jsonResponse(w, enhancedResults)
}

// Ratings
func (h *Handler) GetRatings(w http.ResponseWriter, r *http.Request) {
	title := r.URL.Query().Get("title")
	year := r.URL.Query().Get("year")
	mediaType := r.URL.Query().Get("type")
	imdbID := r.URL.Query().Get("imdb_id")
	tmdbID, _ := strconv.Atoi(r.URL.Query().Get("tmdb_id"))

	if title == "" && imdbID == "" && tmdbID == 0 {
		h.errorResponse(w, "Title or ID required", http.StatusBadRequest)
		return
	}

	ratings, err := h.ratings.GetRatings(title, year, mediaType, imdbID, tmdbID)
	if err != nil {
		h.jsonResponse(w, map[string]interface{}{})
		return
	}

	h.jsonResponse(w, ratings)
}

// Requests
func (h *Handler) CreateRequest(w http.ResponseWriter, r *http.Request) {
	var raw map[string]interface{}

	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		h.errorResponse(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Extract fields with type flexibility
	requesterName, _ := raw["requesterName"].(string)
	requesterEmail, _ := raw["requesterEmail"].(string)
	mediaType, _ := raw["mediaType"].(string)
	title, _ := raw["title"].(string)
	poster, _ := raw["poster"].(string)
	imdbID, _ := raw["imdbId"].(string)

	// Handle year - could be string or number
	var year *int
	if y, ok := raw["year"].(float64); ok {
		yi := int(y)
		year = &yi
	} else if y, ok := raw["year"].(string); ok && y != "" {
		if yi, err := strconv.Atoi(y); err == nil {
			year = &yi
		}
	}

	// Handle tmdbId - could be float64 from JSON
	var tmdbID *int
	if id, ok := raw["tmdbId"].(float64); ok {
		i := int(id)
		tmdbID = &i
	}

	// Handle tvdbId
	var tvdbID *int
	if id, ok := raw["tvdbId"].(float64); ok {
		i := int(id)
		tvdbID = &i
	}

	if requesterName == "" || title == "" {
		h.errorResponse(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	if mediaType == "" {
		mediaType = "series"
	}

	// Check if already exists
	if mediaType == "series" {
		if tvdbID == nil {
			h.errorResponse(w, "Missing tvdbId for series", http.StatusBadRequest)
			return
		}
		exists, _ := h.sonarr.CheckExists(*tvdbID)
		if exists {
			h.errorResponse(w, "Series already exists in library", http.StatusConflict)
			return
		}
	} else {
		if tmdbID == nil {
			h.errorResponse(w, "Missing tmdbId for movie", http.StatusBadRequest)
			return
		}
		exists, _ := h.radarr.CheckExists(*tmdbID)
		if exists {
			h.errorResponse(w, "Movie already exists in library", http.StatusConflict)
			return
		}
	}

	// Check for duplicate request
	duplicate, _ := h.db.CheckDuplicateRequest(mediaType, tmdbID, tvdbID)
	if duplicate {
		h.errorResponse(w, "This has already been requested", http.StatusConflict)
		return
	}

	// Build request object
	var reqEmail, reqPoster, reqImdbID *string
	if requesterEmail != "" {
		reqEmail = &requesterEmail
	}
	if poster != "" {
		reqPoster = &poster
	}
	if imdbID != "" {
		reqImdbID = &imdbID
	}

	req := &models.Request{
		RequesterName:  requesterName,
		RequesterEmail: reqEmail,
		MediaType:      mediaType,
		TmdbID:         tmdbID,
		TvdbID:         tvdbID,
		ImdbID:         reqImdbID,
		Title:          title,
		Year:           year,
		Poster:         reqPoster,
	}

	requestID, err := h.db.CreateRequest(req)
	if err != nil {
		h.errorResponse(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.db.LogActivity("request_created", map[string]interface{}{
		"request_id": requestID,
		"media_type": mediaType,
		"title":      title,
		"requester":  requesterName,
	})

	emoji := "📺"
	typeWord := "Series"
	if mediaType == "movie" {
		emoji = "🎬"
		typeWord = "Movie"
	}
	h.notify.Send(fmt.Sprintf("%s New %s Request", emoji, typeWord), fmt.Sprintf("**%s** requested **%s**", requesterName, title), "")

	h.jsonResponse(w, map[string]interface{}{
		"success":   true,
		"requestId": requestID,
		"message":   "Request submitted successfully",
	})
}

func (h *Handler) GetRequests(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	mediaType := r.URL.Query().Get("mediaType")

	requests, err := h.db.GetRequests(status, mediaType)
	if err != nil {
		h.errorResponse(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if requests == nil {
		requests = []models.Request{}
	}

	h.jsonResponse(w, requests)
}

func (h *Handler) GetRequest(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.Atoi(vars["id"])

	req, err := h.db.GetRequest(id)
	if err != nil {
		h.errorResponse(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if req == nil {
		h.errorResponse(w, "Request not found", http.StatusNotFound)
		return
	}

	h.jsonResponse(w, req)
}

func (h *Handler) UpdateRequestStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.Atoi(vars["id"])

	var data struct {
		Status     string `json:"status"`
		AdminNotes string `json:"adminNotes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		h.errorResponse(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	validStatuses := map[string]bool{"pending": true, "approved": true, "rejected": true, "completed": true}
	if !validStatuses[data.Status] {
		h.errorResponse(w, "Invalid status", http.StatusBadRequest)
		return
	}

	if err := h.db.UpdateRequestStatus(id, data.Status, data.AdminNotes); err != nil {
		h.errorResponse(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.db.LogActivity("request_status_updated", map[string]interface{}{
		"request_id": id,
		"new_status": data.Status,
	})

	h.jsonResponse(w, map[string]bool{"success": true})
}

func (h *Handler) ApproveRequest(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.Atoi(vars["id"])

	req, err := h.db.GetRequest(id)
	if err != nil {
		h.errorResponse(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if req == nil {
		h.errorResponse(w, "Request not found", http.StatusNotFound)
		return
	}

	// Parse with flexible types
	var raw map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		h.errorResponse(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	rootFolder, _ := raw["rootFolder"].(string)
	monitor, _ := raw["monitor"].(string)
	minimumAvailability, _ := raw["minimumAvailability"].(string)

	// Handle qualityProfile - could be string or number
	var qualityProfileID int
	if qp, ok := raw["qualityProfile"].(float64); ok {
		qualityProfileID = int(qp)
	} else if qp, ok := raw["qualityProfile"].(string); ok && qp != "" {
		qualityProfileID, _ = strconv.Atoi(qp)
	}

	if qualityProfileID == 0 {
		h.errorResponse(w, "Quality profile required", http.StatusBadRequest)
		return
	}

	var arrID int
	if req.MediaType == "series" {
		if req.TvdbID == nil {
			h.errorResponse(w, "No TVDB ID for series", http.StatusBadRequest)
			return
		}
		if monitor == "" {
			monitor = "all"
		}
		result, err := h.sonarr.AddSeries(*req.TvdbID, rootFolder, qualityProfileID, monitor)
		if err != nil {
			h.errorResponse(w, "Failed to add to Sonarr: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if id, ok := result["id"].(float64); ok {
			arrID = int(id)
		}
	} else {
		if req.TmdbID == nil {
			h.errorResponse(w, "No TMDB ID for movie", http.StatusBadRequest)
			return
		}
		if minimumAvailability == "" {
			minimumAvailability = "announced"
		}
		result, err := h.radarr.AddMovie(*req.TmdbID, rootFolder, qualityProfileID, minimumAvailability)
		if err != nil {
			h.errorResponse(w, "Failed to add to Radarr: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if id, ok := result["id"].(float64); ok {
			arrID = int(id)
		}
	}

	h.db.UpdateRequestStatus(id, "approved", "")
	h.db.UpdateRequestArrID(id, arrID)

	h.db.LogActivity("request_approved", map[string]interface{}{
		"request_id": id,
		"title":      req.Title,
		"arr_id":     arrID,
	})

	emoji := "📺"
	typeWord := "Series"
	if req.MediaType == "movie" {
		emoji = "🎬"
		typeWord = "Movie"
	}
	h.notify.Send(fmt.Sprintf("%s %s Approved", emoji, typeWord), fmt.Sprintf("**%s** has been approved and is being downloaded!", req.Title), "")

	h.jsonResponse(w, map[string]interface{}{
		"success": true,
		"arrId":   arrID,
	})
}

// Admin
func (h *Handler) AdminCheck(w http.ResponseWriter, r *http.Request) {
	session, _ := h.store.Get(r, "session")
	isAdmin := session.Values["is_admin"] == true
	h.jsonResponse(w, map[string]bool{"isAdmin": isAdmin})
}

func (h *Handler) AdminLogin(w http.ResponseWriter, r *http.Request) {
	var data struct {
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		h.errorResponse(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if data.Password != h.adminPassword {
		h.errorResponse(w, "Invalid password", http.StatusUnauthorized)
		return
	}

	session, _ := h.store.Get(r, "session")
	session.Values["is_admin"] = true
	session.Save(r, w)

	h.db.LogActivity("admin_login", nil)

	h.jsonResponse(w, map[string]bool{"success": true})
}

func (h *Handler) AdminLogout(w http.ResponseWriter, r *http.Request) {
	session, _ := h.store.Get(r, "session")
	session.Values["is_admin"] = false
	session.Save(r, w)

	h.jsonResponse(w, map[string]bool{"success": true})
}

func (h *Handler) GetAdminSettings(w http.ResponseWriter, r *http.Request) {
	settings, _ := h.db.GetAllSettings()

	// Initialize as empty slices (not nil) so JSON returns [] instead of null
	sonarrRootFolders := make([]map[string]interface{}, 0)
	sonarrQualityProfiles := make([]map[string]interface{}, 0)
	var sonarrError string
	if settings["sonarr_url"] != "" && settings["sonarr_api_key"] != "" {
		rf, err := h.sonarr.GetRootFolders()
		if err != nil {
			sonarrError = err.Error()
		} else if rf != nil {
			sonarrRootFolders = rf
		}
		qp, err := h.sonarr.GetQualityProfiles()
		if err != nil && sonarrError == "" {
			sonarrError = err.Error()
		} else if qp != nil {
			sonarrQualityProfiles = qp
		}
	}

	radarrRootFolders := make([]map[string]interface{}, 0)
	radarrQualityProfiles := make([]map[string]interface{}, 0)
	var radarrError string
	if settings["radarr_url"] != "" && settings["radarr_api_key"] != "" {
		rf, err := h.radarr.GetRootFolders()
		if err != nil {
			radarrError = err.Error()
		} else if rf != nil {
			radarrRootFolders = rf
		}
		qp, err := h.radarr.GetQualityProfiles()
		if err != nil && radarrError == "" {
			radarrError = err.Error()
		} else if qp != nil {
			radarrQualityProfiles = qp
		}
	}

	h.jsonResponse(w, map[string]interface{}{
		"settings": map[string]string{
			"sonarr_url":      settings["sonarr_url"],
			"sonarr_api_key":  settings["sonarr_api_key"],
			"radarr_url":      settings["radarr_url"],
			"radarr_api_key":  settings["radarr_api_key"],
			"discord_webhook": settings["discord_webhook"],
			"ntfy_url":        settings["ntfy_url"],
			"ntfy_topic":      settings["ntfy_topic"],
			"tmdb_api_key":    settings["tmdb_api_key"],
			"mdblist_api_key": settings["mdblist_api_key"],
		},
		"sonarr": map[string]interface{}{
			"rootFolders":     sonarrRootFolders,
			"qualityProfiles": sonarrQualityProfiles,
			"error":           sonarrError,
		},
		"radarr": map[string]interface{}{
			"rootFolders":     radarrRootFolders,
			"qualityProfiles": radarrQualityProfiles,
			"error":           radarrError,
		},
	})
}

func (h *Handler) UpdateAdminSettings(w http.ResponseWriter, r *http.Request) {
	var data map[string]string

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		h.errorResponse(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	allowedSettings := map[string]bool{
		"sonarr_url":      true,
		"sonarr_api_key":  true,
		"radarr_url":      true,
		"radarr_api_key":  true,
		"discord_webhook": true,
		"ntfy_url":        true,
		"ntfy_topic":      true,
		"tmdb_api_key":    true,
		"mdblist_api_key": true,
	}

	for key, value := range data {
		if allowedSettings[key] {
			h.db.SetSetting(key, value)
		}
	}

	h.db.LogActivity("settings_updated", map[string]interface{}{
		"keys": getKeys(data),
	})

	h.jsonResponse(w, map[string]bool{"success": true})
}

func (h *Handler) TestConnection(w http.ResponseWriter, r *http.Request) {
	var data struct {
		Service string `json:"service"`
		URL     string `json:"url"`
		APIKey  string `json:"apiKey"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		h.errorResponse(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if data.URL == "" || data.APIKey == "" {
		h.errorResponse(w, "URL and API key are required", http.StatusBadRequest)
		return
	}

	var result map[string]interface{}
	var err error

	if data.Service == "sonarr" {
		result, err = h.sonarr.TestConnection(data.URL, data.APIKey)
	} else {
		result, err = h.radarr.TestConnection(data.URL, data.APIKey)
	}

	if err != nil {
		h.jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	h.jsonResponse(w, map[string]interface{}{
		"success": true,
		"version": result["version"],
		"appName": result["appName"],
	})
}

func (h *Handler) GetActivity(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}

	activities, err := h.db.GetActivity(limit)
	if err != nil {
		h.errorResponse(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if activities == nil {
		activities = []models.Activity{}
	}

	h.jsonResponse(w, activities)
}

func getKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
