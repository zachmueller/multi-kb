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
