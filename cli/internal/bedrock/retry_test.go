package bedrock

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestRetryWithBackoff_SuccessFirstAttempt(t *testing.T) {
	calls := 0
	result, err := RetryWithBackoff(context.Background(), 3, func() (string, error) {
		calls++
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Fatalf("expected %q, got %q", "ok", result)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestRetryWithBackoff_RetryOnMalformedOutput(t *testing.T) {
	calls := 0
	result, err := RetryWithBackoff(context.Background(), 3, func() (string, error) {
		calls++
		if calls < 3 {
			return "", fmt.Errorf("%w: bad json", ErrMalformedOutput)
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Fatalf("expected %q, got %q", "ok", result)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestRetryWithBackoff_RetryOnModelTimeout(t *testing.T) {
	calls := 0
	result, err := RetryWithBackoff(context.Background(), 3, func() (string, error) {
		calls++
		if calls < 2 {
			return "", fmt.Errorf("%w: took too long", ErrModelTimeout)
		}
		return "done", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "done" {
		t.Fatalf("expected %q, got %q", "done", result)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestRetryWithBackoff_NoRetryOnNonRetryable(t *testing.T) {
	calls := 0
	_, err := RetryWithBackoff(context.Background(), 3, func() (string, error) {
		calls++
		return "", fmt.Errorf("%w: forbidden", ErrAccessDenied)
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("expected ErrAccessDenied, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call (no retry), got %d", calls)
	}
}

func TestRetryWithBackoff_ExhaustedRetries(t *testing.T) {
	calls := 0
	_, err := RetryWithBackoff(context.Background(), 2, func() (string, error) {
		calls++
		return "", fmt.Errorf("%w: bad output", ErrMalformedOutput)
	})
	if err == nil {
		t.Fatal("expected error after exhausted retries")
	}
	if !errors.Is(err, ErrMalformedOutput) {
		t.Fatalf("expected ErrMalformedOutput, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestRetryWithBackoff_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := RetryWithBackoff(ctx, 5, func() (string, error) {
		calls++
		return "", fmt.Errorf("%w: retry me", ErrMalformedOutput)
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls >= 5 {
		t.Fatalf("expected fewer than 5 calls with cancellation, got %d", calls)
	}
}

func TestClassifyError_Nil(t *testing.T) {
	if classifyError(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestClassifyError_Unknown(t *testing.T) {
	err := errors.New("some random error")
	classified := classifyError(err)
	if classified != err {
		t.Fatalf("expected same error passthrough, got %v", classified)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"model timeout", ErrModelTimeout, true},
		{"malformed output", ErrMalformedOutput, true},
		{"wrapped model timeout", fmt.Errorf("x: %w", ErrModelTimeout), true},
		{"throttled", ErrThrottled, false},
		{"access denied", ErrAccessDenied, false},
		{"sso expired", ErrSSOExpired, false},
		{"service unavailable", ErrServiceUnavail, false},
		{"generic error", errors.New("generic"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isRetryable(tc.err)
			if got != tc.want {
				t.Errorf("isRetryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
