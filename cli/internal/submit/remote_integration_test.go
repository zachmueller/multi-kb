//go:build integration

package submit

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSubmitToRemoteKB_RealEndpoint(t *testing.T) {
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

	resp, err := SubmitToRemoteKB(ctx, endpoint, "iam", profile, region, RemoteSubmitRequest{
		Title:   "Integration Test Note",
		Content: "This is a test note submitted by the integration test suite.",
		Author:  "integration-test",
	})
	if err != nil {
		if strings.Contains(err.Error(), "credentials") || strings.Contains(err.Error(), "SSO") {
			t.Skipf("skipping: credentials unavailable: %v", err)
		}
		t.Fatalf("SubmitToRemoteKB: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.UID == "" {
		t.Error("expected non-empty UID in response")
	}
	t.Logf("submitted note with UID: %s", resp.UID)
}
