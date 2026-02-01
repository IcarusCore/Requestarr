package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/IcarusCore/Requestarr/internal/cache"
	"github.com/IcarusCore/Requestarr/internal/models"
)

const (
	tmdbBaseURL  = "https://api.themoviedb.org/3"
	tmdbImageURL = "https://image.tmdb.org/t/p"
)

type TMDBService struct {
	db     *models.DB
	cache  *cache.Cache
	client *http.Client
}

type TMDBDiscoverResult struct {
	Results    []map[string]interface{} `json:"results"`
	Page       int                      `json:"page"`
	TotalPages int                      `json:"total_pages"`
}

type MediaItem struct {
	TmdbID        int     `json:"tmdbId,omitempty"`
	TvdbID        int     `json:"tvdbId,omitempty"`
	ImdbID        string  `json:"imdbId,omitempty"`
	Title         string  `json:"title"`
	Year          string  `json:"year,omitempty"`
	Overview      string  `json:"overview,omitempty"`
	Rating        float64 `json:"rating,omitempty"`
	VoteCount     int     `json:"voteCount,omitempty"`
	Poster        string  `json:"poster,omitempty"`
	Fanart        string  `json:"fanart,omitempty"`
	Network       string  `json:"network,omitempty"`
	Runtime       int     `json:"runtime,omitempty"`
	RequestStatus string  `json:"requestStatus"`
	Source        string  `json:"source"`
}

