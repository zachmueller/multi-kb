package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// signRequest signs an HTTP request with AWS SigV4 for the "aoss" service.
func signRequest(ctx context.Context, req *http.Request, body []byte, creds aws.Credentials, region string) error {
	service := "aoss"
	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("host", req.Host)
	if creds.SessionToken != "" {
		req.Header.Set("x-amz-security-token", creds.SessionToken)
	}

	payloadHash := sha256Hex(body)
	req.Header.Set("x-amz-content-sha256", payloadHash)

	// Canonical request
	signedHeaders, canonicalHeaders := buildCanonicalHeaders(req)
	canonicalRequest := strings.Join([]string{
		req.Method,
		req.URL.Path,
		req.URL.RawQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, service)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := deriveSigningKey(creds.SecretAccessKey, dateStamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	authHeader := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		creds.AccessKeyID, credentialScope, signedHeaders, signature,
	)
	req.Header.Set("Authorization", authHeader)

	return nil
}

func buildCanonicalHeaders(req *http.Request) (signedHeaders, canonicalHeaders string) {
	headers := make(map[string]string)
	for key := range req.Header {
		lk := strings.ToLower(key)
		if lk == "host" || lk == "content-type" || strings.HasPrefix(lk, "x-amz-") {
			headers[lk] = strings.TrimSpace(req.Header.Get(key))
		}
	}
	headers["host"] = req.Host

	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var canonicalParts []string
	for _, k := range keys {
		canonicalParts = append(canonicalParts, k+":"+headers[k]+"\n")
	}

	return strings.Join(keys, ";"), strings.Join(canonicalParts, "")
}

func deriveSigningKey(secretKey, dateStamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
