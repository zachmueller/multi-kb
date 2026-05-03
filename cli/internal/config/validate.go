package config

import (
	"fmt"
	"regexp"
	"strings"
)

var scheduleRegex = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// Validate returns all validation errors for a parsed Config.
func Validate(cfg *Config) []error {
	var errs []error

	// mode
	if cfg.Mode != "client" && cfg.Mode != "server" {
		errs = append(errs, fmt.Errorf("mode: must be 'client' or 'server', got %q", cfg.Mode))
	}

	// author
	if strings.TrimSpace(cfg.Author) == "" {
		errs = append(errs, fmt.Errorf("author: must be non-empty"))
	} else if len(cfg.Author) > 100 {
		errs = append(errs, fmt.Errorf("author: must be ≤100 characters"))
	}

	// knowledge_bases: unique names
	kbNames := make(map[string]bool)
	for i, kb := range cfg.KnowledgeBases {
		if kb.Name == "" {
			errs = append(errs, fmt.Errorf("knowledge_bases[%d]: name must be non-empty", i))
			continue
		}
		if kbNames[kb.Name] {
			errs = append(errs, fmt.Errorf("knowledge_bases[%d]: duplicate name %q", i, kb.Name))
		}
		kbNames[kb.Name] = true

		if kb.Auth != "iam" && kb.Auth != "federate" {
			errs = append(errs, fmt.Errorf("knowledge_bases[%d] (%s): auth must be 'iam' or 'federate'", i, kb.Name))
		}
		if kb.Auth == "iam" && kb.AWSProfile == "" {
			errs = append(errs, fmt.Errorf("knowledge_bases[%d] (%s): aws_profile required when auth=iam", i, kb.Name))
		}
	}

	// sources
	for i, src := range cfg.Sources {
		if src.Directory == "" {
			errs = append(errs, fmt.Errorf("sources[%d]: directory must be non-empty", i))
		}
		if len(src.Harnesses) == 0 {
			errs = append(errs, fmt.Errorf("sources[%d]: harnesses must be non-empty", i))
		}
		for _, h := range src.Harnesses {
			if h != "claude-code" && h != "notor" {
				errs = append(errs, fmt.Errorf("sources[%d]: unknown harness %q (must be 'claude-code' or 'notor')", i, h))
			}
		}
		for j, t := range src.Targets {
			errs = append(errs, validateTarget(fmt.Sprintf("sources[%d].targets[%d]", i, j), t, kbNames)...)
		}
		for j, ov := range src.Overrides {
			for k, t := range ov.Targets {
				errs = append(errs, validateTarget(fmt.Sprintf("sources[%d].overrides[%d].targets[%d]", i, j, k), t, kbNames)...)
			}
		}
	}

	// extraction
	if cfg.Extraction.ModelID == "" {
		errs = append(errs, fmt.Errorf("extraction.model_id: must be non-empty"))
	}

	// hook.timeout
	if err := validateDuration("hook.timeout", cfg.Hook.Timeout); err != nil {
		errs = append(errs, err)
	}

	// dream_cycle.interval (optional)
	if err := validateDuration("dream_cycle.interval", cfg.DreamCycle.Interval); err != nil {
		errs = append(errs, err)
	}

	// tick_interval (optional, server mode)
	if err := validateDuration("tick_interval", cfg.TickInterval); err != nil {
		errs = append(errs, err)
	}

	// recall_log.schedule (optional)
	if cfg.RecallLog != nil && cfg.RecallLog.Schedule != "" {
		if !scheduleRegex.MatchString(cfg.RecallLog.Schedule) {
			errs = append(errs, fmt.Errorf("recall_log.schedule: must be HH:MM 24-hour UTC format (e.g. 02:00), got %q", cfg.RecallLog.Schedule))
		}
	}

	// Server-mode required fields — only validated when mode == "server"
	if cfg.Mode == "server" {
		errs = append(errs, validateServerMode(cfg)...)
	}

	return errs
}

