package submit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
)

// RemoteSubmitRequest is the body sent to the submitKnowledge endpoint.
type RemoteSubmitRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Author  string `json:"author"`
}

// RemoteSubmitResponse is the 202 response from the submitKnowledge endpoint.
type RemoteSubmitResponse struct {
	UID       string `json:"uid"`
	RequestID string `json:"request_id"`
}

// throttler enforces the 10 req/s self-throttle per KB endpoint.
type throttler struct {
	mu       sync.Mutex
	last     time.Time
	minDelay time.Duration
}

var throttlers sync.Map // map[string]*throttler keyed by endpoint

func getThrottler(endpoint string) *throttler {
	v, _ := throttlers.LoadOrStore(endpoint, &throttler{
		minDelay: 100 * time.Millisecond, // 10 req/s max
	})
	return v.(*throttler)
}

func (t *throttler) wait(ctx context.Context) error {
	t.mu.Lock()
	delay := t.minDelay - time.Since(t.last)
	t.mu.Unlock()

	if delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	t.mu.Lock()
	t.last = time.Now()
	t.mu.Unlock()
	return nil
}

// SubmitToRemoteKB sends a note to a remote KB endpoint, handling auth, throttling, and retries.
// For 400 responses, returns a *ValidationError with the server's error details.
// For 401/403 responses, returns an *AuthError — callers should skip this KB for the run.
func SubmitToRemoteKB(
	ctx context.Context,
	endpoint, auth, awsProfile, awsRegion string,
	req RemoteSubmitRequest,
) (*RemoteSubmitResponse, error) {
	// Pre-flight validation
	if strings.TrimSpace(req.Title) == "" || len(req.Title) > 255 {
		return nil, fmt.Errorf("submit: title must be non-empty and ≤255 chars")
	}
	if strings.TrimSpace(req.Content) == "" || len(req.Content) > 100_000 {
		return nil, fmt.Errorf("submit: content must be non-empty and ≤100,000 chars")
	}
	if strings.TrimSpace(req.Author) == "" || len(req.Author) > 100 {
		return nil, fmt.Errorf("submit: author must be non-empty and ≤100 chars")
	}

	// Throttle
	t := getThrottler(endpoint)
	if err := t.wait(ctx); err != nil {
		return nil, err
	}

	url := strings.TrimRight(endpoint, "/") + "/submitKnowledge"
	body, _ := json.Marshal(req)

	var lastErr error
	backoff := 1 * time.Second

	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := doSubmitRequest(ctx, "POST", url, body, auth, awsProfile, awsRegion)
		if err != nil {
			lastErr = err
			if attempt < 3 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(backoff):
				}
				backoff *= 2
			}
			continue
		}

		switch {
		case resp.StatusCode == 202:
			var result RemoteSubmitResponse
			_ = json.NewDecoder(resp.Body).Decode(&result)
			resp.Body.Close()
			return &result, nil

		case resp.StatusCode == 400:
			errBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, &ValidationError{Body: string(errBody)}

		case resp.StatusCode == 401 || resp.StatusCode == 403:
			errBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, &AuthError{StatusCode: resp.StatusCode, Message: string(errBody)}

		default:
			errBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(errBody))
			if attempt < 3 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(backoff):
				}
				backoff *= 2
			}
		}
	}

	return nil, fmt.Errorf("submit: %s after 3 attempts: %w", url, lastErr)
}

// doSubmitRequest performs an HTTP request, signing with SigV4 for iam auth.
func doSubmitRequest(ctx context.Context, method, url string, body []byte, auth, profile, region string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	if auth == "iam" {
		opts := []func(*config.LoadOptions) error{
			config.WithRegion(region),
		}
		if profile != "" {
			opts = append(opts, config.WithSharedConfigProfile(profile))
		}
		cfg, err := config.LoadDefaultConfig(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("submit: load AWS config: %w", err)
		}
		creds, err := cfg.Credentials.Retrieve(ctx)
		if err != nil {
			return nil, fmt.Errorf("submit: retrieve credentials: %w", err)
		}
		signer := v4.NewSigner()
		h := sha256.Sum256(body)
		bodyHash := fmt.Sprintf("%x", h)
		if err := signer.SignHTTP(ctx, creds, req, bodyHash, "execute-api", region, time.Now()); err != nil {
			return nil, fmt.Errorf("submit: sign request: %w", err)
		}
	}

	return http.DefaultClient.Do(req)
}

// ValidationError is returned when the server responds with HTTP 400.
type ValidationError struct {
	Body string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("submit: validation error from server: %s", e.Body)
}

// AuthError is returned when the server responds with HTTP 401 or 403.
type AuthError struct {
	StatusCode int
	Message    string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("submit: auth error (HTTP %d): %s", e.StatusCode, e.Message)
}
