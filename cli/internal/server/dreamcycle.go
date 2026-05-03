package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagent"
	"github.com/zmueller/multi-kb/internal/bedrock"
	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/dreamcycle"
)

// RunDreamCycle executes the server-mode dream cycle with OpenSearch-backed phases.
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

	// Phase 1: Query OpenSearch for pending notes, group into batches
	slog.Info("dream cycle: phase 1 — find pending notes")
	pendingNotes, err := queryOpenSearchPending(ctx, cfg)
	if err != nil {
		return fmt.Errorf("dream cycle: phase 1: %w", err)
	}
	if len(pendingNotes) == 0 {
		slog.Info("dream cycle: no pending notes found")
		return nil
	}

	batches := groupIntoBatches(ctx, cfg, pendingNotes)
	slog.Info("dream cycle: phase 1 complete", "pending", len(pendingNotes), "batches", len(batches))

	// Phase 2: For each batch, query OpenSearch for related active notes
	slog.Info("dream cycle: phase 2 — find related notes")
	for i := range batches {
		related, err := queryOpenSearchRelated(ctx, cfg, batches[i])
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

func queryOpenSearchPending(ctx context.Context, cfg *config.Config) ([]dreamcycle.Note, error) {
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"term": map[string]interface{}{
				"AMAZON_BEDROCK_METADATA.status": "pending",
			},
		},
		"size": 100,
	}

	results, err := opensearchQuery(ctx, cfg, query)
	if err != nil {
		return nil, err
	}

	return parseOpenSearchNotes(results), nil
}

func queryOpenSearchRelated(ctx context.Context, cfg *config.Config, batch dreamcycle.Batch) ([]dreamcycle.Note, error) {
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []interface{}{
					map[string]interface{}{
						"term": map[string]interface{}{
							"AMAZON_BEDROCK_METADATA.status": "active",
						},
					},
					map[string]interface{}{
						"more_like_this": map[string]interface{}{
							"fields":            []string{"AMAZON_BEDROCK_TEXT_CHUNK"},
							"like":              batch.PendingNote.Content,
							"min_term_freq":     1,
							"max_query_terms":   25,
							"min_doc_freq":      1,
						},
					},
				},
			},
		},
		"size": 10,
	}

	results, err := opensearchQuery(ctx, cfg, query)
	if err != nil {
		return nil, err
	}

	return parseOpenSearchNotes(results), nil
}

func groupIntoBatches(ctx context.Context, cfg *config.Config, pendingNotes []dreamcycle.Note) []dreamcycle.Batch {
	// Create singleton batches (one pending note per batch)
	// Similarity grouping can be added later
	batches := make([]dreamcycle.Batch, len(pendingNotes))
	for i, note := range pendingNotes {
		batches[i] = dreamcycle.Batch{PendingNote: note}
	}
	return batches
}

func opensearchQuery(ctx context.Context, cfg *config.Config, query interface{}) (map[string]interface{}, error) {
	body, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("opensearch: marshal query: %w", err)
	}

	endpoint := strings.TrimRight(cfg.OpenSearch.Endpoint, "/")
	url := endpoint + "/bedrock-kb-index/_search"

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("opensearch: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Use default AWS credentials for SigV4 signing via the AOSS VPC endpoint
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.OpenSearch.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("opensearch: load AWS config: %w", err)
	}

	creds, err := awsCfg.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("opensearch: get credentials: %w", err)
	}

	if err := signRequest(ctx, req, body, creds, cfg.OpenSearch.Region); err != nil {
		return nil, fmt.Errorf("opensearch: sign request: %w", err)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opensearch: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("opensearch: read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("opensearch: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("opensearch: parse response: %w", err)
	}

	return result, nil
}

func parseOpenSearchNotes(result map[string]interface{}) []dreamcycle.Note {
	var notes []dreamcycle.Note

	hits, ok := result["hits"].(map[string]interface{})
	if !ok {
		return notes
	}
	hitList, ok := hits["hits"].([]interface{})
	if !ok {
		return notes
	}

	for _, hit := range hitList {
		h, ok := hit.(map[string]interface{})
		if !ok {
			continue
		}
		source, ok := h["_source"].(map[string]interface{})
		if !ok {
			continue
		}

		metadata, _ := source["AMAZON_BEDROCK_METADATA"].(map[string]interface{})
		text, _ := source["AMAZON_BEDROCK_TEXT_CHUNK"].(string)

		uid, _ := metadata["uid"].(string)
		title, _ := metadata["title"].(string)
		status, _ := metadata["status"].(string)
		author, _ := metadata["author"].(string)

		if uid == "" {
			continue
		}

		notes = append(notes, dreamcycle.Note{
			UID:     uid,
			Title:   title,
			Content: text,
			Status:  status,
			Author:  author,
		})
	}

	return notes
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
