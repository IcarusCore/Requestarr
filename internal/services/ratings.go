package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/IcarusCore/Requestarr/internal/cache"
	"github.com/IcarusCore/Requestarr/internal/models"
)

const (
	rtAlgoliaAppID  = "79FRDP12PN"
	rtAlgoliaAPIKey = "175588f6e5f8319b27702e4cc4013571"
	rtAlgoliaIndex  = "content_rt"
)

type RatingsService struct {
	db     *models.DB
	cache  *cache.Cache
	client *http.Client
}

type RatingsResult struct {
	RottenTomatoes  *int   `json:"rottenTomatoes,omitempty"`
	RTAudienceScore *int   `json:"rtAudienceScore,omitempty"`
	RTCertified     bool   `json:"rtCertified,omitempty"`
	IMDB            string `json:"imdb,omitempty"`
	Metacritic      *int   `json:"metacritic,omitempty"`
}

func NewRatingsService(db *models.DB, cache *cache.Cache) *RatingsService {
	return &RatingsService{
		db:    db,
		cache: cache,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *RatingsService) GetRatings(title, year, mediaType, imdbID string, tmdbID int) (*RatingsResult, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("ratings_%s_%s_%s", title, year, mediaType)
	if cached, found := s.cache.Get(cacheKey); found {
		return cached.(*RatingsResult), nil
	}

	result := &RatingsResult{}

	// Try MDBList first (best source)
	mdblistKey := s.db.GetSetting("mdblist_api_key")
	if mdblistKey != "" {
		mdbResult, err := s.getMDBListRatings(mdblistKey, imdbID, tmdbID, mediaType)
		if err == nil && mdbResult != nil {
			result = mdbResult
		}
	}

	// Fallback to RT Algolia if no RT data
	if result.RottenTomatoes == nil && title != "" {
		rtResult, err := s.getRTRatings(title, year, mediaType)
		if err == nil && rtResult != nil {
			if result.RottenTomatoes == nil {
				result.RottenTomatoes = rtResult.RottenTomatoes
			}
			if result.RTAudienceScore == nil {
				result.RTAudienceScore = rtResult.RTAudienceScore
			}
			if rtResult.RTCertified {
				result.RTCertified = rtResult.RTCertified
			}
		}
	}

	// Cache the result
	s.cache.Set(cacheKey, result)

	return result, nil
}

func (s *RatingsService) getMDBListRatings(apiKey, imdbID string, tmdbID int, mediaType string) (*RatingsResult, error) {
	params := url.Values{}
	params.Set("apikey", apiKey)

	if imdbID != "" {
		params.Set("i", imdbID)
	} else if tmdbID > 0 {
		params.Set("tm", fmt.Sprintf("%d", tmdbID))
		if mediaType == "tv" || mediaType == "series" {
			params.Set("m", "show")
		} else {
			params.Set("m", "movie")
		}
	} else {
		return nil, fmt.Errorf("no ID provided")
	}

	resp, err := s.client.Get("https://mdblist.com/api/?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("MDBList returned %d", resp.StatusCode)
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if data["error"] != nil {
		return nil, fmt.Errorf("MDBList error")
	}

	result := &RatingsResult{}

	if ratings, ok := data["ratings"].([]interface{}); ok {
		for _, r := range ratings {
			rating := r.(map[string]interface{})
			source, _ := rating["source"].(string)
			value, _ := rating["value"].(float64)

			switch source {
			case "imdb":
				result.IMDB = fmt.Sprintf("%.1f", value)
			case "tomatoes":
				v := int(value)
				result.RottenTomatoes = &v
			case "tomatoesaudience":
				v := int(value)
				result.RTAudienceScore = &v
			case "metacritic":
				v := int(value)
				result.Metacritic = &v
			}
		}
	}

	return result, nil
}

func (s *RatingsService) getRTRatings(title, year, mediaType string) (*RatingsResult, error) {
	searchQuery := title
	if year != "" {
		searchQuery = title + " " + year
	}

	typeFilter := "isEmsSearchable:true"
	if mediaType == "movie" {
		typeFilter += " AND type:movie"
	} else {
		typeFilter += " AND type:tvSeries"
	}

	payload := map[string]interface{}{
		"query":       searchQuery,
		"filters":     typeFilter,
		"hitsPerPage": 5,
	}

	jsonData, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", fmt.Sprintf("https://%s-dsn.algolia.net/1/indexes/%s/query", rtAlgoliaAppID, rtAlgoliaIndex), bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-algolia-api-key", rtAlgoliaAPIKey)
	req.Header.Set("x-algolia-application-id", rtAlgoliaAppID)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("RT Algolia returned %d", resp.StatusCode)
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	hits, ok := data["hits"].([]interface{})
	if !ok || len(hits) == 0 {
		return nil, nil
	}

	// Find best match
	var bestMatch map[string]interface{}
	for _, h := range hits {
		hit := h.(map[string]interface{})
		hitTitle, _ := hit["title"].(string)
		hitYear, _ := hit["releaseYear"].(float64)

		if hitTitle == title {
			if year != "" && hitYear > 0 && fmt.Sprintf("%.0f", hitYear) == year {
				bestMatch = hit
				break
			} else if bestMatch == nil {
				bestMatch = hit
			}
		} else if bestMatch == nil {
			bestMatch = hit
		}
	}

	if bestMatch == nil {
		return nil, nil
	}

	result := &RatingsResult{}

	if rtData, ok := bestMatch["rottenTomatoes"].(map[string]interface{}); ok {
		if criticsScore, ok := rtData["criticsScore"].(float64); ok {
			v := int(criticsScore)
			result.RottenTomatoes = &v
		}
		if audienceScore, ok := rtData["audienceScore"].(float64); ok {
			v := int(audienceScore)
			result.RTAudienceScore = &v
		}
		if certified, ok := rtData["certifiedFresh"].(bool); ok {
			result.RTCertified = certified
		}
	}

	return result, nil
}
