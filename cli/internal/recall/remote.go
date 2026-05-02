package recall

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
)

// RemoteResult is a single recall result from a remote KB.
type RemoteResult struct {
	UID     string  `json:"uid"`
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// RecallFromRemoteKB calls the recallKnowledge endpoint and returns results.
// Returns partial results if the context times out before completion.
func RecallFromRemoteKB(
	ctx context.Context,
	endpoint, auth, awsProfile, awsRegion string,
	query string,
	limit int,
) ([]RemoteResult, error) {
	url := strings.TrimRight(endpoint, "/") + "/recallKnowledge"

	body, _ := json.Marshal(map[string]interface{}{
		"query": query,
		"limit": limit,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("recall: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if auth == "iam" {
		opts := []func(*config.LoadOptions) error{
			config.WithRegion(awsRegion),
		}
		if awsProfile != "" {
			opts = append(opts, config.WithSharedConfigProfile(awsProfile))
		}
		cfg, err := config.LoadDefaultConfig(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("recall: load AWS config: %w", err)
		}
		creds, err := cfg.Credentials.Retrieve(ctx)
		if err != nil {
			return nil, fmt.Errorf("recall: retrieve credentials: %w", err)
		}
		signer := v4.NewSigner()
		h := sha256.Sum256(body)
		bodyHash := fmt.Sprintf("%x", h)
		if err := signer.SignHTTP(ctx, creds, req, bodyHash, "execute-api", awsRegion, time.Now()); err != nil {
			return nil, fmt.Errorf("recall: sign request: %w", err)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// On context cancellation (timeout), return empty results (partial)
		if ctx.Err() != nil {
			return nil, nil
		}
		return nil, fmt.Errorf("recall: HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 400 {
		body, _ := io.ReadAll(resp.Body)
		// Log warning but return empty (don't inject on bad query)
		fmt.Printf("recall: 400 from remote KB %s: %s\n", endpoint, string(body))
		return nil, nil
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("recall: HTTP %d from %s", resp.StatusCode, endpoint)
	}

	var results []RemoteResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("recall: decode response: %w", err)
	}

	return results, nil
}
