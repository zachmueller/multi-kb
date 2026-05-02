package bedrock

import (
	"context"
	"time"
)

// RetryWithBackoff retries the function up to maxAttempts times with exponential backoff.
// It retries only when isRetryable returns true for the error.
func RetryWithBackoff[T any](
	ctx context.Context,
	maxAttempts int,
	fn func() (T, error),
) (T, error) {
	var (
		result T
		err    error
	)

	backoff := 1 * time.Second
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, err = fn()
		if err == nil {
			return result, nil
		}

		classified := classifyError(err)
		if !isRetryable(classified) {
			return result, classified
		}

		if attempt == maxAttempts {
			break
		}

		select {
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > 16*time.Second {
			backoff = 16 * time.Second
		}
	}

	var zero T
	return zero, classifyError(err)
}
