package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/zmueller/multi-kb/internal/config"
)

// recallLogEntry matches the schema from CDK's recallKnowledge Lambda.
type recallLogEntry struct {
	Timestamp   string   `json:"timestamp"`
	Query       string   `json:"query"`
	RecalledUIDs []string `json:"recalled_uids"`
}

// RunRecallLogProcessing processes the previous day's recall logs from S3,
// updating last-recalled timestamps in the CodeCommit notes.
func RunRecallLogProcessing(ctx context.Context, cfg *config.Config) error {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.S3.Region),
	)
	if err != nil {
		return fmt.Errorf("recall-log: load AWS config: %w", err)
	}

	s3Client := s3.NewFromConfig(awsCfg)

	// Process previous day's logs
	yesterday := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
	prefix := fmt.Sprintf("recall-logs/%s/", yesterday)

	slog.Info("recall-log: scanning", "prefix", prefix)

	paginator := s3.NewListObjectsV2Paginator(s3Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(cfg.S3.Bucket),
		Prefix: aws.String(prefix),
	})

	recalledUIDs := make(map[string]bool)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("recall-log: list objects: %w", err)
		}

		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)
			if !strings.HasSuffix(key, ".json") {
				continue
			}

			entry, err := readRecallLog(ctx, s3Client, cfg.S3.Bucket, key)
			if err != nil {
				slog.Warn("recall-log: read failed", "key", key, "error", err)
				continue
			}

			for _, uid := range entry.RecalledUIDs {
				recalledUIDs[uid] = true
			}
		}
	}

	if len(recalledUIDs) == 0 {
		slog.Info("recall-log: no recalled UIDs to update")
		return nil
	}

	slog.Info("recall-log: updating notes", "uids", len(recalledUIDs))

	now := time.Now().UTC().Format(time.RFC3339)
	updated := 0

	for uid := range recalledUIDs {
		notePath := filepath.Join(repoDir, uid+".md")
		if _, err := os.Stat(notePath); os.IsNotExist(err) {
			slog.Debug("recall-log: note not found, skipping", "uid", uid)
			continue
		}

		data, err := os.ReadFile(notePath)
		if err != nil {
			slog.Warn("recall-log: read note failed", "uid", uid, "error", err)
			continue
		}

		content := string(data)
		updatedContent := updateLastRecalled(content, now)
		if updatedContent == content {
			continue
		}

		if err := os.WriteFile(notePath, []byte(updatedContent), 0o644); err != nil {
			slog.Warn("recall-log: write note failed", "uid", uid, "error", err)
			continue
		}
		updated++
	}

	if updated == 0 {
		return nil
	}

	// Commit all last-recalled updates
	if err := gitCmd(ctx, repoDir, "add", "-A"); err != nil {
		return fmt.Errorf("recall-log: git add: %w", err)
	}

	msg := fmt.Sprintf("recall-log: updated last-recalled for %d note(s) from %s", updated, yesterday)
	if err := gitCmd(ctx, repoDir, "commit", "-m", msg); err != nil {
		return fmt.Errorf("recall-log: git commit: %w", err)
	}

	if err := gitCmd(ctx, repoDir, "push"); err != nil {
		return fmt.Errorf("recall-log: git push: %w", err)
	}

	// Sync to S3
	if err := SyncToS3(ctx, cfg); err != nil {
		slog.Warn("recall-log: S3 sync failed", "error", err)
	}

	slog.Info("recall-log: complete", "updated", updated, "date", yesterday)
	return nil
}

func readRecallLog(ctx context.Context, client *s3.Client, bucket, key string) (*recallLogEntry, error) {
	result, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()

	var entry recallLogEntry
	if err := json.NewDecoder(result.Body).Decode(&entry); err != nil {
		return nil, fmt.Errorf("parse %s: %w", key, err)
	}

	return &entry, nil
}

// updateLastRecalled replaces the last-recalled frontmatter field value.
func updateLastRecalled(content, timestamp string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "last-recalled:") {
			lines[i] = "last-recalled: " + timestamp
			return strings.Join(lines, "\n")
		}
	}
	return content
}
