# multi-kb

A unified toolset for automatically extracting knowledge from AI conversations and sharing it across team and personal knowledge bases.

## Repository Structure

```
multi-kb/
├── cli/                        Go CLI binary (client-mode + server-mode)
│   ├── cmd/                    Cobra command definitions
│   └── internal/               Internal packages
│       ├── config/             Config and state file parsing
│       ├── translate/          Per-harness conversation translators (Notor, Claude Code)
│       ├── extract/            Extraction sub-agent (Bedrock API, prompt management)
│       ├── route/              Note routing logic (always/consider, approval staging)
│       ├── hook/               Harness hook integration (injection at conversation start)
│       ├── recall/             Knowledge recall (local git grep, remote API, result merging)
│       ├── dream/              Local dream cycle logic (phases 0-4)
│       ├── kb/                 Local KB storage (git ops, note CRUD, UID generation)
│       ├── approve/            On-demand web server for approval UI
│       └── schedule/           OS-native scheduler registration (crontab, Task Scheduler)
├── cdk/                        AWS CDK stack (TypeScript)
│   ├── bin/                    CDK app entry point
│   └── lib/                    Stack and construct definitions
│       ├── multi-kb-stack.ts   Main stack (composes all constructs)
│       ├── api.ts              API Gateway + Lambda constructs
│       ├── storage.ts          CodeCommit, S3, OpenSearch Serverless
│       ├── compute.ts          EC2 instance, VPC, security groups
│       └── knowledge-base.ts   Bedrock Knowledge Base configuration
└── specs/                      Specifications by feature, then component
    └── 01-mvp/
        ├── cli/                CLI spec and checklists
        └── cdk/                CDK spec and checklists
```

## Components

### CLI (`cli/`)

A single Go binary (`multi-kb`) that handles conversation scanning, knowledge extraction via LLM, routing to local and remote KBs, hook-based injection into AI conversations, local dream cycle consolidation, and an on-demand approval web UI. Supports both client mode (local) and server mode (EC2) from the same binary. Built with `CGO_ENABLED=0` for fully static cross-platform binaries.

### CDK Infrastructure (`cdk/`)

An AWS CDK (TypeScript) stack that deploys a self-contained knowledge base instance per team: API Gateway with `submitKnowledge`/`recallKnowledge` endpoints, CodeCommit repo, S3 bucket, OpenSearch Serverless collection, Bedrock Knowledge Base, and an EC2 instance running the CLI in server mode.

## License

MIT — see [LICENSE](LICENSE).
