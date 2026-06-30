package action

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/prabaltripathiofficial/edict/internal/event"
	"github.com/prabaltripathiofficial/edict/internal/rule"
)

func init() {
	Register("webhook", buildWebhook)
}

// sharedClient is reused across webhook actions so connections pool rather than
// a new transport per action. The timeout bounds a single attempt; the
// dispatcher owns the overall retry budget.
var sharedClient = &http.Client{Timeout: 10 * time.Second}

// webhookAction POSTs (or uses a configured method) a JSON payload describing
// the firing to an external URL.
type webhookAction struct {
	url     string
	method  string
	headers map[string]string
}

func buildWebhook(spec rule.ActionSpec) (Action, error) {
	if spec.URL == "" {
		return nil, fmt.Errorf("webhook action requires a url")
	}
	if _, err := url.ParseRequestURI(spec.URL); err != nil {
		return nil, fmt.Errorf("webhook action has invalid url %q: %w", spec.URL, err)
	}
	method := spec.Method
	if method == "" {
		method = http.MethodPost
	}
	return &webhookAction{url: spec.URL, method: method, headers: spec.Headers}, nil
}

func (a *webhookAction) Type() string { return "webhook" }

// webhookPayload is the body delivered to the endpoint. It is intentionally a
// stable, self-describing envelope so receivers can route on rule/operation.
type webhookPayload struct {
	RuleID    string            `json:"ruleId"`
	Operation event.Operation   `json:"operation"`
	Database  string            `json:"database"`
	Coll      string            `json:"collection"`
	Event     event.ChangeEvent `json:"event"`
}

func (a *webhookAction) Execute(ctx context.Context, ev event.ChangeEvent, r rule.Rule) error {
	body, err := json.Marshal(webhookPayload{
		RuleID:    r.ID,
		Operation: ev.Operation,
		Database:  ev.Database,
		Coll:      ev.Collection,
		Event:     ev,
	})
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, a.method, a.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range a.headers {
		req.Header.Set(k, v)
	}

	resp, err := sharedClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request: %w", err)
	}
	defer resp.Body.Close()

	// 2xx is success; everything else is retryable from the dispatcher's view.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}
