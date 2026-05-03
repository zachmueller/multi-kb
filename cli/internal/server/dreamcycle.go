package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagent"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime/document"
	bratypes "github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime/types"
	"github.com/zmueller/multi-kb/internal/bedrock"
	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/dreamcycle"
)

// RunDreamCycle executes the server-mode dream cycle with Bedrock Retrieve API-backed phases.
func RunDreamCycle(ctx context.Context, cfg *config.Config) error {
	slog.Info("dream cycle: starting")

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(regionOrDefault(cfg)),
	)
	if err != nil {
		return fmt.Errorf("dream cycle: load AWS config: %w", err)
	}

	// Phase 0: Sync CodeCommit→S3, trigger ingestion job
	slog.Info("dream cycle: phase 0 — sync and reindex")
	if err := SyncToS3(ctx, cfg); err != nil {
		slog.Warn("dream cycle: phase 0 S3 sync failed, continuing", "error", err)
	}
	if err := triggerAndWaitIngestion(ctx, awsCfg, cfg); err != nil {
		slog.Warn("dream cycle: phase 0 ingestion timeout or error, continuing best-effort", "error", err)
	}

	braClient := bedrockagentruntime.NewFromConfig(awsCfg)
	kbID := cfg.BedrockKB.KnowledgeBaseID

	// Phase 1: Query for pending notes via Retrieve API, group into batches
	slog.Info("dream cycle: phase 1 — find pending notes")
	pendingNotes, err := queryRetrievePending(ctx, braClient, kbID)
	if err != nil {
		return fmt.Errorf("dream cycle: phase 1: %w", err)
	}
	if len(pendingNotes) == 0 {
		slog.Info("dream cycle: no pending notes found")
		return nil
	}

	batches := groupIntoBatches(ctx, cfg, pendingNotes)
	slog.Info("dream cycle: phase 1 complete", "pending", len(pendingNotes), "batches", len(batches))

	// Phase 2: For each batch, query for related active notes via Retrieve API
	slog.Info("dream cycle: phase 2 — find related notes")
	for i := range batches {
		related, err := queryRetrieveRelated(ctx, braClient, kbID, batches[i])
		if err != nil {
			slog.Warn("dream cycle: phase 2 error", "batch", i, "error", err)
			continue
		}
		batches[i].RelatedNotes = related
	}

	// Phase 3: LLM consolidation (shared logic with local mode)
	slog.Info("dream cycle: phase 3 — LLM consolidation")
	bedrockClient := bedrock.NewClientFromConfig(awsCfg, cfg.DreamCycle.ModelID)

	store := &serverNoteStore{repoDir: repoDir}
	for i, batch := range batches {
		actionCounts, err := dreamcycle.ConsolidateBatch(ctx, bedrockClient, store, batch)
		if err != nil {
			slog.Error("dream cycle: phase 3 error", "batch", i, "error", err)
			continue
		}
		slog.Info("dream cycle: batch consolidated", "batch", i, "actions", actionCounts)
	}

	// Phase 4: Final S3 sync + reindex
	slog.Info("dream cycle: phase 4 — final sync and reindex")
	if err := gitCmd(ctx, repoDir, "push"); err != nil {
		slog.Warn("dream cycle: phase 4 push failed", "error", err)
	}
	if err := SyncToS3(ctx, cfg); err != nil {
		slog.Warn("dream cycle: phase 4 S3 sync failed", "error", err)
	}
	if err := triggerAndWaitIngestion(ctx, awsCfg, cfg); err != nil {
		slog.Warn("dream cycle: phase 4 ingestion timeout", "error", err)
	}

	slog.Info("dream cycle: complete")
	return nil
}

func triggerAndWaitIngestion(ctx context.Context, awsCfg aws.Config, cfg *config.Config) error {
	client := bedrockagent.NewFromConfig(awsCfg)

	result, err := client.StartIngestionJob(ctx, &bedrockagent.StartIngestionJobInput{
		KnowledgeBaseId: aws.String(cfg.BedrockKB.KnowledgeBaseID),
		DataSourceId:    aws.String(cfg.BedrockKB.DataSourceID),
	})
	if err != nil {
		return fmt.Errorf("start ingestion job: %w", err)
	}

	jobId := aws.ToString(result.IngestionJob.IngestionJobId)
	slog.Info("ingestion job started", "job_id", jobId)

	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		status, err := client.GetIngestionJob(ctx, &bedrockagent.GetIngestionJobInput{
			KnowledgeBaseId: aws.String(cfg.BedrockKB.KnowledgeBaseID),
			DataSourceId:    aws.String(cfg.BedrockKB.DataSourceID),
			IngestionJobId:  aws.String(jobId),
		})
		if err != nil {
			return fmt.Errorf("get ingestion job: %w", err)
		}

		switch status.IngestionJob.Status {
		case "COMPLETE":
			slog.Info("ingestion job complete", "job_id", jobId)
			return nil
		case "FAILED":
			reason := "unknown"
			if len(status.IngestionJob.FailureReasons) > 0 {
				reason = status.IngestionJob.FailureReasons[0]
			}
			return fmt.Errorf("ingestion job failed: %s", reason)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(15 * time.Second):
		}
	}

	return fmt.Errorf("ingestion job timed out after 10 minutes")
}

