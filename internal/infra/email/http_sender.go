package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"MKK-Luna/internal/config"
)

type HTTPSender struct {
	baseURL string
	client  *http.Client
}

type invitePayload struct {
	Email    string `json:"email"`
	TeamName string `json:"team_name"`
}

func NewHTTPSender(cfg config.EmailConfig) *HTTPSender {
	return &HTTPSender{
		baseURL: cfg.BaseURL,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (s *HTTPSender) SendInvite(ctx context.Context, toEmail, teamName string) error {
	if s.baseURL == "" {
		return errors.New("email base url is empty")
	}
	body, _ := json.Marshal(invitePayload{Email: toEmail, TeamName: teamName})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/send", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("email service error")
	}
	return nil
}
