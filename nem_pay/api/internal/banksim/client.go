// Package banksim is the api's HTTP client for the bank simulator. It talks to bank-sim over
// HTTP only (bank-sim is a separate service, never imported), and surfaces an authorize timeout
// as context.DeadlineExceeded so the money service can apply its safe "did the bank receive it?"
// policy.
package banksim

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Outcome is the acquirer's decision.
type Outcome string

const (
	Approved Outcome = "approved"
	Declined Outcome = "declined"
)

// Client calls bank-sim with a per-request deadline.
type Client struct {
	baseURL string
	timeout time.Duration
	hc      *http.Client
}

// New builds a client. timeout bounds each call; if bank-sim sleeps past it (tok_timeout), the
// call returns a context deadline error rather than hanging.
func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		timeout: timeout,
		hc:      &http.Client{Timeout: timeout},
	}
}

// AuthorizeRequest is the authorize payload (the token carries the magic outcome).
type AuthorizeRequest struct {
	IntentID uuid.UUID `json:"intent_id"`
	Amount   int64     `json:"amount"`
	Currency string    `json:"currency"`
	Token    string    `json:"token"`
}

type outcomeResponse struct {
	Status string `json:"status"`
}

// Authorize places a hold. A timeout surfaces as an error (wrapping context.DeadlineExceeded via
// the http client), which the caller MUST treat as "unknown" — never as success.
func (c *Client) Authorize(ctx context.Context, req AuthorizeRequest) (Outcome, error) {
	return c.call(ctx, "/authorize", req)
}

// Capture captures a previously authorized hold. Always approved in the simulator.
func (c *Client) Capture(ctx context.Context, intentID uuid.UUID, amount int64) (Outcome, error) {
	return c.call(ctx, "/capture", map[string]any{"intent_id": intentID, "amount": amount})
}

func (c *Client) call(ctx context.Context, path string, payload any) (Outcome, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("banksim marshal: %w", err)
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("banksim request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("banksim %s: %w", path, err) // wraps context.DeadlineExceeded on timeout
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("banksim %s: unexpected status %d", path, resp.StatusCode)
	}
	var out outcomeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("banksim decode: %w", err)
	}
	switch Outcome(out.Status) {
	case Approved:
		return Approved, nil
	case Declined:
		return Declined, nil
	default:
		return "", fmt.Errorf("banksim %s: unknown status %q", path, out.Status)
	}
}
