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

type SonarrService struct {
	db     *models.DB
	client *http.Client
}

func NewSonarrService(db *models.DB) *SonarrService {
	return &SonarrService{
		db: db,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *SonarrService) getConfig() (string, string) {
	return s.db.GetSetting("sonarr_url"), s.db.GetSetting("sonarr_api_key")
}

func (s *SonarrService) request(method, endpoint string, data interface{}) (interface{}, error) {
	sonarrURL, apiKey := s.getConfig()
	if sonarrURL == "" || apiKey == "" {
		return nil, fmt.Errorf("Sonarr not configured")
	}

	url := strings.TrimRight(sonarrURL, "/") + "/api/v3/" + endpoint

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
		return nil, fmt.Errorf("Sonarr returned %d", resp.StatusCode)
	}

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *SonarrService) Search(term string) ([]map[string]interface{}, error) {
	result, err := s.request("GET", "series/lookup?term="+term, nil)
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

func (s *SonarrService) GetExisting() ([]map[string]interface{}, error) {
	result, err := s.request("GET", "series", nil)
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

func (s *SonarrService) GetSeries(id int) (map[string]interface{}, error) {
	result, err := s.request("GET", fmt.Sprintf("series/%d", id), nil)
	if err != nil {
		return nil, err
	}
	if m, ok := result.(map[string]interface{}); ok {
		return m, nil
	}
	return nil, nil
}

func (s *SonarrService) GetRootFolders() ([]map[string]interface{}, error) {
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

func (s *SonarrService) GetQualityProfiles() ([]map[string]interface{}, error) {
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

func (s *SonarrService) AddSeries(tvdbID int, rootFolder string, qualityProfileID int, monitor string) (map[string]interface{}, error) {
	// First lookup the series
	result, err := s.request("GET", fmt.Sprintf("series/lookup?term=tvdb:%d", tvdbID), nil)
	if err != nil {
		return nil, err
	}

	arr, ok := result.([]interface{})
	if !ok || len(arr) == 0 {
		return nil, fmt.Errorf("series not found")
	}

	seriesData := arr[0].(map[string]interface{})
	seriesData["rootFolderPath"] = rootFolder
	seriesData["qualityProfileId"] = qualityProfileID
	seriesData["monitored"] = true
	seriesData["seasonFolder"] = true
	seriesData["addOptions"] = map[string]interface{}{
		"monitor":                     monitor,
		"searchForMissingEpisodes":    true,
		"searchForCutoffUnmetEpisodes": false,
	}

	addResult, err := s.request("POST", "series", seriesData)
	if err != nil {
		return nil, err
	}

	if m, ok := addResult.(map[string]interface{}); ok {
		return m, nil
	}
	return nil, nil
}

func (s *SonarrService) CheckExists(tvdbID int) (bool, error) {
	existing, err := s.GetExisting()
	if err != nil {
		return false, err
	}

	for _, series := range existing {
		if id, ok := series["tvdbId"].(float64); ok && int(id) == tvdbID {
			return true, nil
		}
	}
	return false, nil
}

func (s *SonarrService) GetStatus() (map[string]interface{}, error) {
	result, err := s.request("GET", "system/status", nil)
	if err != nil {
		return nil, err
	}
	if m, ok := result.(map[string]interface{}); ok {
		return m, nil
	}
	return nil, nil
}

func (s *SonarrService) TestConnection(url, apiKey string) (map[string]interface{}, error) {
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
