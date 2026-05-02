package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// KnowledgeBase defines a remote KB endpoint.
type KnowledgeBase struct {
	Name       string `yaml:"name"`
	Endpoint   string `yaml:"endpoint"`
	Auth       string `yaml:"auth"`
	AWSProfile string `yaml:"aws_profile"`
	AWSRegion  string `yaml:"aws_region"`
	Desc       string `yaml:"description"`
}

// Target describes a routing target for a source.
type Target struct {
	KB       string `yaml:"kb"`
	Routing  string `yaml:"routing"`
	Approval string `yaml:"approval"`
}

// Override adds per-harness or harness+persona routing.
type Override struct {
	Harness string   `yaml:"harness"`
	Persona string   `yaml:"persona,omitempty"`
	Targets []Target `yaml:"targets"`
}

// Source describes a watched directory and its routing rules.
type Source struct {
	Directory string     `yaml:"directory"`
	Harnesses []string   `yaml:"harnesses"`
	Targets   []Target   `yaml:"targets"`
	Overrides []Override `yaml:"overrides,omitempty"`
}

// ExtractionConfig holds extraction LLM settings.
type ExtractionConfig struct {
	ModelID    string `yaml:"model_id"`
	AWSProfile string `yaml:"aws_profile"`
	AWSRegion  string `yaml:"aws_region"`
}

// TranslationConfig holds translation LLM settings.
type TranslationConfig struct {
	SummarizationModelID string `yaml:"summarization_model_id"`
}

// DreamCycleConfig holds dream cycle settings.
type DreamCycleConfig struct {
	ModelID  string `yaml:"model_id"`
	Interval string `yaml:"interval,omitempty"`
}

// HookConfig holds hook settings.
type HookConfig struct {
	Timeout string `yaml:"timeout"`
}

// SQSConfig holds server-mode SQS settings.
type SQSConfig struct {
	QueueURL  string `yaml:"queue_url"`
	BatchSize int    `yaml:"batch_size"`
}

// CodeCommitConfig holds server-mode CodeCommit settings.
type CodeCommitConfig struct {
	RepoName string `yaml:"repo_name"`
	Region   string `yaml:"region"`
}

// S3Config holds server-mode S3 settings.
type S3Config struct {
	Bucket string `yaml:"bucket"`
	Region string `yaml:"region"`
}

// OpenSearchConfig holds server-mode OpenSearch settings.
type OpenSearchConfig struct {
	Endpoint string `yaml:"endpoint"`
	Region   string `yaml:"region"`
}

// BedrockKBConfig holds server-mode Bedrock KB settings.
type BedrockKBConfig struct {
	KnowledgeBaseID string `yaml:"knowledge_base_id"`
	DataSourceID    string `yaml:"data_source_id"`
}

// RecallLogConfig holds server-mode recall log schedule.
type RecallLogConfig struct {
	Schedule string `yaml:"schedule"`
}

// Config is the full config.yaml schema.
type Config struct {
	Mode           string          `yaml:"mode"`
	Author         string          `yaml:"author"`
	KnowledgeBases []KnowledgeBase `yaml:"knowledge_bases"`
	Extraction     ExtractionConfig `yaml:"extraction"`
	Translation    TranslationConfig `yaml:"translation"`
	DreamCycle     DreamCycleConfig `yaml:"dream_cycle"`
	Hook           HookConfig      `yaml:"hook"`
	ExclusionRules []string        `yaml:"exclusion_rules"`
	Sources        []Source        `yaml:"sources"`

	// Server-mode extensions (populated when mode == "server")
	SQS          *SQSConfig          `yaml:"sqs,omitempty"`
	CodeCommit   *CodeCommitConfig   `yaml:"codecommit,omitempty"`
	S3           *S3Config           `yaml:"s3,omitempty"`
	OpenSearch   *OpenSearchConfig   `yaml:"opensearch,omitempty"`
	BedrockKB    *BedrockKBConfig    `yaml:"bedrock_kb,omitempty"`
	TickInterval string              `yaml:"tick_interval,omitempty"`
	RecallLog    *RecallLogConfig    `yaml:"recall_log,omitempty"`
}

// DefaultConfigPath returns the default config file path.
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".multi-kb/config.yaml"
	}
	return filepath.Join(home, ".multi-kb", "config.yaml")
}

// Load reads and parses the config file at the given path.
// Returns a parsed Config and any validation errors.
func Load(path string) (*Config, []error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, []error{fmt.Errorf("cannot read config file %q: %w", path, err)}
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, []error{fmt.Errorf("cannot parse config file %q: %w", path, err)}
	}

	// Apply defaults
	if cfg.Hook.Timeout == "" {
		cfg.Hook.Timeout = "8s"
	}
	if cfg.Extraction.ModelID == "" {
		cfg.Extraction.ModelID = "anthropic.claude-sonnet-4-20250514"
	}
	if cfg.DreamCycle.ModelID == "" {
		cfg.DreamCycle.ModelID = "anthropic.claude-sonnet-4-20250514"
	}
	if cfg.Translation.SummarizationModelID == "" {
		cfg.Translation.SummarizationModelID = "anthropic.claude-haiku-4-5-20251001"
	}

	errs := Validate(&cfg)
	if len(errs) > 0 {
		return nil, errs
	}
	return &cfg, nil
}

// KBNames returns the set of valid knowledge base names including local/<name> variants.
func (c *Config) KBNames() map[string]bool {
	names := make(map[string]bool)
	for _, kb := range c.KnowledgeBases {
		names[kb.Name] = true
	}
	return names
}

// validateDuration checks a duration string is parseable and non-empty.
func validateDuration(field, value string) error {
	if value == "" {
		return nil // empty means use default
	}
	if _, err := time.ParseDuration(value); err != nil {
		return fmt.Errorf("%s: invalid duration %q (use format like 5m, 3h, 8s)", field, value)
	}
	return nil
}
