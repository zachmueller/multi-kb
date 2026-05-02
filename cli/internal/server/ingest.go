package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/zmueller/multi-kb/internal/config"
)

// SQSMessage matches the schema from CDK's submitKnowledge Lambda.
type SQSMessage struct {
	UID         string `json:"uid"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	Author      string `json:"author"`
	SubmittedAt string `json:"submitted_at"`
}

// RunIngestion polls SQS, processes messages, and commits notes to CodeCommit.
func RunIngestion(ctx context.Context, cfg *config.Config) error {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(regionOrDefault(cfg)),
	)
	if err != nil {
		return fmt.Errorf("ingest: load AWS config: %w", err)
	}

	sqsClient := sqs.NewFromConfig(awsCfg)
	batchSize := cfg.SQS.BatchSize
	if batchSize <= 0 || batchSize > 10 {
		batchSize = 10
	}

	result, err := sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(cfg.SQS.QueueURL),
		MaxNumberOfMessages: int32(batchSize),
		WaitTimeSeconds:     5,
	})
	if err != nil {
		return fmt.Errorf("ingest: receive messages: %w", err)
	}

	if len(result.Messages) == 0 {
		slog.Debug("ingest: no messages in queue")
		return nil
	}

	slog.Info("ingest: received messages", "count", len(result.Messages))

	var notes []NoteFile
	var receiptHandles []string

	for _, msg := range result.Messages {
		var sqsMsg SQSMessage
		if err := json.Unmarshal([]byte(aws.ToString(msg.Body)), &sqsMsg); err != nil {
			slog.Warn("ingest: malformed message, skipping", "error", err, "message_id", aws.ToString(msg.MessageId))
			continue
		}

		note := NoteFile{
			UID:         sanitizeField(sqsMsg.UID),
			Title:       sanitizeField(sqsMsg.Title),
			Content:     sqsMsg.Content,
			Author:      sanitizeField(sqsMsg.Author),
			SubmittedAt: sqsMsg.SubmittedAt,
		}
		notes = append(notes, note)
		receiptHandles = append(receiptHandles, aws.ToString(msg.ReceiptHandle))
	}

	if len(notes) == 0 {
		return nil
	}

	// Commit notes to CodeCommit repo
	if err := CommitBatch(ctx, cfg, notes); err != nil {
		return fmt.Errorf("ingest: commit batch: %w", err)
	}

	// Sync to S3
	if err := SyncToS3(ctx, cfg); err != nil {
		slog.Error("ingest: S3 sync failed (will retry next tick)", "error", err)
	}

	// Delete successfully processed messages
	for _, handle := range receiptHandles {
		_, err := sqsClient.DeleteMessage(ctx, &sqs.DeleteMessageInput{
			QueueUrl:      aws.String(cfg.SQS.QueueURL),
			ReceiptHandle: aws.String(handle),
		})
		if err != nil {
			slog.Warn("ingest: failed to delete message", "error", err)
		}
	}

	slog.Info("ingest: batch processed", "notes", len(notes))
	return nil
}

// sanitizeField removes characters unsafe for file names and git commit messages.
func sanitizeField(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == '/' || r == '\\' || r == '\'' || r == '"' || r == '`' {
			return '_'
		}
		return r
	}, s)
	if len(s) > 255 {
		s = s[:255]
	}
	return s
}

func regionOrDefault(cfg *config.Config) string {
	if cfg.S3 != nil && cfg.S3.Region != "" {
		return cfg.S3.Region
	}
	if cfg.CodeCommit != nil && cfg.CodeCommit.Region != "" {
		return cfg.CodeCommit.Region
	}
	return "us-east-1"
}

// NoteFile represents a knowledge note to be written as a Markdown file.
type NoteFile struct {
	UID         string
	Title       string
	Content     string
	Author      string
	SubmittedAt string
}

// ToMarkdown renders the note as a Markdown file with YAML frontmatter.
func (n NoteFile) ToMarkdown() string {
	lastUpdated := n.SubmittedAt
	if lastUpdated == "" {
		lastUpdated = time.Now().UTC().Format(time.RFC3339)
	}

	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "uid: %s\n", n.UID)
	fmt.Fprintf(&b, "title: %q\n", n.Title)
	b.WriteString("status: pending\n")
	fmt.Fprintf(&b, "author: %q\n", n.Author)
	fmt.Fprintf(&b, "last-updated: %s\n", lastUpdated)
	b.WriteString("last-recalled: \"\"\n")
	b.WriteString("consolidated-from-notes: []\n")
	b.WriteString("---\n\n")
	b.WriteString(n.Content)
	b.WriteString("\n")
	return b.String()
}

// Filename returns the file name for this note.
func (n NoteFile) Filename() string {
	return n.UID + ".md"
}
