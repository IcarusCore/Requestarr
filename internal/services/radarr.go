package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/IcarusCore/Requestarr/internal/models"
)

type RadarrService struct {
	db     *models.DB
	client *http.Client
}

func NewRadarrService(db *models.DB) *RadarrService {
	return &RadarrService{
		db: db,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *RadarrService) getConfig() (string, string) {
	return s.db.GetSetting("radarr_url"), s.db.GetSetting("radarr_api_key")
}

func (s *RadarrService) request(method, endpoint string, data interface{}) (interface{}, error) {
	radarrURL, apiKey := s.getConfig()
	if radarrURL == "" || apiKey == "" {
		return nil, fmt.Errorf("Radarr not configured")
	}

	url := strings.TrimRight(radarrURL, "/") + "/api/v3/" + endpoint

	var req *http.Request
	var err error

	if data != nil {
		jsonData, _ := json.Marshal(data)
		req, err = http.NewRequest(method, url, bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Api-Key", apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Radarr returned %d", resp.StatusCode)
	}

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *RadarrService) Search(term string) ([]map[string]interface{}, error) {
	result, err := s.request("GET", "movie/lookup?term="+term, nil)
	if err != nil {
		return nil, err
	}

	if arr, ok := result.([]interface{}); ok {
		items := make([]map[string]interface{}, len(arr))
		for i, item := range arr {
			items[i] = item.(map[string]interface{})
		}
		return items, nil
	}
	return nil, nil
}

func (s *RadarrService) GetExisting() ([]map[string]interface{}, error) {
	result, err := s.request("GET", "movie", nil)
	if err != nil {
		return nil, err
	}

	if arr, ok := result.([]interface{}); ok {
		items := make([]map[string]interface{}, len(arr))
		for i, item := range arr {
			items[i] = item.(map[string]interface{})
		}
		return items, nil
	}
	return nil, nil
}

func (s *RadarrService) GetMovie(id int) (map[string]interface{}, error) {
	result, err := s.request("GET", fmt.Sprintf("movie/%d", id), nil)
	if err != nil {
		return nil, err
	}
	if m, ok := result.(map[string]interface{}); ok {
		return m, nil
	}
	return nil, nil
}

func (s *RadarrService) GetRootFolders() ([]map[string]interface{}, error) {
	result, err := s.request("GET", "rootfolder", nil)
	if err != nil {
		return nil, err
	}

	if arr, ok := result.([]interface{}); ok {
		items := make([]map[string]interface{}, len(arr))
		for i, item := range arr {
			items[i] = item.(map[string]interface{})
		}
		return items, nil
	}
	return nil, nil
}

func (s *RadarrService) GetQualityProfiles() ([]map[string]interface{}, error) {
	result, err := s.request("GET", "qualityprofile", nil)
	if err != nil {
		return nil, err
	}

	if arr, ok := result.([]interface{}); ok {
		items := make([]map[string]interface{}, len(arr))
		for i, item := range arr {
			items[i] = item.(map[string]interface{})
		}
		return items, nil
	}
	return nil, nil
}

func (s *RadarrService) AddMovie(tmdbID int, rootFolder string, qualityProfileID int, minimumAvailability string) (map[string]interface{}, error) {
	// First lookup the movie
	result, err := s.request("GET", fmt.Sprintf("movie/lookup/tmdb?tmdbId=%d", tmdbID), nil)
	if err != nil {
		// Try alternative lookup
		result, err = s.request("GET", fmt.Sprintf("movie/lookup?term=tmdb:%d", tmdbID), nil)
		if err != nil {
			return nil, err
		}
	}

	var movieData map[string]interface{}
	if m, ok := result.(map[string]interface{}); ok {
		movieData = m
	} else if arr, ok := result.([]interface{}); ok && len(arr) > 0 {
		movieData = arr[0].(map[string]interface{})
	} else {
		return nil, fmt.Errorf("movie not found")
	}

	movieData["rootFolderPath"] = rootFolder
	movieData["qualityProfileId"] = qualityProfileID
	movieData["monitored"] = true
	movieData["minimumAvailability"] = minimumAvailability
	movieData["addOptions"] = map[string]interface{}{
		"searchForMovie": true,
	}

	addResult, err := s.request("POST", "movie", movieData)
	if err != nil {
		return nil, err
	}

	if m, ok := addResult.(map[string]interface{}); ok {
		return m, nil
	}
	return nil, nil
}

func (s *RadarrService) CheckExists(tmdbID int) (bool, error) {
	existing, err := s.GetExisting()
	if err != nil {
		return false, err
	}

	for _, movie := range existing {
		if id, ok := movie["tmdbId"].(float64); ok && int(id) == tmdbID {
			return true, nil
		}
	}
	return false, nil
}

func (s *RadarrService) GetStatus() (map[string]interface{}, error) {
	result, err := s.request("GET", "system/status", nil)
	if err != nil {
		return nil, err
	}
	if m, ok := result.(map[string]interface{}); ok {
		return m, nil
	}
	return nil, nil
}

func (s *RadarrService) TestConnection(url, apiKey string) (map[string]interface{}, error) {
	req, _ := http.NewRequest("GET", strings.TrimRight(url, "/")+"/api/v3/system/status", nil)
	req.Header.Set("X-Api-Key", apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("Invalid API key")
	}
	if resp.StatusCode == 403 {
		return nil, fmt.Errorf("Access forbidden")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Connection failed: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}