func validateServerMode(cfg *Config) []error {
	var errs []error

	if cfg.SQS == nil {
		errs = append(errs, fmt.Errorf("sqs.queue_url: required in server mode"))
		errs = append(errs, fmt.Errorf("sqs.batch_size: required in server mode (must be 1-10)"))
	} else {
		if strings.TrimSpace(cfg.SQS.QueueURL) == "" {
			errs = append(errs, fmt.Errorf("sqs.queue_url: required in server mode"))
		}
		if cfg.SQS.BatchSize < 1 || cfg.SQS.BatchSize > 10 {
			errs = append(errs, fmt.Errorf("sqs.batch_size: must be between 1 and 10, got %d", cfg.SQS.BatchSize))
		}
	}

	if cfg.CodeCommit == nil {
		errs = append(errs, fmt.Errorf("codecommit.repo_name: required in server mode"))
		errs = append(errs, fmt.Errorf("codecommit.region: required in server mode"))
	} else {
		if strings.TrimSpace(cfg.CodeCommit.RepoName) == "" {
			errs = append(errs, fmt.Errorf("codecommit.repo_name: required in server mode"))
		}
		if strings.TrimSpace(cfg.CodeCommit.Region) == "" {
			errs = append(errs, fmt.Errorf("codecommit.region: required in server mode"))
		}
	}

	if cfg.S3 == nil {
		errs = append(errs, fmt.Errorf("s3.bucket: required in server mode"))
		errs = append(errs, fmt.Errorf("s3.region: required in server mode"))
	} else {
		if strings.TrimSpace(cfg.S3.Bucket) == "" {
			errs = append(errs, fmt.Errorf("s3.bucket: required in server mode"))
		}
		if strings.TrimSpace(cfg.S3.Region) == "" {
			errs = append(errs, fmt.Errorf("s3.region: required in server mode"))
		}
	}

	if cfg.OpenSearch == nil {
		errs = append(errs, fmt.Errorf("opensearch.endpoint: required in server mode"))
		errs = append(errs, fmt.Errorf("opensearch.region: required in server mode"))
	} else {
		if strings.TrimSpace(cfg.OpenSearch.Endpoint) == "" {
			errs = append(errs, fmt.Errorf("opensearch.endpoint: required in server mode"))
		}
		if strings.TrimSpace(cfg.OpenSearch.Region) == "" {
			errs = append(errs, fmt.Errorf("opensearch.region: required in server mode"))
		}
	}

	if cfg.BedrockKB == nil {
		errs = append(errs, fmt.Errorf("bedrock_kb: required in server mode"))
	} else {
		if strings.TrimSpace(cfg.BedrockKB.KnowledgeBaseID) == "" {
			errs = append(errs, fmt.Errorf("bedrock_kb.knowledge_base_id: required in server mode"))
		}
		if strings.TrimSpace(cfg.BedrockKB.DataSourceID) == "" {
			errs = append(errs, fmt.Errorf("bedrock_kb.data_source_id: required in server mode"))
		}
	}

	if strings.TrimSpace(cfg.TickInterval) == "" {
		errs = append(errs, fmt.Errorf("tick_interval: required in server mode"))
	}

	if strings.TrimSpace(cfg.DreamCycle.Interval) == "" {
		errs = append(errs, fmt.Errorf("dream_cycle.interval: required in server mode"))
	}

	if strings.TrimSpace(cfg.DreamCycle.ModelID) == "" {
		errs = append(errs, fmt.Errorf("dream_cycle.model_id: required in server mode"))
	}

	if cfg.RecallLog == nil || strings.TrimSpace(cfg.RecallLog.Schedule) == "" {
		errs = append(errs, fmt.Errorf("recall_log.schedule: required in server mode"))
	}

	return errs
}

func validateTarget(field string, t Target, kbNames map[string]bool) []error {
	var errs []error

	if t.KB == "" {
		errs = append(errs, fmt.Errorf("%s: kb must be non-empty", field))
	} else if !strings.HasPrefix(t.KB, "local/") && !kbNames[t.KB] {
		errs = append(errs, fmt.Errorf("%s: kb %q does not reference a known knowledge base", field, t.KB))
	}

	if t.Routing != "always" && t.Routing != "consider" {
		errs = append(errs, fmt.Errorf("%s: routing must be 'always' or 'consider', got %q", field, t.Routing))
	}

	if t.Approval != "auto-approve" && t.Approval != "require-manual-approval" {
		errs = append(errs, fmt.Errorf("%s: approval must be 'auto-approve' or 'require-manual-approval', got %q", field, t.Approval))
	}

	return errs
}
