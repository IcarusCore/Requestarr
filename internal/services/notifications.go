package services

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/IcarusCore/Requestarr/internal/models"
)

type NotificationService struct {
	db     *models.DB
	client *http.Client
}

func NewNotificationService(db *models.DB) *NotificationService {
	return &NotificationService{
		db: db,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *NotificationService) Send(title, message, url string) {
	// Discord webhook
	discordWebhook := s.db.GetSetting("discord_webhook")
	if discordWebhook != "" {
		s.sendDiscord(discordWebhook, title, message, url)
	}

	// ntfy
	ntfyURL := s.db.GetSetting("ntfy_url")
	ntfyTopic := s.db.GetSetting("ntfy_topic")
	if ntfyURL != "" && ntfyTopic != "" {
		s.sendNtfy(ntfyURL, ntfyTopic, title, message, url)
	}
}

func (s *NotificationService) sendDiscord(webhook, title, message, url string) error {
	embed := map[string]interface{}{
		"title":       title,
		"description": message,
		"color":       5814783,
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"footer":      map[string]string{"text": "Requestarrr"},
	}

	if url != "" {
		embed["url"] = url
	}

	payload := map[string]interface{}{
		"embeds": []interface{}{embed},
	}

	jsonData, _ := json.Marshal(payload)

	resp, err := s.client.Post(webhook, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	resp.Body.Close()

	return nil
}

func (s *NotificationService) sendNtfy(ntfyURL, topic, title, message, url string) error {
	req, err := http.NewRequest("POST", ntfyURL+"/"+topic, bytes.NewBufferString(message))
	if err != nil {
		return err
	}

	req.Header.Set("Title", title)
	if url != "" {
		req.Header.Set("Click", url)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	return nil
}
