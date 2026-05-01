# Quickstart: Multi-KB CLI Development

**Created:** 2026-05-01
**Plan:** [plan.md](plan.md)

## Prerequisites

- **Go 1.22+** — `go version` should report 1.22 or later
- **git** — required for local KB operations and building
- **AWS CLI v2** — for testing against Bedrock and remote KBs (`aws --version`)
- **AWS account** with Bedrock model access enabled (Claude Sonnet, Claude Haiku, or configured models)

## Repository Setup

```bash
# Clone the repo
git clone <repo-url> multi-kb
cd multi-kb

# Initialize Go module (first time only)
go mod init github.com/<org>/multi-kb
go mod tidy

# Verify build
CGO_ENABLED=0 go build -o multi-kb ./cmd/multi-kb/
./multi-kb --version
```

## Project Structure

```
cmd/multi-kb/main.go       # Entry point
internal/
  cmd/                      # Cobra subcommands
  config/                   # Config + state YAML
  translate/                # Conversation translators
  extract/                  # Extraction sub-agent
  route/                    # Routing engine
  recall/                   # Knowledge recall
  submit/                   # KB submission
  dreamcycle/               # Local dream cycle
  hook/                     # Harness hook logic
  bedrock/                  # AWS Bedrock client
  lock/                     # Lock file
  schedule/                 # Cron registration
  server/                   # Server mode
  approve/                  # Approval web UI
  git/                      # Git operations
  logging/                  # Structured logging
  token/                    # Token counting
```

## Build Commands

```bash
# Development build (current platform)
go build -o multi-kb ./cmd/multi-kb/

# Cross-platform builds
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o multi-kb-linux-amd64 ./cmd/multi-kb/
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o multi-kb-linux-arm64 ./cmd/multi-kb/
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o multi-kb-darwin-amd64 ./cmd/multi-kb/
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o multi-kb-darwin-arm64 ./cmd/multi-kb/
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o multi-kb-windows-amd64.exe ./cmd/multi-kb/

# Run tests
go test ./...

# Run tests with race detector
go test -race ./...

# Run specific package tests
go test ./internal/lock/...
go test ./internal/config/...
```

## Testing Strategy

### Unit Tests
Each `internal/` package has co-located `_test.go` files. Focus on:
- Config/state YAML parsing and validation
- UID generation (format, uniqueness, alphabet)
- Lock file acquisition, heartbeat, stale detection
- Routing rule resolution
- Result interleaving
- Pending queue file lifecycle
- Token counting approximation

### Integration Tests
Require AWS credentials and/or local git repos:
- Bedrock InvokeModel calls (extraction, summarization)
- Remote KB submitKnowledge / recallKnowledge calls
- Git operations (init, commit, grep)
- Cron registration and next-run parsing

Tag integration tests with `//go:build integration` to exclude from `go test ./...`.

### Manual Testing
- Setup wizard flow (interactive terminal)
- Approval web UI (browser)
- Hook injection (requires Claude Code or Notor installation)
- Full end-to-end: setup → process → dream-cycle → hook injection

## Local Development Config

For development, create a test config at `~/.multi-kb/config.yaml`:

```yaml
mode: client
author: "dev-test"

knowledge_bases: []

extraction:
  model_id: "anthropic.claude-sonnet-4-20250514"
  aws_profile: "your-profile"
  aws_region: "us-west-2"

sources:
  - directory: "/path/to/test/project"
    harnesses:
      - claude-code
    targets:
      - kb: local/default
        routing: always
        approval: auto-approve

exclusion_rules: []
```

## Key Dependencies

| Dependency | Purpose |
|------------|---------|
| `github.com/spf13/cobra` | CLI command framework |
| `github.com/charmbracelet/bubbletea` | Terminal UI for setup wizard |
| `github.com/charmbracelet/huh` | Form components for setup wizard |
| `github.com/charmbracelet/lipgloss` | Terminal styling |
| `github.com/aws/aws-sdk-go-v2` | AWS service clients |
| `github.com/aws/aws-sdk-go-v2/service/bedrockruntime` | Bedrock InvokeModel |
| `gopkg.in/yaml.v3` | YAML parsing |
| `github.com/stretchr/testify` | Test assertions |
