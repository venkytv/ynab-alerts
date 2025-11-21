package notifier

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PushoverConfig holds credentials for Pushover notifications.
type PushoverConfig struct {
	AppToken string
	UserKey  string
	Device   string
	Endpt    string
}

// NewPushover returns a Pushover notifier.
func NewPushover(cfg PushoverConfig) Notifier {
	if cfg.Endpt == "" {
		cfg.Endpt = "https://api.pushover.net/1/messages.json"
	}
	return &PushoverNotifier{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// PushoverNotifier implements Notifier using the Pushover API.
type PushoverNotifier struct {
	cfg    PushoverConfig
	client *http.Client
}

func (p *PushoverNotifier) Notify(ctx context.Context, subject, message string) error {
	if p.cfg.AppToken == "" || p.cfg.UserKey == "" {
		return errors.New("pushover credentials missing")
	}

	form := url.Values{}
	form.Set("token", p.cfg.AppToken)
	form.Set("user", p.cfg.UserKey)
	form.Set("title", subject)
	form.Set("message", message)
	if p.cfg.Device != "" {
		form.Set("device", p.cfg.Device)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.Endpt, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("pushover returned status %s", resp.Status)
	}
	return nil
}