func queryRetrievePending(ctx context.Context, client *bedrockagentruntime.Client, kbID string) ([]dreamcycle.Note, error) {
	filter := &bratypes.RetrievalFilterMemberEquals{
		Value: bratypes.FilterAttribute{
			Key:   aws.String("status"),
			Value: document.NewLazyDocument("pending"),
		},
	}
	return retrieveNotes(ctx, client, kbID, "*", filter, 100)
}

func queryRetrieveRelated(ctx context.Context, client *bedrockagentruntime.Client, kbID string, batch dreamcycle.Batch) ([]dreamcycle.Note, error) {
	filter := &bratypes.RetrievalFilterMemberEquals{
		Value: bratypes.FilterAttribute{
			Key:   aws.String("status"),
			Value: document.NewLazyDocument("active"),
		},
	}
	return retrieveNotes(ctx, client, kbID, batch.PendingNote.Content, filter, 10)
}

func retrieveNotes(ctx context.Context, client *bedrockagentruntime.Client, kbID, queryText string, filter bratypes.RetrievalFilter, limit int32) ([]dreamcycle.Note, error) {
	var allNotes []dreamcycle.Note
	var nextToken *string

	for {
		out, err := client.Retrieve(ctx, &bedrockagentruntime.RetrieveInput{
			KnowledgeBaseId: aws.String(kbID),
			RetrievalQuery:  &bratypes.KnowledgeBaseQuery{Text: aws.String(queryText)},
			RetrievalConfiguration: &bratypes.KnowledgeBaseRetrievalConfiguration{
				VectorSearchConfiguration: &bratypes.KnowledgeBaseVectorSearchConfiguration{
					NumberOfResults: aws.Int32(limit),
					Filter:          filter,
				},
			},
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("retrieve: %w", err)
		}

		notes := parseRetrieveResults(out.RetrievalResults)
		allNotes = append(allNotes, notes...)

		if out.NextToken == nil || int32(len(allNotes)) >= limit {
			break
		}
		nextToken = out.NextToken
	}

	if int32(len(allNotes)) > limit {
		allNotes = allNotes[:limit]
	}
	return allNotes, nil
}

func parseRetrieveResults(results []bratypes.KnowledgeBaseRetrievalResult) []dreamcycle.Note {
	var notes []dreamcycle.Note
	for _, r := range results {
		uid := docString(r.Metadata, "uid")
		if uid == "" {
			continue
		}
		content := ""
		if r.Content != nil && r.Content.Text != nil {
			content = *r.Content.Text
		}
		notes = append(notes, dreamcycle.Note{
			UID:     uid,
			Title:   docString(r.Metadata, "title"),
			Content: content,
			Status:  docString(r.Metadata, "status"),
			Author:  docString(r.Metadata, "author"),
		})
	}
	return notes
}

func docString(metadata map[string]document.Interface, key string) string {
	v, ok := metadata[key]
	if !ok || v == nil {
		return ""
	}
	b, err := v.MarshalSmithyDocument()
	if err != nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return ""
	}
	return s
}

func groupIntoBatches(ctx context.Context, cfg *config.Config, pendingNotes []dreamcycle.Note) []dreamcycle.Batch {
	batches := make([]dreamcycle.Batch, len(pendingNotes))
	for i, note := range pendingNotes {
		batches[i] = dreamcycle.Batch{PendingNote: note}
	}
	return batches
}

// serverNoteStore implements dreamcycle.NoteStore for server mode.
type serverNoteStore struct {
	repoDir string
}

func (s *serverNoteStore) ReadNote(uid string) (*dreamcycle.Note, error) {
	path := filepath.Join(s.repoDir, uid+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return dreamcycle.ParseNote(uid, string(data))
}

func (s *serverNoteStore) WriteNote(note dreamcycle.Note) error {
	path := filepath.Join(s.repoDir, note.UID+".md")
	return os.WriteFile(path, []byte(note.ToMarkdown()), 0o600)
}

func (s *serverNoteStore) DeleteNote(uid string) error {
	path := filepath.Join(s.repoDir, uid+".md")
	return os.Remove(path)
}

func (s *serverNoteStore) CommitBatch(message string) error {
	ctx := context.Background()
	if err := gitCmd(ctx, s.repoDir, "add", "-A"); err != nil {
		return err
	}
	return gitCmd(ctx, s.repoDir, "commit", "-m", message)
}
