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
	"github.com/zmueller/multi-kb/internal/dreamcycle"
)

type sidecarFile struct {
	MetadataAttributes map[string]sidecarAttribute `json:"metadataAttributes"`
}

type sidecarAttribute struct {
	Value sidecarValue `json:"value"`
}

type sidecarValue struct {
	Type        string `json:"type"`
	StringValue string `json:"stringValue"`
}

func generateSidecar(filename string, content []byte) ([]byte, error) {
	uid := strings.TrimSuffix(filepath.Base(filename), ".md")
	note, err := dreamcycle.ParseNote(uid, string(content))
	if err != nil {
		note = &dreamcycle.Note{UID: uid}
	}

	sc := sidecarFile{
		MetadataAttributes: map[string]sidecarAttribute{
			"status": {Value: sidecarValue{Type: "STRING", StringValue: note.Status}},
			"uid":    {Value: sidecarValue{Type: "STRING", StringValue: note.UID}},
			"title":  {Value: sidecarValue{Type: "STRING", StringValue: note.Title}},
			"author": {Value: sidecarValue{Type: "STRING", StringValue: note.Author}},
		},
	}

	return json.Marshal(sc)
}

// SyncToS3 performs an incremental sync from the CodeCommit repo to S3.
// Uses `git diff` between HEAD~1 and HEAD to determine the changeset.
func SyncToS3(ctx context.Context, cfg *config.Config) error {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.S3.Region),
	)
	if err != nil {
		return fmt.Errorf("s3sync: load AWS config: %w", err)
	}

	s3Client := s3.NewFromConfig(awsCfg)

	// Get list of changed files from latest commit
	out, err := gitOutput(ctx, repoDir, "diff", "--name-status", "HEAD~1", "HEAD")
	if err != nil {
		// If HEAD~1 fails (first commit), sync all tracked files
		out, err = gitOutput(ctx, repoDir, "ls-files")
		if err != nil {
			return fmt.Errorf("s3sync: list files: %w", err)
		}
		return syncAllFiles(ctx, s3Client, cfg.S3.Bucket, out)
	}

	return syncDiff(ctx, s3Client, cfg.S3.Bucket, out)
}

func syncDiff(ctx context.Context, client *s3.Client, bucket, diffOutput string) error {
	lines := strings.Split(strings.TrimSpace(diffOutput), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		status := parts[0]
		filename := parts[1]

		switch {
		case status == "D":
			if err := s3Delete(ctx, client, bucket, filename); err != nil {
				slog.Warn("s3sync: delete failed", "file", filename, "error", err)
			}
		case status == "A" || status == "M" || strings.HasPrefix(status, "R"):
			if strings.HasPrefix(status, "R") && len(parts) >= 3 {
				oldFile := parts[1]
				filename = parts[2]
				if err := s3Delete(ctx, client, bucket, oldFile); err != nil {
					slog.Warn("s3sync: rename delete old failed", "file", oldFile, "error", err)
				}
			}
			if err := s3Upload(ctx, client, bucket, filename); err != nil {
				slog.Warn("s3sync: upload failed", "file", filename, "error", err)
			}
		}
	}
	return nil
}

func syncAllFiles(ctx context.Context, client *s3.Client, bucket, lsOutput string) error {
	lines := strings.Split(strings.TrimSpace(lsOutput), "\n")
	for _, filename := range lines {
		filename = strings.TrimSpace(filename)
		if filename == "" {
			continue
		}
		if err := s3Upload(ctx, client, bucket, filename); err != nil {
			slog.Warn("s3sync: upload failed", "file", filename, "error", err)
		}
	}
	return nil
}

func s3Upload(ctx context.Context, client *s3.Client, bucket, filename string) error {
	path := filepath.Join(repoDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", filename, err)
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		_, lastErr = client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(filename),
			Body:   strings.NewReader(string(data)),
		})
		if lastErr == nil {
			break
		}
		time.Sleep(time.Duration(1<<attempt) * time.Second)
	}
	if lastErr != nil {
		return fmt.Errorf("s3 put %s after 3 attempts: %w", filename, lastErr)
	}

	if strings.HasSuffix(filename, ".md") {
		sidecarData, err := generateSidecar(filename, data)
		if err != nil {
			slog.Warn("s3sync: sidecar generation failed", "file", filename, "error", err)
			return nil
		}
		sidecarKey := filename + ".metadata.json"
		for attempt := 0; attempt < 3; attempt++ {
			_, err = client.PutObject(ctx, &s3.PutObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(sidecarKey),
				Body:   strings.NewReader(string(sidecarData)),
			})
			if err == nil {
				break
			}
			time.Sleep(time.Duration(1<<attempt) * time.Second)
		}
		if err != nil {
			slog.Warn("s3sync: sidecar upload failed", "file", sidecarKey, "error", err)
		}
	}

	return nil
}

func s3Delete(ctx context.Context, client *s3.Client, bucket, filename string) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		_, lastErr = client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(filename),
		})
		if lastErr == nil {
			break
		}
		time.Sleep(time.Duration(1<<attempt) * time.Second)
	}
	if lastErr != nil {
		return fmt.Errorf("s3 delete %s after 3 attempts: %w", filename, lastErr)
	}

	if strings.HasSuffix(filename, ".md") {
		sidecarKey := filename + ".metadata.json"
		for attempt := 0; attempt < 3; attempt++ {
			_, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(sidecarKey),
			})
			if err == nil {
				break
			}
			if attempt == 2 {
				slog.Warn("s3sync: sidecar delete failed", "file", sidecarKey, "error", err)
			}
			time.Sleep(time.Duration(1<<attempt) * time.Second)
		}
	}

	return nil
}
