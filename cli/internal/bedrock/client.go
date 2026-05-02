package bedrock

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// Client wraps the Bedrock Runtime client with retry and error classification.
type Client struct {
	inner   *bedrockruntime.Client
	modelID string
}

// NewClient creates a Bedrock client configured for the given profile, region, and model.
// An empty profile uses the default credential chain (useful for EC2 instance role).
func NewClient(ctx context.Context, profile, region, modelID string) (*Client, error) {
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
		// 5 SDK-level attempts: handles ThrottlingException, ServiceUnavailableException,
		// InternalServerException, and network errors automatically.
		config.WithRetryMaxAttempts(5),
		// 5-minute HTTP client timeout — Claude models can take minutes for large contexts.
		config.WithHTTPClient(&http.Client{
			Timeout: 5 * time.Minute,
		}),
	}

	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("bedrock: load AWS config: %w", classifyError(err))
	}

	return &Client{
		inner:   bedrockruntime.NewFromConfig(cfg),
		modelID: modelID,
	}, nil
}

// NewClientFromConfig creates a Bedrock client from an existing aws.Config.
// Used when the caller already has a configured aws.Config (e.g., server mode with instance role).
func NewClientFromConfig(cfg aws.Config, modelID string) *Client {
	return &Client{
		inner:   bedrockruntime.NewFromConfig(cfg),
		modelID: modelID,
	}
}

// InvokeModel sends a system prompt and user message to the model and returns the text response.
// SDK-level retries handle throttling, service failures, and network errors.
// Application-level retry (via RetryWithBackoff) handles malformed JSON output and model timeout.
func (c *Client) InvokeModel(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	return RetryWithBackoff(ctx, 3, func() (string, error) {
		return c.invokeOnce(ctx, systemPrompt, userMessage)
	})
}

// Summarize delegates to InvokeModel, satisfying the translate.LLMSummarizer interface.
func (c *Client) Summarize(ctx context.Context, systemPrompt, userContent string) (string, error) {
	return c.InvokeModel(ctx, systemPrompt, userContent)
}

func (c *Client) invokeOnce(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	reqBody := claudeRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		MaxTokens:        8192,
		System:           systemPrompt,
		Messages: []message{
			{Role: "user", Content: userMessage},
		},
		Temperature: 0.0,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("bedrock: marshal request: %w", err)
	}

	output, err := c.inner.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(c.modelID),
		ContentType: aws.String("application/json"),
		Body:        bodyBytes,
	})
	if err != nil {
		return "", classifyError(err)
	}

	var resp claudeResponse
	if err := json.Unmarshal(output.Body, &resp); err != nil {
		// Malformed response — triggers application-level retry
		return "", fmt.Errorf("%w: %s", ErrMalformedOutput, err.Error())
	}

	var text string
	for _, block := range resp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	if text == "" {
		return "", fmt.Errorf("%w: no text content in response (stop_reason: %s)",
			ErrMalformedOutput, resp.StopReason)
	}

	return text, nil
}