func NewTMDBService(db *models.DB, cache *cache.Cache) *TMDBService {
	return &TMDBService{
		db:    db,
		cache: cache,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (s *TMDBService) getAPIKey() string {
	return s.db.GetSetting("tmdb_api_key")
}

func (s *TMDBService) request(endpoint string, params map[string]string) (map[string]interface{}, error) {
	apiKey := s.getAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("TMDB API key not configured")
	}

	u, _ := url.Parse(tmdbBaseURL + "/" + endpoint)
	q := u.Query()
	q.Set("api_key", apiKey)
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	resp, err := s.client.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("TMDB returned %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *TMDBService) DiscoverMovies(page int, sortBy string, year string) ([]MediaItem, int, error) {
	params := map[string]string{
		"page":                   fmt.Sprintf("%d", page),
		"sort_by":                sortBy,
		"include_adult":          "false",
		"include_video":          "false",
		"with_original_language": "en",
		"region":                 "US",
		"vote_count.gte":         "100",
	}

	if sortBy == "vote_average.desc" {
		params["vote_count.gte"] = "500"
	}

	if year != "" {
		params["primary_release_year"] = year
	}

	data, err := s.request("discover/movie", params)
	if err != nil {
		return nil, 0, err
	}

	results, _ := data["results"].([]interface{})
	totalPages := int(data["total_pages"].(float64))
	if totalPages > 500 {
		totalPages = 500
	}

	// Get existing and requested IDs
	existingIDs, _ := s.getExistingMovieIDs()
	requestedIDs, _ := s.db.GetRequestedIDs("movie")

	// Process results in parallel to fetch external IDs
	items := make([]MediaItem, len(results))
	var wg sync.WaitGroup

	for i, r := range results {
		wg.Add(1)
		go func(idx int, movie map[string]interface{}) {
			defer wg.Done()

			tmdbID := int(movie["id"].(float64))
			
			// Check cache first for external IDs
			cacheKey := fmt.Sprintf("tmdb_movie_%d", tmdbID)
			var imdbID string
			
			if cached, found := s.cache.Get(cacheKey); found {
				imdbID = cached.(string)
			} else {
				// Fetch details to get IMDB ID
				details, err := s.request(fmt.Sprintf("movie/%d", tmdbID), map[string]string{"append_to_response": "external_ids"})
				if err == nil {
					if extIDs, ok := details["external_ids"].(map[string]interface{}); ok {
						if id, ok := extIDs["imdb_id"].(string); ok {
							imdbID = id
						}
					}
					if imdbID == "" {
						if id, ok := details["imdb_id"].(string); ok {
							imdbID = id
						}
					}
					s.cache.Set(cacheKey, imdbID)
				}
			}

			status := "available"
			if existingIDs[tmdbID] {
				status = "exists"
			} else if requestedIDs[tmdbID] {
				status = "requested"
			}

			var posterPath, backdropPath string
			if p, ok := movie["poster_path"].(string); ok {
				posterPath = tmdbImageURL + "/w500" + p
			}
			if b, ok := movie["backdrop_path"].(string); ok {
				backdropPath = tmdbImageURL + "/original" + b
			}

			year := ""
			if rd, ok := movie["release_date"].(string); ok && len(rd) >= 4 {
				year = rd[:4]
			}

			rating := 0.0
			if r, ok := movie["vote_average"].(float64); ok {
				rating = r
			}

			items[idx] = MediaItem{
				TmdbID:        tmdbID,
				ImdbID:        imdbID,
				Title:         getString(movie, "title"),
				Year:          year,
				Overview:      getString(movie, "overview"),
				Rating:        rating,
				VoteCount:     getInt(movie, "vote_count"),
				Poster:        posterPath,
				Fanart:        backdropPath,
				RequestStatus: status,
				Source:        "tmdb",
			}
		}(i, r.(map[string]interface{}))
	}

	wg.Wait()
	return items, totalPages, nil
}

func (s *TMDBService) DiscoverTV(page int, sortBy string, year string) ([]MediaItem, int, error) {
	params := map[string]string{
		"page":                         fmt.Sprintf("%d", page),
		"sort_by":                      sortBy,
		"include_null_first_air_dates": "false",
		"with_original_language":       "en",
		"vote_count.gte":               "50",
	}

	if sortBy == "vote_average.desc" {
		params["vote_count.gte"] = "200"
	}

	if year != "" {
		params["first_air_date_year"] = year
	}

	data, err := s.request("discover/tv", params)
	if err != nil {
		return nil, 0, err
	}

	results, _ := data["results"].([]interface{})
	totalPages := int(data["total_pages"].(float64))
	if totalPages > 500 {
		totalPages = 500
	}

	// Get existing and requested IDs
	existingIDs, _ := s.getExistingSeriesIDs()
	requestedIDs, _ := s.db.GetRequestedIDs("series")

	// Process results in parallel to fetch external IDs
	items := make([]MediaItem, len(results))
	var wg sync.WaitGroup

	for i, r := range results {
		wg.Add(1)
		go func(idx int, show map[string]interface{}) {
			defer wg.Done()

			tmdbID := int(show["id"].(float64))
			
			// Check cache first for external IDs
			cacheKey := fmt.Sprintf("tmdb_tv_%d", tmdbID)
			var tvdbID int
			var imdbID string
			
			if cached, found := s.cache.Get(cacheKey); found {
				if ids, ok := cached.(map[string]interface{}); ok {
					tvdbID = int(ids["tvdb"].(float64))
					imdbID, _ = ids["imdb"].(string)
				}
			} else {
				// Fetch details to get TVDB ID
				details, err := s.request(fmt.Sprintf("tv/%d", tmdbID), map[string]string{"append_to_response": "external_ids"})
				if err == nil {
					if extIDs, ok := details["external_ids"].(map[string]interface{}); ok {
						if id, ok := extIDs["tvdb_id"].(float64); ok {
							tvdbID = int(id)
						}
						if id, ok := extIDs["imdb_id"].(string); ok {
							imdbID = id
						}
					}
					s.cache.Set(cacheKey, map[string]interface{}{"tvdb": float64(tvdbID), "imdb": imdbID})
				}
			}

			status := "available"
			if tvdbID > 0 && existingIDs[tvdbID] {
				status = "exists"
			} else if tvdbID > 0 && requestedIDs[tvdbID] {
				status = "requested"
			}

			var posterPath, backdropPath string
			if p, ok := show["poster_path"].(string); ok {
				posterPath = tmdbImageURL + "/w500" + p
			}
			if b, ok := show["backdrop_path"].(string); ok {
				backdropPath = tmdbImageURL + "/original" + b
			}

			year := ""
			if rd, ok := show["first_air_date"].(string); ok && len(rd) >= 4 {
				year = rd[:4]
			}

			rating := 0.0
			if r, ok := show["vote_average"].(float64); ok {
				rating = r
			}

			items[idx] = MediaItem{
				TmdbID:        tmdbID,
				TvdbID:        tvdbID,
				ImdbID:        imdbID,
				Title:         getString(show, "name"),
				Year:          year,
				Overview:      getString(show, "overview"),
				Rating:        rating,
				VoteCount:     getInt(show, "vote_count"),
				Poster:        posterPath,
				Fanart:        backdropPath,
				RequestStatus: status,
				Source:        "tmdb",
			}
		}(i, r.(map[string]interface{}))
	}

	wg.Wait()
	return items, totalPages, nil
}

func (s *TMDBService) getExistingMovieIDs() (map[int]bool, error) {
	// Get from Radarr
	radarrURL := s.db.GetSetting("radarr_url")
	radarrKey := s.db.GetSetting("radarr_api_key")
	
	if radarrURL == "" || radarrKey == "" {
		return map[int]bool{}, nil
	}

	cacheKey := "existing_movies"
	if cached, found := s.cache.Get(cacheKey); found {
		return cached.(map[int]bool), nil
	}

	req, _ := http.NewRequest("GET", radarrURL+"/api/v3/movie", nil)
	req.Header.Set("X-Api-Key", radarrKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return map[int]bool{}, err
	}
	defer resp.Body.Close()

	var movies []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&movies); err != nil {
		return map[int]bool{}, err
	}

	ids := make(map[int]bool)
	for _, m := range movies {
		if id, ok := m["tmdbId"].(float64); ok {
			ids[int(id)] = true
		}
	}

	s.cache.SetWithTTL(cacheKey, ids, 2*time.Minute)
	return ids, nil
}

func (s *TMDBService) getExistingSeriesIDs() (map[int]bool, error) {
	// Get from Sonarr
	sonarrURL := s.db.GetSetting("sonarr_url")
	sonarrKey := s.db.GetSetting("sonarr_api_key")
	
	if sonarrURL == "" || sonarrKey == "" {
		return map[int]bool{}, nil
	}

	cacheKey := "existing_series"
	if cached, found := s.cache.Get(cacheKey); found {
		return cached.(map[int]bool), nil
	}

	req, _ := http.NewRequest("GET", sonarrURL+"/api/v3/series", nil)
	req.Header.Set("X-Api-Key", sonarrKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return map[int]bool{}, err
	}
	defer resp.Body.Close()

	var series []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&series); err != nil {
		return map[int]bool{}, err
	}

	ids := make(map[int]bool)
	for _, s := range series {
		if id, ok := s["tvdbId"].(float64); ok {
			ids[int(id)] = true
		}
	}

	s.cache.SetWithTTL(cacheKey, ids, 2*time.Minute)
	return ids, nil
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getInt(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}
