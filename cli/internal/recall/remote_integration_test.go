//go:build integration

package recall

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRecallFromRemoteKB_RealEndpoint(t *testing.T) {
	endpoint := os.Getenv("MULTI_KB_ENDPOINT")
	if endpoint == "" {
		t.Skip("skipping: MULTI_KB_ENDPOINT not set")
	}

	profile := os.Getenv("MULTI_KB_AWS_PROFILE")
	region := os.Getenv("MULTI_KB_AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results, err := RecallFromRemoteKB(ctx, endpoint, "iam", profile, region,
		"AWS VPC configuration patterns", 5)
	if err != nil {
		if strings.Contains(err.Error(), "credentials") || strings.Contains(err.Error(), "SSO") {
			t.Skipf("skipping: credentials unavailable: %v", err)
		}
		t.Fatalf("RecallFromRemoteKB: %v", err)
	}

	// Results may be empty if KB is empty, which is valid
	t.Logf("received %d results", len(results))

	for i, r := range results {
		if r.UID == "" {
			t.Errorf("result[%d]: missing UID", i)
		}
		if r.Title == "" {
			t.Errorf("result[%d]: missing title", i)
		}
		if r.Score < 0 {
			t.Errorf("result[%d]: negative score %f", i, r.Score)
		}
	}
}
