# Research: Multi-KB CLI — MVP

**Created:** 2026-05-01
**Plan:** [plan.md](plan.md)
**Status:** Open (findings to be populated during Phase 0)

## R-1: Bubbletea Wizard Pattern ✅

**Question:** Which terminal UI library best handles the multi-step setup wizard flow?

**Options:**
- `bubbletea` + `huh` (Charmbracelet form library)
- `survey` (AlecAivazis/survey)
- Raw `bubbletea` with custom components

**Evaluation Criteria:**
- Multi-step form flow support (branching based on answers)
- Terminal compatibility (macOS Terminal, iTerm2, Windows Terminal, WSL)
- Accessibility (screen readers, keyboard-only navigation)
- Maintained and well-documented

**Prototype Task:** Build a 3-step wizard: (1) select from list, (2) text input, (3) confirmation screen. Evaluate all three options.

**Findings:**

### Option 1: `bubbletea` + `huh` (Charmbracelet) -- RECOMMENDED

#### What is `huh`?

`huh` is a Go library for building interactive terminal forms and prompts, built on top of `bubbletea`. It provides high-level form components (Input, Select, MultiSelect, Confirm, Text, FilePicker, Note) that compose into multi-step wizard flows using a Group/Form abstraction. Each Group is a "page" in the wizard; the Form manages sequential progression through Groups.

- **Module path:** `charm.land/huh/v2`
- **Current version:** v2.0.3 (released March 10, 2025)
- **Status:** Stable (v2 is the current major release)
- **Stars:** 6.8k GitHub stars
- **License:** MIT
- **Parent ecosystem:** Charmbracelet (`bubbletea` at 42k stars)

#### Available Input Types

| Component | Description | Relevant to multi-kb? |
|-----------|-------------|----------------------|
| `Select[T]` | Single selection from a list | **Yes** -- harness selection, auth type, routing mode |
| `MultiSelect[T]` | Multiple selections with optional limit | **Yes** -- harness multi-select (step 1) |
| `Input` | Single-line text input | **Yes** -- directory path, author identity, KB name |
| `Text` | Multi-line text input | Maybe -- exclusion rules |
| `Confirm` | Yes/No prompt | **Yes** -- confirmation screens, "add another KB?" |
| `FilePicker` | Interactive file/directory browser | **Yes** -- directory selection (supports `DirAllowed(true)`) |
| `Note` | Read-only informational card | **Yes** -- summary screens, auto-discovery results |

#### Multi-Step Form Flow (Groups)

Forms are organized as a sequence of Groups. Each Group is one "page" of the wizard. The user progresses through Groups sequentially:

```go
form := huh.NewForm(
    huh.NewGroup(/* Step 1 fields */),
    huh.NewGroup(/* Step 2 fields */),
    huh.NewGroup(/* Step 3 fields */),
)
err := form.Run()
```

Each Group can have its own height, title, description, theme, and visibility function. The Form manages keyboard navigation (Tab/Shift+Tab between fields, Enter to advance).

#### Branching / Conditional Logic

Two mechanisms for branching:

1. **`WithHideFunc` on Groups** -- Dynamically skip entire pages based on prior answers:
   ```go
   huh.NewGroup(
       huh.NewInput().Title("Notor vault path").Value(&notorPath),
   ).WithHideFunc(func() bool {
       // Only show if user selected Notor in the harness multi-select
       return !slices.Contains(selectedHarnesses, "notor")
   })
   ```

2. **`TitleFunc` / `OptionsFunc` on Fields** -- Dynamically change field titles and options based on bound variables:
   ```go
   huh.NewSelect[string]().
       TitleFunc(func() string {
           return fmt.Sprintf("Select province for %s", country)
       }, &country).
       OptionsFunc(func() []huh.Option[string] {
           return getProvincesFor(country)
       }, &country)
   ```

Both mechanisms use closure-based callbacks with dependency tracking (pass the address of the variable to watch). The form re-evaluates these functions whenever the bound variable changes.

#### Validation

Fields accept a `Validate` function that returns an error:

```go
huh.NewInput().
    Title("Directory path").
    Value(&dirPath).
    Validate(func(s string) error {
        if _, err := os.Stat(s); os.IsNotExist(err) {
            return fmt.Errorf("directory does not exist: %s", s)
        }
        return nil
    })
```

Built-in validators: `ValidateNotEmpty()`, `ValidateMinLength(n)`, `ValidateMaxLength(n)`, `ValidateLength(min, max)`, `ValidateOneOf(values...)`.

Errors are displayed inline next to the field. The form prevents advancement until validation passes.

#### Terminal Compatibility

`huh` inherits terminal support from `bubbletea` and `lipgloss`:
- **macOS Terminal.app:** Full support
- **iTerm2:** Full support
- **Windows Terminal:** Full support (bubbletea v2 has explicit Windows support)
- **WSL:** Full support (uses the Windows terminal emulator)
- **SSH sessions:** Supported (Charmbracelet has a dedicated `wish` library for SSH TUIs)
- **Piped stdin/stdout:** NOT supported (requires a TTY) -- this is fine since the wizard is interactive

#### Accessibility

First-class accessible mode:
```go
form.WithAccessible(true)
// Or via environment variable
form.WithAccessible(os.Getenv("ACCESSIBLE") != "")
```

Accessible mode replaces the graphical TUI with standard sequential prompts, providing better screen reader dictation and feedback. This is a form-level setting (one toggle covers all fields).

#### Bubbletea Integration

A `huh.Form` implements `tea.Model` (the bubbletea interface). This means:
- Forms can be embedded inside larger bubbletea applications
- Custom pre/post-step logic can wrap form steps
- The form's `Init()`, `Update()`, `View()` methods are standard bubbletea
- After `form.Run()` completes, all bound variables are populated

This is critical for multi-kb: if any step requires custom logic between form pages (e.g., running auto-discovery after directory input, then showing results), the form can be embedded in a bubbletea app that intercepts completion of specific groups.

#### Realistic 3-Step Wizard Prototype

```go
package main

import (
    "fmt"
    "os"
    "slices"
    "charm.land/huh/v2"
)

func main() {
    // Step 1 outputs
    var harnesses []string

    // Step 2 outputs
    var notorVaultPath string
    var claudeCodeDirs []string

    // Step 3 outputs
    var confirmed bool

    form := huh.NewForm(
        // --- Step 1: Select Harnesses ---
        huh.NewGroup(
            huh.NewNote().
                Title("multi-kb Setup").
                Description("Welcome! Let's configure your knowledge base pipeline.\n\n"+
                    "This wizard will walk you through:\n"+
                    "  1. Selecting your AI harnesses\n"+
                    "  2. Pointing to your project directories\n"+
                    "  3. Confirming auto-discovered chat history"),
            huh.NewMultiSelect[string]().
                Title("Which AI harnesses do you use?").
                Options(
                    huh.NewOption("Notor (Obsidian plugin)", "notor"),
                    huh.NewOption("Claude Code (CLI/IDE)", "claude-code"),
                ).
                Value(&harnesses).
                Validate(func(s []string) error {
                    if len(s) == 0 {
                        return fmt.Errorf("select at least one harness")
                    }
                    return nil
                }),
        ),

        // --- Step 2a: Notor directory (shown only if Notor selected) ---
        huh.NewGroup(
            huh.NewInput().
                Title("Notor: Obsidian vault path").
                Description("Enter the path to your Obsidian vault where Notor is installed.").
                Placeholder("/Users/you/obsidian-vault").
                Value(&notorVaultPath).
                Validate(func(s string) error {
                    info, err := os.Stat(s)
                    if err != nil {
                        return fmt.Errorf("path does not exist: %s", s)
                    }
                    if !info.IsDir() {
                        return fmt.Errorf("path is not a directory: %s", s)
                    }
                    return nil
                }),
        ).WithHideFunc(func() bool {
            return !slices.Contains(harnesses, "notor")
        }),

        // --- Step 2b: Claude Code directories (shown only if Claude Code selected) ---
        huh.NewGroup(
            huh.NewInput().
                Title("Claude Code: Project directory").
                Description("Enter a directory where you use Claude Code.\n"+
                    "(You can add more directories later with `multi-kb add-source`.)").
                Placeholder("/Users/you/my-project").
                Value(&claudeCodeDirs[0]). // simplified; real impl uses a loop
                Validate(func(s string) error {
                    info, err := os.Stat(s)
                    if err != nil {
                        return fmt.Errorf("path does not exist: %s", s)
                    }
                    if !info.IsDir() {
                        return fmt.Errorf("path is not a directory: %s", s)
                    }
                    return nil
                }),
        ).WithHideFunc(func() bool {
            return !slices.Contains(harnesses, "claude-code")
        }),

        // --- Step 3: Confirmation ---
        huh.NewGroup(
            huh.NewNote().
                Title("Summary").
                DescriptionFunc(func() string {
                    summary := "Selected harnesses:\n"
                    for _, h := range harnesses {
                        summary += fmt.Sprintf("  - %s\n", h)
                    }
                    if slices.Contains(harnesses, "notor") {
                        summary += fmt.Sprintf("\nNotor vault: %s\n", notorVaultPath)
                    }
                    if slices.Contains(harnesses, "claude-code") {
                        summary += fmt.Sprintf("\nClaude Code dir: %s\n", claudeCodeDirs[0])
                    }
                    return summary
                }, &harnesses, &notorVaultPath),
            huh.NewConfirm().
                Title("Proceed with this configuration?").
                Affirmative("Yes, continue").
                Negative("No, start over").
                Value(&confirmed),
        ),
    ).WithTheme(huh.ThemeCharm(true))

    if err := form.Run(); err != nil {
        fmt.Fprintf(os.Stderr, "Setup cancelled: %v\n", err)
        os.Exit(1)
    }

    if !confirmed {
        fmt.Println("Setup cancelled.")
        os.Exit(0)
    }

    fmt.Println("Configuration saved!")
}
```

**Note:** For the real multi-kb wizard, the form would be embedded in a bubbletea app to run auto-discovery between Steps 2 and 3 (i.e., after the user inputs directories but before showing the confirmation summary). The bubbletea wrapper intercepts the form's group completion, runs discovery logic, then allows the form to proceed to the summary group.

### Option 2: `survey` (AlecAivazis/survey) -- NOT RECOMMENDED

#### Maintenance Status: ARCHIVED

**The `survey` library is archived and no longer maintained.** The repository was archived on April 19, 2024. The maintainer explicitly recommends `bubbletea` as the alternative.

- **Module path:** `github.com/AlecAivazis/survey/v2`
- **Last release:** v2.3.7 (June 13, 2023)
- **Stars:** 4.1k GitHub stars
- **Status:** ARCHIVED -- no bug fixes, no security patches, no new features

#### Input Types

- `Input` (text entry with suggestions)
- `Multiline` (multi-line text)
- `Password` (masked input)
- `Confirm` (yes/no)
- `Select` (single choice)
- `MultiSelect` (multiple choice)
- `Editor` (launches external editor)

No FilePicker.

#### Multi-Step Flows

`survey` uses two functions: `Ask` (batch of questions) and `AskOne` (single question). For branching, the docs say: "for surveys with complicated branching logic, we recommend that you break out your questions into multiple calls to both of these functions to fit your needs." This means manual imperative branching:

```go
var harness string
survey.AskOne(&survey.Select{Message: "Harness?", Options: opts}, &harness)
if harness == "notor" {
    var path string
    survey.AskOne(&survey.Input{Message: "Vault path?"}, &path)
}
```

This works but produces brittle, deeply nested code for a 9-step wizard.

#### Terminal Compatibility

Supports ANSI terminals on macOS, Linux, and Windows. Does NOT support piped stdin/stdout. Pagination defaults to 7 options (configurable).

#### Why NOT survey

1. **Archived** -- no maintenance, no security patches, no bug fixes
2. **No declarative branching** -- manual imperative flow for a 9-step wizard would be unwieldy
3. **No accessibility mode** -- no screen reader support
4. **No dynamic field updates** -- cannot update options based on prior selections without re-creating prompts
5. **No FilePicker** -- would need a separate library
6. **No Charmbracelet ecosystem integration** -- misses lipgloss styling, spinner, etc.

### Option 3: Raw `bubbletea` (No `huh`) -- NOT RECOMMENDED

#### What Would Be Required

Building the 9-step wizard entirely with raw bubbletea would require:

1. **Custom field components** for each input type (text input, single select, multi-select, confirm, file picker). The `bubbles` companion library provides `textinput` and `list` models, but no multi-select, confirm, or file picker.
2. **Custom group/page navigation** -- managing which "page" of the wizard is active, transitioning between pages, handling back navigation.
3. **Custom validation display** -- rendering inline errors, preventing advancement on validation failure.
4. **Custom focus management** -- Tab/Shift+Tab between fields within a page.
5. **Custom accessibility mode** -- building a fallback sequential prompt mode from scratch.

#### Code Comparison

The bubbletea `isbn-form` example (a simple 2-field form with validation) is ~245 lines of Go. Extrapolating to a 9-step wizard with multi-select, file picker, dynamic options, branching, validation, and accessibility:

| Approach | Estimated Lines | Effort |
|----------|----------------|--------|
| `huh` forms | ~200-400 | Low -- declare fields, groups, and callbacks |
| Raw `bubbletea` | ~1,500-2,500 | High -- build every component, navigation, validation, accessibility |

The raw approach provides maximum control but at 5-8x the code volume and significantly more testing surface. Since `huh` forms ARE bubbletea models, the flexibility of raw bubbletea is always available as an escape hatch when needed.

### Key API Patterns for `huh`

#### Pattern 1: Multi-Step Form with Groups

```go
form := huh.NewForm(
    huh.NewGroup(fields...).Title("Step 1"),
    huh.NewGroup(fields...).Title("Step 2"),
    huh.NewGroup(fields...).Title("Step 3"),
)
err := form.Run()
```

#### Pattern 2: Conditional Group Display

```go
huh.NewGroup(fields...).WithHideFunc(func() bool {
    return !shouldShowThisStep
})
```

#### Pattern 3: Dynamic Field Options

```go
huh.NewSelect[string]().
    OptionsFunc(func() []huh.Option[string] {
        return computeOptionsBasedOnPriorAnswers()
    }, &dependencyVariable)
```

#### Pattern 4: Embedding in Bubbletea (for inter-step logic)

```go
type wizardModel struct {
    form        *huh.Form
    currentStep int
    discovering bool
}

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    form, cmd := m.form.Update(msg)
    if f, ok := form.(*huh.Form); ok {
        m.form = f
        if m.form.State == huh.StateCompleted {
            // Run inter-step logic before creating next form
        }
    }
    return m, cmd
}
```

#### Pattern 5: Standalone Field Execution

```go
// For post-setup subcommands (add-source, add-kb)
var path string
huh.NewInput().Title("Directory path").Value(&path).Run()
```

### Known Limitations and Gotchas

1. **No looping within forms.** If the user needs to add multiple directories (variable count), you cannot loop within a single `huh.Form`. Use either: (a) a fixed maximum number of directory groups with `WithHideFunc`, or (b) break the wizard into multiple sequential forms with a "Add another?" confirm between them.

2. **`WithHideFunc` evaluates on every render.** Keep hide functions cheap (simple boolean checks). Do not perform I/O or expensive computation in hide functions.

3. **FilePicker requires TTY.** The interactive file browser needs a terminal. In CI/headless environments, use `WithAccessible(true)` which falls back to text input.

4. **v2 import path change.** The v2 module uses `charm.land/huh/v2`, not the old `github.com/charmbracelet/huh`. Ensure `go get charm.land/huh/v2`.

5. **Theme functions changed in v2.** Theme constructors now take an `isDark bool` parameter: `huh.ThemeCharm(true)`. The old pointer-based API is gone.

6. **Inter-step logic requires bubbletea embedding.** If you need to run async operations between wizard pages (e.g., auto-discovery after directory input), you must embed the form in a bubbletea program rather than using simple `form.Run()`.

7. **No built-in "list of dynamic items" pattern.** For "add multiple directories" or "add multiple KBs," you need to manage an outer loop yourself.

### Multi-KB Setup Wizard Architecture (Recommended)

Given the 9-step wizard requirements, the recommended architecture is:

```
[Bubbletea Program]
  |
  +-- Phase 1 Form (huh.Form)
  |     Group 1: Welcome + harness multi-select
  |     Group 2: Notor vault path (hidden if Notor not selected)
  |     Group 3: Claude Code dir (hidden if CC not selected)
  |
  +-- Auto-Discovery (async, with spinner)
  |     Scan directories, find chat history, build summary
  |
  +-- Phase 2 Form (huh.Form)
  |     Group 4: Discovery summary (Note) + confirmation
  |     Group 5: Remote KB addition (endpoint, auth, description)
  |     Group 6: "Add another KB?" loop
  |
  +-- Phase 3 Form (huh.Form)
  |     Group 7: Routing rules per directory
  |     Group 8: Approval mode preset
  |     Group 9: Author identity
  |     Group 10: Exclusion rules (optional)
  |
  +-- Hook Registration + Cron Setup (async, with spinner)
  |
  +-- Done (summary output)
```

This splits the wizard into three sequential `huh.Form` instances managed by a parent bubbletea program. The parent runs auto-discovery and hook registration between form phases, displaying a spinner during async operations. Each form phase is a self-contained multi-group form with `WithHideFunc` for branching.

**Decision:**

1. **Use `charm.land/huh/v2` (v2.0.3+) as the primary setup wizard library.** It provides all required input types (MultiSelect, Input, FilePicker, Confirm, Select, Note), declarative branching via `WithHideFunc`, dynamic fields via `OptionsFunc`/`TitleFunc`, built-in validation, and first-class accessibility.

2. **Embed huh forms in a bubbletea program** to handle inter-step logic (auto-discovery, hook registration, cron setup) between form phases. This provides the flexibility of raw bubbletea where needed while keeping form declaration concise.

3. **Do NOT use `survey`.** It is archived, unmaintained, and explicitly recommends bubbletea as a replacement.

4. **Do NOT build raw bubbletea forms.** The 5-8x code overhead is not justified when `huh` provides all required components and is itself a bubbletea model (so raw bubbletea is always available as an escape hatch).

5. **Minimum Go module dependencies for the wizard:**
   - `charm.land/huh/v2` (v2.0.3+) -- forms
   - `charm.land/bubbletea/v2` (v2.0.6+) -- parent program for inter-step logic
   - `charm.land/lipgloss/v2` -- styling (transitive dependency, also used for output formatting)

6. **Split the wizard into 3 sequential form phases** managed by a parent bubbletea model, with async operations (discovery, hook registration) between phases. This avoids the "no looping in forms" limitation and keeps each form phase manageable.

7. **Use `WithAccessible(true)` gated on an env var or `--accessible` flag** for screen reader support.

---

## R-2: Bedrock InvokeModel Go SDK Pattern ✅

**Question:** What is the correct Go SDK v2 pattern for calling Bedrock InvokeModel with Claude models?

**Areas to Investigate:**
- `github.com/aws/aws-sdk-go-v2/service/bedrockruntime` — InvokeModel API
- Request body format for Claude models (Messages API vs. legacy)
- How to specify model ID, system prompt, user content
- Response parsing (JSON body extraction from InvokeModelOutput)
- Credential chain with named SSO profiles (`aws_profile` in config)
- Retry and backoff configuration (SDK-level vs. application-level)

**Prototype Task:** Make a working InvokeModel call that sends a system prompt + user message and parses a JSON array response.

**Findings:**

### 1. SDK Module & Client Setup

#### Go Module Paths

Each AWS service is a separate Go module. The required modules for this project are:

| Module | Import Path | Purpose |
|--------|-------------|---------|
| AWS Config | `github.com/aws/aws-sdk-go-v2/config` | Load credentials, region, profile |
| AWS Core | `github.com/aws/aws-sdk-go-v2/aws` | Shared types (`aws.String()`, `aws.Config`) |
| Bedrock Runtime | `github.com/aws/aws-sdk-go-v2/service/bedrockruntime` | `InvokeModel` client |
| Bedrock Runtime Types | `github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types` | Error types (`ThrottlingException`, etc.) |
| Retry | `github.com/aws/aws-sdk-go-v2/aws/retry` | Custom retry configuration |
| SSO Credentials | `github.com/aws/aws-sdk-go-v2/credentials/ssocreds` | SSO error types (`InvalidTokenError`) |

**Install command:**
```bash
go get github.com/aws/aws-sdk-go-v2/config \
       github.com/aws/aws-sdk-go-v2/aws \
       github.com/aws/aws-sdk-go-v2/service/bedrockruntime \
       github.com/aws/aws-sdk-go-v2/aws/retry \
       github.com/aws/aws-sdk-go-v2/credentials/ssocreds
```

#### Creating a Client with Named SSO Profile

```go
import (
    "context"
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/aws/retry"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

func NewBedrockClient(ctx context.Context, profile, region string) (*bedrockruntime.Client, error) {
    // Build config options
    opts := []func(*config.LoadOptions) error{
        config.WithRegion(region),
    }
    if profile != "" {
        opts = append(opts, config.WithSharedConfigProfile(profile))
    }

    // Configure retry: 5 attempts with 20s max backoff (SDK default backoff is exponential jitter)
    opts = append(opts, config.WithRetryMaxAttempts(5))

    cfg, err := config.LoadDefaultConfig(ctx, opts...)
    if err != nil {
        return nil, fmt.Errorf("load AWS config: %w", err)
    }

    client := bedrockruntime.NewFromConfig(cfg)
    return client, nil
}
```

`config.LoadDefaultConfig` handles all credential resolution automatically, including SSO profiles. When the user specifies `aws_profile: my-sso-profile` in `config.yaml`, the SDK:

1. Reads `~/.aws/config` and finds the `[profile my-sso-profile]` section
2. Detects `sso_session` or `sso_start_url` configuration
3. Loads the cached SSO token from `~/.aws/sso/cache/<sha1-hash>.json`
4. Exchanges the SSO token for temporary IAM credentials via the SSO service
5. Caches those credentials for the session duration

**No special SSO-specific code is needed** -- `config.WithSharedConfigProfile("my-sso-profile")` handles it transparently.

#### Credential Resolution Chain Order

The SDK resolves credentials in this order (first match wins):

1. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`)
2. Explicit `WithCredentialsProvider()` option
3. Shared config file `~/.aws/config` -- SSO, assume role, web identity
4. Shared credentials file `~/.aws/credentials` -- static credentials
5. Container credentials endpoint (`AWS_CONTAINER_CREDENTIALS_ENDPOINT`)
6. EC2 Instance Metadata Service (IMDS)

For client mode (user's machine), resolution typically stops at step 3 (SSO profile) or step 4 (static credentials). For server mode (EC2), resolution reaches step 6 (instance role).

### 2. InvokeModel API

#### Function Signature

```go
func (c *Client) InvokeModel(
    ctx context.Context,
    params *InvokeModelInput,
    optFns ...func(*Options),
) (*InvokeModelOutput, error)
```

#### InvokeModelInput Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ModelId` | `*string` | **Yes** | Model ID or ARN (e.g., `"anthropic.claude-sonnet-4-20250514"`) |
| `Body` | `[]byte` | **Yes** | JSON request body (Claude Messages API format) |
| `ContentType` | `*string` | **Yes** | Must be `"application/json"` |
| `Accept` | `*string` | No | Response MIME type. Default: `"application/json"` |
| `GuardrailIdentifier` | `*string` | No | Guardrail ID (not used by multi-kb) |
| `GuardrailVersion` | `*string` | No | Guardrail version |
| `PerformanceConfigLatency` | enum | No | Performance tier |
| `ServiceTier` | enum | No | Service tier |
| `Trace` | enum | No | Enable Bedrock trace |

#### InvokeModelOutput Fields

| Field | Type | Description |
|-------|------|-------------|
| `Body` | `[]byte` | JSON response body (Claude Messages API response format) |
| `ContentType` | `*string` | Response MIME type |
| `ResultMetadata` | `middleware.Metadata` | Request metadata |

**Key insight:** The model ID is specified in `InvokeModelInput.ModelId`, NOT in the JSON body. The JSON body contains only the Claude Messages API parameters.

### 3. Claude Messages API Format (within Bedrock)

#### Request Body JSON

```json
{
    "anthropic_version": "bedrock-2023-05-31",
    "max_tokens": 4096,
    "system": "You are a knowledge extraction engine. Extract knowledge notes as a JSON array.",
    "messages": [
        {
            "role": "user",
            "content": "Here is the conversation to analyze:\n\n..."
        }
    ],
    "temperature": 0.0,
    "stop_sequences": []
}
```

**`anthropic_version`**: Must be `"bedrock-2023-05-31"`. This is the only supported value for Bedrock. It is different from the direct Anthropic API version string.

#### Request Body Fields

| Field | Type | Required | Default | Constraints |
|-------|------|----------|---------|-------------|
| `anthropic_version` | string | **Yes** | -- | Must be `"bedrock-2023-05-31"` |
| `max_tokens` | int | **Yes** | -- | Model-dependent maximum |
| `messages` | array | **Yes** | -- | 0-2000 messages; roles alternate user/assistant |
| `system` | string or array | No | -- | System prompt (string or `[{type: "text", text: "..."}]`) |
| `temperature` | float | No | 1.0 | Range: 0.0-1.0 |
| `top_p` | float | No | 0.999 | Range: 0.0-1.0 |
| `top_k` | int | No | disabled | Range: 0-500 |
| `stop_sequences` | string[] | No | [] | Max 8191 entries |
| `tools` | array | No | -- | Tool definitions (not used by multi-kb) |
| `tool_choice` | object | No | -- | Tool choice control |
| `anthropic_beta` | string[] | No | -- | Beta feature flags |

**For multi-kb usage:** Set `temperature: 0.0` for deterministic extraction output. Set `max_tokens: 4096` (sufficient for JSON array output). System prompt is passed as a plain string in the `system` field.

#### System Prompt Format

The `system` field accepts either a plain string or an array of content blocks:

```json
// Simple string (recommended for multi-kb)
"system": "You are a knowledge extraction engine..."

// Array format (supports cache control markers)
"system": [
    {"type": "text", "text": "You are a knowledge extraction engine..."}
]
```

**Use the plain string format** -- multi-kb has no need for the array format's cache control features.

#### Messages Array Format

```json
"messages": [
    {
        "role": "user",
        "content": "The translated conversation content goes here..."
    }
]
```

Content can be a plain string (recommended for multi-kb) or an array of content blocks:
```json
"content": [
    {"type": "text", "text": "..."}
]
```

For multi-kb's single-turn extraction calls, a single user message with string content is sufficient.

#### Response Body JSON

```json
{
    "id": "msg_bdrk_01XfDUDYJgAACzvnptvVoYEL",
    "type": "message",
    "role": "assistant",
    "model": "anthropic.claude-sonnet-4-20250514",
    "content": [
        {
            "type": "text",
            "text": "[{\"title\": \"...\", \"content\": \"...\", \"suggested_target_kbs\": [\"...\"]}]"
        }
    ],
    "stop_reason": "end_turn",
    "stop_sequence": null,
    "usage": {
        "input_tokens": 1520,
        "output_tokens": 450
    }
}
```

#### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique message ID |
| `type` | string | Always `"message"` |
| `role` | string | Always `"assistant"` |
| `model` | string | Model that generated the response |
| `content` | array | Content blocks (text, tool_use, etc.) |
| `stop_reason` | string | Why generation stopped |
| `stop_sequence` | string or null | Which stop sequence was hit |
| `usage` | object | Token counts |

#### Stop Reason Values

| Value | Meaning |
|-------|---------|
| `end_turn` | Model finished naturally |
| `max_tokens` | Hit the `max_tokens` limit |
| `stop_sequence` | Hit a custom stop sequence |
| `refusal` | Safety policy triggered |
| `tool_use` | Model invoked a tool |

#### Extracting Text from Response

```go
type claudeResponse struct {
    ID         string          `json:"id"`
    Type       string          `json:"type"`
    Role       string          `json:"role"`
    Model      string          `json:"model"`
    Content    []contentBlock  `json:"content"`
    StopReason string          `json:"stop_reason"`
    Usage      usageInfo       `json:"usage"`
}

type contentBlock struct {
    Type string `json:"type"`
    Text string `json:"text"`
}

type usageInfo struct {
    InputTokens  int `json:"input_tokens"`
    OutputTokens int `json:"output_tokens"`
}

// Parse response and extract text
var resp claudeResponse
if err := json.Unmarshal(output.Body, &resp); err != nil {
    return "", fmt.Errorf("parse response: %w", err)
}

// Extract all text blocks (typically just one)
var text string
for _, block := range resp.Content {
    if block.Type == "text" {
        text += block.Text
    }
}
```

### 4. Go Type Definitions for Request/Response

```go
// claudeRequest represents the Claude Messages API request body for Bedrock.
type claudeRequest struct {
    AnthropicVersion string    `json:"anthropic_version"`
    MaxTokens        int       `json:"max_tokens"`
    System           string    `json:"system,omitempty"`
    Messages         []message `json:"messages"`
    Temperature      float64   `json:"temperature,omitempty"`
    TopP             float64   `json:"top_p,omitempty"`
    StopSequences    []string  `json:"stop_sequences,omitempty"`
}

type message struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

// claudeResponse represents the Claude Messages API response body from Bedrock.
type claudeResponse struct {
    ID         string         `json:"id"`
    Type       string         `json:"type"`
    Role       string         `json:"role"`
    Model      string         `json:"model"`
    Content    []contentBlock `json:"content"`
    StopReason string         `json:"stop_reason"`
    Usage      usageInfo      `json:"usage"`
}

type contentBlock struct {
    Type string `json:"type"`
    Text string `json:"text,omitempty"`
    // Tool use fields omitted -- not used by multi-kb
}

type usageInfo struct {
    InputTokens  int `json:"input_tokens"`
    OutputTokens int `json:"output_tokens"`
}
```

### 5. Complete Working Example: InvokeModel with Claude

```go
package bedrock

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "net/http"
    "time"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/aws/retry"
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
    "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
    brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// Client wraps the Bedrock Runtime client with multi-kb-specific logic.
type Client struct {
    runtime *bedrockruntime.Client
    modelID string
}

// NewClient creates a Bedrock client configured for the given profile, region, and model.
func NewClient(ctx context.Context, profile, region, modelID string) (*Client, error) {
    opts := []func(*config.LoadOptions) error{
        config.WithRegion(region),
        config.WithRetryMaxAttempts(5),
        config.WithHTTPClient(&http.Client{
            Timeout: 5 * time.Minute, // Long timeout for large context windows
        }),
    }
    if profile != "" {
        opts = append(opts, config.WithSharedConfigProfile(profile))
    }

    cfg, err := config.LoadDefaultConfig(ctx, opts...)
    if err != nil {
        return nil, fmt.Errorf("load AWS config (profile=%q, region=%q): %w", profile, region, err)
    }

    runtime := bedrockruntime.NewFromConfig(cfg)

    return &Client{
        runtime: runtime,
        modelID: modelID,
    }, nil
}

// InvokeModel sends a system prompt + user message to Claude and returns the text response.
func (c *Client) InvokeModel(ctx context.Context, systemPrompt, userMessage string, maxTokens int) (string, error) {
    req := claudeRequest{
        AnthropicVersion: "bedrock-2023-05-31",
        MaxTokens:        maxTokens,
        System:           systemPrompt,
        Messages: []message{
            {Role: "user", Content: userMessage},
        },
        Temperature: 0.0,
    }

    body, err := json.Marshal(req)
    if err != nil {
        return "", fmt.Errorf("marshal request: %w", err)
    }

    input := &bedrockruntime.InvokeModelInput{
        ModelId:     aws.String(c.modelID),
        ContentType: aws.String("application/json"),
        Accept:      aws.String("application/json"),
        Body:        body,
    }

    output, err := c.runtime.InvokeModel(ctx, input)
    if err != nil {
        return "", classifyError(err)
    }

    var resp claudeResponse
    if err := json.Unmarshal(output.Body, &resp); err != nil {
        return "", fmt.Errorf("parse response body: %w", err)
    }

    // Extract text from content blocks
    var text string
    for _, block := range resp.Content {
        if block.Type == "text" {
            text += block.Text
        }
    }

    if text == "" {
        return "", fmt.Errorf("empty response (stop_reason=%s)", resp.StopReason)
    }

    return text, nil
}
```

### 6. Retry & Error Handling

#### SDK Built-In Retry

The AWS SDK Go v2 has built-in retry with exponential jitter backoff. Default configuration:

| Setting | Default | Recommended for multi-kb |
|---------|---------|--------------------------|
| Max attempts | 3 | 5 |
| Max backoff | 20 seconds | 20 seconds (keep default) |
| Backoff algorithm | Exponential jitter | Keep default |
| Retryable errors | HTTP 500/502/503/504, throttling codes, connection errors | Keep defaults |

The SDK automatically retries `ThrottlingException`, `ServiceUnavailableException`, `InternalServerException`, and network errors. **No application-level retry is needed for these.**

**Configuration at the config level:**
```go
cfg, err := config.LoadDefaultConfig(ctx,
    config.WithRetryMaxAttempts(5),
)
```

**Configuration at the client level (overrides config):**
```go
client := bedrockruntime.NewFromConfig(cfg, func(o *bedrockruntime.Options) {
    o.Retryer = retry.AddWithMaxAttempts(o.Retryer, 5)
})
```

**Custom retry with specific error codes:**
```go
client := bedrockruntime.NewFromConfig(cfg, func(o *bedrockruntime.Options) {
    o.Retryer = retry.NewStandard(func(so *retry.StandardOptions) {
        so.MaxAttempts = 5
        so.MaxBackoff = 30 * time.Second
    })
})
```

#### Bedrock-Specific Error Types

All error types are in `github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types`:

| Error Type | Category | SDK Auto-Retries? | Multi-kb Handling |
|------------|----------|-------------------|-------------------|
| `ThrottlingException` | Throttle | **Yes** | SDK handles; application-level retry only after SDK exhaustion |
| `ModelTimeoutException` | Timeout | No (not in default list) | Add to retryable or handle in application |
| `ServiceUnavailableException` | Server error | **Yes** (503) | SDK handles |
| `InternalServerException` | Server error | **Yes** (500) | SDK handles |
| `ModelNotReadyException` | Transient | No | Retry with backoff (model loading) |
| `AccessDeniedException` | Auth | No | Do not retry; surface credential error |
| `ValidationException` | Client error | No | Do not retry; fix request |
| `ModelErrorException` | Model failure | No | Log and skip |
| `ResourceNotFoundException` | Not found | No | Invalid model ID; do not retry |
| `ServiceQuotaExceededException` | Quota | No | Log; suggest quota increase |

#### Error Classification Function

```go
import (
    "errors"
    brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
    "github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
)

// Sentinel errors for multi-kb error handling
var (
    ErrThrottled     = errors.New("bedrock: throttled")
    ErrTimeout       = errors.New("bedrock: model timeout")
    ErrAuth          = errors.New("bedrock: authentication/authorization failure")
    ErrValidation    = errors.New("bedrock: validation error")
    ErrModelError    = errors.New("bedrock: model error")
    ErrServerError   = errors.New("bedrock: server error")
    ErrNotFound      = errors.New("bedrock: model not found")
    ErrQuotaExceeded = errors.New("bedrock: quota exceeded")
    ErrSSOExpired    = errors.New("bedrock: SSO token expired -- run 'aws sso login --profile <profile>'")
)

func classifyError(err error) error {
    if err == nil {
        return nil
    }

    // Check for SSO token expiration (wraps deep in credential chain)
    var ssoErr *ssocreds.InvalidTokenError
    if errors.As(err, &ssoErr) {
        return fmt.Errorf("%w: %v", ErrSSOExpired, err)
    }

    // Check Bedrock-specific error types
    var throttleErr *brtypes.ThrottlingException
    if errors.As(err, &throttleErr) {
        return fmt.Errorf("%w: %s", ErrThrottled, throttleErr.ErrorMessage())
    }

    var timeoutErr *brtypes.ModelTimeoutException
    if errors.As(err, &timeoutErr) {
        return fmt.Errorf("%w: %s", ErrTimeout, timeoutErr.ErrorMessage())
    }

    var accessErr *brtypes.AccessDeniedException
    if errors.As(err, &accessErr) {
        return fmt.Errorf("%w: %s", ErrAuth, accessErr.ErrorMessage())
    }

    var validationErr *brtypes.ValidationException
    if errors.As(err, &validationErr) {
        return fmt.Errorf("%w: %s", ErrValidation, validationErr.ErrorMessage())
    }

    var modelErr *brtypes.ModelErrorException
    if errors.As(err, &modelErr) {
        return fmt.Errorf("%w: %s", ErrModelError, modelErr.ErrorMessage())
    }

    var notFoundErr *brtypes.ResourceNotFoundException
    if errors.As(err, &notFoundErr) {
        return fmt.Errorf("%w: %s", ErrNotFound, notFoundErr.ErrorMessage())
    }

    var quotaErr *brtypes.ServiceQuotaExceededException
    if errors.As(err, &quotaErr) {
        return fmt.Errorf("%w: %s", ErrQuotaExceeded, quotaErr.ErrorMessage())
    }

    var serverErr *brtypes.InternalServerException
    if errors.As(err, &serverErr) {
        return fmt.Errorf("%w: %s", ErrServerError, serverErr.ErrorMessage())
    }

    var unavailErr *brtypes.ServiceUnavailableException
    if errors.As(err, &unavailErr) {
        return fmt.Errorf("%w: %s", ErrServerError, unavailErr.ErrorMessage())
    }

    var notReadyErr *brtypes.ModelNotReadyException
    if errors.As(err, &notReadyErr) {
        return fmt.Errorf("%w: %s", ErrServerError, notReadyErr.ErrorMessage())
    }

    // Unknown error -- wrap as-is
    return fmt.Errorf("bedrock: %w", err)
}
```

#### Application-Level Retry (for non-SDK-retried errors)

The SDK handles retry for throttling and server errors automatically. Application-level retry is needed for:

1. **Malformed JSON output** -- the LLM returned invalid JSON; retry with a fresh API call
2. **`ModelTimeoutException`** -- not in the SDK's default retryable set
3. **`ModelNotReadyException`** -- model is loading; retry after delay

```go
import (
    "math"
    "math/rand"
    "time"
)

// RetryWithBackoff retries a function with exponential backoff and jitter.
// The function should return (result, retryable, error).
func RetryWithBackoff[T any](ctx context.Context, maxAttempts int, fn func() (T, bool, error)) (T, error) {
    var zero T
    var lastErr error

    for attempt := 0; attempt < maxAttempts; attempt++ {
        if attempt > 0 {
            backoff := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
            jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
            delay := backoff + jitter
            if delay > 30*time.Second {
                delay = 30 * time.Second
            }

            select {
            case <-ctx.Done():
                return zero, ctx.Err()
            case <-time.After(delay):
            }
        }

        result, retryable, err := fn()
        if err == nil {
            return result, nil
        }
        lastErr = err
        if !retryable {
            return zero, err
        }
    }

    return zero, fmt.Errorf("exhausted %d attempts: %w", maxAttempts, lastErr)
}
```

**Usage for extraction with JSON retry:**
```go
notes, err := RetryWithBackoff(ctx, 3, func() ([]Note, bool, error) {
    text, err := client.InvokeModel(ctx, systemPrompt, userMessage, 4096)
    if err != nil {
        // SDK already retried throttle/server errors.
        // Retry ModelTimeout at application level.
        if errors.Is(err, ErrTimeout) {
            return nil, true, err
        }
        return nil, false, err
    }

    notes, err := parseExtractionOutput(text)
    if err != nil {
        // Malformed JSON -- retry with fresh API call
        return nil, true, fmt.Errorf("malformed extraction output: %w", err)
    }

    return notes, false, nil
})
```

### 7. Credential Chain and SSO

#### SSO Profile Configuration (in `~/.aws/config`)

```ini
[profile my-sso-profile]
sso_session = my-sso-session
sso_account_id = 123456789012
sso_role_name = BedrockAccessRole
region = us-west-2

[sso-session my-sso-session]
sso_start_url = https://my-company.awsapps.com/start
sso_region = us-east-1
sso_registration_scopes = sso:account:access
```

#### SSO Login Requirement

**Yes, the user must run `aws sso login --profile <profile>` before the CLI can make Bedrock calls.** The SDK does not perform the interactive browser-based SSO login flow -- it only reads cached tokens from `~/.aws/sso/cache/`.

If the SSO token is expired, the SDK returns an error that wraps `*ssocreds.InvalidTokenError`. The CLI should catch this and display a user-friendly message:

```
Error: SSO session expired. Please run:
    aws sso login --profile my-sso-profile
```

#### Detecting Expired Credentials

```go
import "github.com/aws/aws-sdk-go-v2/credentials/ssocreds"

func isExpiredCredentials(err error) bool {
    var ssoErr *ssocreds.InvalidTokenError
    if errors.As(err, &ssoErr) {
        return true
    }
    var accessErr *brtypes.AccessDeniedException
    if errors.As(err, &accessErr) {
        return true
    }
    return false
}
```

### 8. HTTP Timeout Configuration

**Critical:** AWS SDK Go v2 defaults to a short HTTP read timeout. For Claude models with large context windows (especially extended thinking), responses can take minutes. The SDK documentation warns:

> "60-minute timeout for Claude 3.7 Sonnet and Claude 4 models. AWS SDKs default to 1-minute timeout. Increase `read_timeout` to at least 3600 seconds in your SDK configuration."

For multi-kb, a 5-minute timeout is reasonable (extraction calls process up to 700K tokens):

```go
import "net/http"

cfg, err := config.LoadDefaultConfig(ctx,
    config.WithHTTPClient(&http.Client{
        Timeout: 5 * time.Minute,
    }),
)
```

### 9. InvokeModel vs. Converse API

AWS recommends the **Converse API** as a unified interface across models. However, for multi-kb's use case, **InvokeModel is the correct choice** because:

1. **Direct control over Claude-specific parameters:** `anthropic_version`, `system` as string, etc.
2. **JSON body passthrough:** The request/response format is well-defined and stable for Claude.
3. **No abstraction overhead:** Converse adds a Go type layer (`types.Message`, `types.ContentBlock`) that adds complexity without value for single-turn extraction calls.
4. **System prompt handling:** InvokeModel passes system prompts directly in the JSON body; Converse uses a separate `System` field with content block types.

The Converse API would be preferable for multi-model support or multi-turn conversations, neither of which multi-kb needs.

### 10. Model ID Format

Model IDs for Bedrock follow specific patterns:

| Model | Model ID |
|-------|----------|
| Claude Sonnet 4 | `anthropic.claude-sonnet-4-20250514` |
| Claude Haiku 3 | `anthropic.claude-haiku-3-20240307` or `anthropic.claude-3-haiku-20240307-v1:0` |
| Claude Sonnet 3.5 v2 | `anthropic.claude-3-5-sonnet-20241022-v2:0` |
| Claude Opus 4 | `anthropic.claude-opus-4-20250514` |

**Cross-region inference profiles** use a `us.` or `eu.` prefix (e.g., `us.anthropic.claude-sonnet-4-20250514`). These are passed as the model ID directly.

The model ID is set in `InvokeModelInput.ModelId`, not in the JSON body.

**Decision:**

1. **Use InvokeModel (not Converse).** InvokeModel gives direct control over the Claude Messages API format, which is simpler for single-turn extraction/summarization/keyword calls. The JSON body format is well-documented and stable.

2. **Use `"bedrock-2023-05-31"` as the `anthropic_version` string.** This is the only supported value for Bedrock and is different from the direct Anthropic API version.

3. **SDK-level retry handles throttling and server errors.** Configure `config.WithRetryMaxAttempts(5)` for a total of 5 attempts. The SDK's built-in exponential jitter backoff is appropriate.

4. **Application-level retry handles malformed JSON and ModelTimeout.** Use a generic `RetryWithBackoff` function with 3 attempts for extraction calls. This catches: (a) LLM returning invalid JSON (retry with fresh call), (b) `ModelTimeoutException` (not in SDK's default retryable set).

5. **SSO profile support via `config.WithSharedConfigProfile()`.** No special SSO-specific code needed. The SDK handles the entire SSO credential flow transparently. Detect expired tokens via `errors.As(err, &ssocreds.InvalidTokenError{})` and surface a user-friendly `aws sso login` message.

6. **HTTP timeout: 5 minutes.** Configure via `config.WithHTTPClient(&http.Client{Timeout: 5 * time.Minute})`. This is generous enough for large extraction calls without risking indefinite hangs.

7. **Error classification with sentinel errors.** Use `errors.As()` to match Bedrock error types and wrap them with sentinel errors for clean upstream handling. The `classifyError` function maps Bedrock types to multi-kb error categories.

8. **Go module dependencies:**
   - `github.com/aws/aws-sdk-go-v2/config` -- credential/region loading
   - `github.com/aws/aws-sdk-go-v2/aws` -- shared types
   - `github.com/aws/aws-sdk-go-v2/service/bedrockruntime` -- InvokeModel client
   - `github.com/aws/aws-sdk-go-v2/aws/retry` -- custom retry (optional, only if overriding per-client)
   - `github.com/aws/aws-sdk-go-v2/credentials/ssocreds` -- SSO error type detection

9. **Request body structure for all four multi-kb use cases:**

   | Use Case | System Prompt | User Message | max_tokens | temperature |
   |----------|---------------|--------------|------------|-------------|
   | Extraction | Hardcoded + exclusion rules + append file | Translated conversation JSONL | 4096 | 0.0 |
   | Translation summarization | "Summarize this tool interaction in 1-2 sentences" | Tool call + result content | 256 | 0.0 |
   | Keyword derivation | "Extract 3-5 search keywords as JSON array" | User's first message | 128 | 0.0 |
   | Dream cycle consolidation | Consolidation prompt from PRM-002 | Pending note + related notes | 4096 | 0.0 |

   All four use cases follow the identical InvokeModel pattern -- only the system prompt, user message, and max_tokens differ.

10. **File placement:** `internal/bedrock/client.go` for the Client struct and InvokeModel wrapper; `internal/bedrock/models.go` for request/response Go types; `internal/bedrock/errors.go` for error classification.

---

## R-3: Claude Code Conversation Format ✅

**Question:** What is the exact schema of Claude Code conversation files?

**Location:** `~/.claude/projects/<project>/<session>.jsonl`

**Areas to Investigate:**
- How `<project>` directory name maps to the user's project path
- JSONL line schema: message roles, content block structure, tool call/result format
- How to identify conversation boundaries (one file = one conversation?)
- Presence/absence of per-message timestamps
- How to detect file modifications for re-processing

**Prototype Task:** Read a real Claude Code conversation file, document the schema, build a parser.

**Findings:**

### Project Directory Naming

The absolute filesystem path is encoded by replacing every `/` with `-`. The result always starts with a leading `-` (since absolute paths start with `/`).

| Filesystem Path | Directory Name |
|---|---|
| `/Volumes/workplace/multi-kb` | `-Volumes-workplace-multi-kb` |
| `/Users/jane/projects/foo` | `-Users-jane-projects-foo` |

**Reverse mapping:** To find the project directory for a user-configured path like `/Volumes/workplace/multi-kb`, replace all `/` with `-` to get `-Volumes-workplace-multi-kb`, then look for `~/.claude/projects/-Volumes-workplace-multi-kb/`.

### Session File Layout

Each project directory contains:
- **`<uuid>.jsonl`** — One file per conversation session. The filename is the session's UUID.
- **`<uuid>/`** — Optional companion directory containing:
  - `subagents/` — JSONL files for Agent sub-conversations (`agent-<id>.jsonl` + `agent-<id>.meta.json`)
  - `tool-results/` — Large tool outputs persisted to disk
- **`memory/`** — Project-level memory directory

**One file = one conversation.** No explicit boundary markers needed.

### JSONL Line Types

Every line is a JSON object with a top-level `type` field:

| `type` | Description | Relevant to translator? |
|---|---|---|
| `user` | User message or tool_result delivery | **Yes** |
| `assistant` | Assistant response (one content block per line) | **Yes** |
| `attachment` | File attachments, tool listings, injected context | Selective |
| `system` | System-level events | No |
| `queue-operation` | Internal queuing metadata | No |
| `permission-mode` | Permission mode changes | No |
| `file-history-snapshot` | File backup state | No |
| `last-prompt` | Truncated last user prompt | No |
| `ai-title` | AI-generated conversation title | No |

### Common Fields on Message Lines

Most message lines share these fields:

```json
{
  "type": "user|assistant|attachment",
  "uuid": "<uuid>",
  "parentUuid": "<uuid> | null",
  "timestamp": "2026-05-01T04:38:24.311Z",
  "sessionId": "<uuid>",
  "cwd": "/Volumes/workplace/multi-kb",
  "version": "2.1.123",
  "entrypoint": "claude-vscode",
  "userType": "external",
  "isSidechain": false,
  "gitBranch": "main"
}
```

### Per-Message Timestamps

**IMPORTANT CHANGE:** Every message line **does** have a `timestamp` field — ISO 8601 with millisecond precision, UTC (`Z`). This contradicts the spec's assumption that "Claude Code's native format lacks reliable per-message timestamps."

**Impact:** The translator can use per-message timestamps for the `previously_processed` flag (same as Notor), rather than the file-level fallback strategy described in the spec. This simplifies re-processing: only messages with timestamps ≤ `last_processed` are flagged `previously_processed: true`.

### User Message Schema (`type: "user"`)

**Human-typed messages:**
```json
{
  "type": "user",
  "promptId": "<uuid>",
  "permissionMode": "default",
  "message": {
    "role": "user",
    "content": [
      { "type": "text", "text": "the user's message" }
    ]
  }
}
```

**Content is always an array of content blocks**, never a bare string. Multiple `text` blocks may exist (e.g., IDE-injected file context alongside user text).

**Tool result messages** (`type: "user"` with `tool_result` content block):
```json
{
  "type": "user",
  "message": {
    "role": "user",
    "content": [
      {
        "type": "tool_result",
        "tool_use_id": "toolu_bdrk_01...",
        "content": "<string or [{type, text}]>",
        "is_error": true
      }
    ]
  },
  "toolUseResult": { ... },
  "sourceToolAssistantUUID": "<uuid>"
}
```

**`toolUseResult`** provides rich metadata beyond the `message.content`:
- `Bash`: `{stdout, stderr, interrupted, ...}`
- `Read`: `{type, file: {filePath, content, numLines, ...}}`
- `Write`/`Edit`: `{filePath, content, structuredPatch, ...}`
- `Agent`: `{status, prompt, agentId, agentType, content, totalDurationMs, totalTokens, ...}`

### Assistant Message Schema (`type: "assistant"`)

**Critical:** A single API response is **split across multiple JSONL lines, one content block per line.** All lines from the same API response share the same `message.id`.

```json
{
  "type": "assistant",
  "message": {
    "model": "claude-opus-4-6",
    "id": "msg_bdrk_013...",
    "role": "assistant",
    "content": [
      { "type": "text", "text": "response text" }
    ],
    "stop_reason": "tool_use|end_turn",
    "usage": { "input_tokens": 3, "output_tokens": 359, ... }
  }
}
```

Content block types:
- `thinking`: `{type: "thinking", thinking: "<text>", signature: "<base64>"}`
- `text`: `{type: "text", text: "<text>"}`
- `tool_use`: `{type: "tool_use", id: "<tool_use_id>", name: "<tool_name>", input: {...}}`

### Attachment Schema (`type: "attachment"`)

Key subtypes:
- `file`: File contents from `@file` references
- `hook_additional_context`: Context injected by hooks
- `deferred_tools_delta`: Tool palette changes
- `skill_listing`: Available slash commands

### Message Threading

Messages form a linked list via `parentUuid` → `uuid`. First message has `parentUuid: null`. The `promptId` groups a user prompt with its responses. `isSidechain: true` indicates branched conversations.

### Subagent Files

Agent tool calls produce companion files under `<session-uuid>/subagents/`:
- `agent-<id>.jsonl` — Sub-conversation (same JSONL format, no queue-operation preamble)
- `agent-<id>.meta.json` — Metadata: `{agentType, description}`

**Decision:**

1. **Per-message timestamps are available and should be used.** The spec's file-level `previously_processed` fallback is unnecessary for Claude Code. The translator should compare each message's `timestamp` to `last_processed`, same as Notor. This change simplifies the translator and improves re-processing precision (only new messages get `previously_processed: false`).

2. **Project directory discovery:** To map a user-configured directory path to the Claude Code project directory, replace all `/` with `-`. No complex path resolution needed.

3. **Translator must reassemble split assistant messages.** Group consecutive `type: "assistant"` lines with the same `message.id` into a single logical assistant message.

4. **Tool call/result pairing:** Match `tool_use` content blocks (on assistant lines) to `tool_result` content blocks (on user lines) via `tool_use_id` ↔ `tool_use_id`. The `toolUseResult` field on the user line provides richer metadata for summarization.

5. **Ignore non-message line types** (`queue-operation`, `permission-mode`, `file-history-snapshot`, `last-prompt`, `ai-title`) during translation. Only process `user`, `assistant`, and selectively `attachment` lines.

6. **Subagent conversations** should be skipped in MVP — they are subsidiary context that would complicate the translator without proportional value. The Agent tool's result is already captured in the parent conversation's tool result.

---

## R-4: Notor Conversation Format ✅

**Question:** What is the exact schema of Notor chat history files?

**Location:** `{vault}/.obsidian/plugins/notor/history/`

**Areas to Investigate:**
- File format (JSON, JSONL, Markdown, other?)
- Message schema (roles, content, timestamps, tool calls)
- Persona/workflow metadata (where stored, how to extract)
- Per-message timestamps (confirmed available per spec — verify format)
- Conversation boundaries

**Prototype Task:** Read a real Notor history directory, document the schema, build a parser.

**Findings:**

### History Directory Location

The default history path is `{vault}/.obsidian/plugins/notor/history/`. This is configurable via the plugin's `history_path` setting in `data.json` (vault-relative). The setting stores a vault-relative path like `.obsidian/plugins/notor/history/`.

**Note:** The spec's placeholder `{vault}/notor/history/` is incorrect. The `{vault}/notor/` directory contains user-facing data (personas, workflows, rules, memory), while history files live under the Obsidian plugin config directory at `{vault}/.obsidian/plugins/notor/history/`.

### File Format: JSONL (one file = one conversation)

Each conversation is a single `.jsonl` file. **One file = one conversation.** No explicit boundary markers needed.

- **Line 1** is always the **conversation header** (`_type: "conversation"`)
- **Lines 2+** are **message records** (`_type: "message"`), appended chronologically

### File Naming Convention

Format: `{timestamp}_{uuid}.jsonl`

The timestamp is derived from the conversation's `created_at` ISO 8601 string by stripping punctuation:
- Input: `2026-03-10T04:15:13.521Z`
- Output: `20260310_041513`

Full example: `20260310_041513_0ecc5e56-6460-41c4-8762-464cec0816e7.jsonl`

Generation logic (from `conversationFilename()` in `src/chat/history.ts`):
```
created_at.replace(/[-:]/g, "").replace("T", "_").replace(/\.\d+Z$/, "Z").replace("Z", "")
```

**Sub-agent files** follow a different convention: `{parent_timestamp}_{parent_id}_subagent_{invocation_id}.jsonl`. These can be identified by containing `_subagent_` in the filename.

### Conversation Header Schema (Line 1)

```json
{
  "_type": "conversation",
  "id": "0ecc5e56-6460-41c4-8762-464cec0816e7",
  "created_at": "2026-03-10T04:15:13.521Z",
  "updated_at": "2026-03-10T04:15:46.659Z",
  "provider_id": "bedrock",
  "model_id": "global.anthropic.claude-sonnet-4-6",
  "total_input_tokens": 20275,
  "total_output_tokens": 509,
  "estimated_cost": 0.06846,
  "mode": "plan",
  "title": "Is there a pandas equivalent in the world of TypeScript?"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `_type` | `"conversation"` | Yes | Discriminator — always `"conversation"` for headers |
| `id` | string (UUID v4) | Yes | Unique conversation identifier |
| `created_at` | string (ISO 8601) | Yes | Conversation creation timestamp |
| `updated_at` | string (ISO 8601) | Yes | Last activity timestamp (updated on each message) |
| `provider_id` | string | Yes | LLM provider type (e.g., `"bedrock"`, `"anthropic"`, `"openai"`, `"local"`) |
| `model_id` | string | Yes | Model identifier (e.g., `"global.anthropic.claude-sonnet-4-6"`) |
| `total_input_tokens` | number | Yes | Cumulative input tokens |
| `total_output_tokens` | number | Yes | Cumulative output tokens |
| `estimated_cost` | number \| null | Yes | Cumulative estimated cost (null if pricing unavailable) |
| `mode` | `"plan"` \| `"act"` | Yes | Conversation mode at time of last update |
| `title` | string \| undefined | No | Display title (auto-generated from first user message) |
| `workflow_path` | string \| null | No | Vault-relative path of workflow note (null for non-workflow) |
| `workflow_name` | string \| null | No | Display name of workflow (e.g., `"daily/review"`) |
| `persona_name` | string \| null | No | Active persona name (null for default persona) |
| `is_background` | boolean | No | True for event-triggered background workflow executions |
| `use_extended_context` | boolean | No | True if 1M extended context was active |
| `forked_from_conversation_id` | string \| null | No | Parent conversation ID for forked conversations |
| `forked_from_message_id` | string \| null | No | Fork-point message ID |
| `is_favorite` | boolean | No | Whether conversation is favorited |
| `preset_name` | string \| null | No | Model preset name active when created |
| `draft_text` | string \| null | No | Unsent draft text saved between conversation switches |

**Workflow conversation header example:**
```json
{
  "_type": "conversation",
  "id": "6b9f1205-817d-4308-9e5f-8eecab4847a1",
  "created_at": "2026-03-18T04:33:22.529Z",
  "updated_at": "2026-03-18T04:35:06.013Z",
  "provider_id": "bedrock",
  "model_id": "global.anthropic.claude-sonnet-4-6",
  "total_input_tokens": 48818,
  "total_output_tokens": 1562,
  "estimated_cost": 0.169884,
  "mode": "act",
  "workflow_path": "notor/workflows/narrative-02-transcript-to-outline.md",
  "workflow_name": "narrative-02-transcript-to-outline",
  "persona_name": null,
  "is_background": false,
  "title": "Workflow: narrative-02-transcript-to-outline"
}
```

### Message Schema (Lines 2+)

```json
{
  "_type": "message",
  "id": "f8bcd674-4f33-46ef-b158-5518ecc885bc",
  "conversation_id": "0ecc5e56-6460-41c4-8762-464cec0816e7",
  "role": "user",
  "content": "Is there a `pandas` equivalent in the world of TypeScript?",
  "timestamp": "2026-03-10T04:15:28.932Z",
  "input_tokens": null,
  "output_tokens": null,
  "cost_estimate": null,
  "tool_call": null,
  "tool_result": null,
  "truncated": false,
  "auto_context": null,
  "attachments": null,
  "hook_injections": null
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `_type` | `"message"` | Yes | Discriminator — always `"message"` |
| `id` | string (UUID v4) | Yes | Unique message identifier |
| `conversation_id` | string (UUID v4) | Yes | Parent conversation ID |
| `role` | MessageRole | Yes | Message role (see below) |
| `content` | string \| ContentBlock[] | Yes | Message text or structured content blocks |
| `timestamp` | string (ISO 8601) | Yes | When the message was created, UTC with ms precision |
| `input_tokens` | number \| null | No | Input token count (null for non-LLM messages) |
| `output_tokens` | number \| null | No | Output token count (null for non-LLM messages) |
| `cost_estimate` | number \| null | No | Estimated cost for this message |
| `tool_call` | ToolCall \| null | No | Tool call details (for `tool_call` role only) |
| `tool_result` | ToolResult \| null | No | Tool result details (for `tool_result` role only) |
| `truncated` | boolean | No | Whether message was truncated from LLM context |
| `auto_context` | string \| null | No | Auto-context XML block injected into user messages |
| `attachments` | array \| null | No | Metadata for attached notes/files on user messages |
| `hook_injections` | string[] \| null | No | Captured stdout from pre-send hooks |
| `is_hook_injection` | boolean | No | Whether this user message is a hook injection |
| `is_workflow_message` | boolean | No | Whether this is the opening workflow instructions message |
| `source_extension` | string \| null | No | Extension name for `extension_block` role messages |
| `exclude_from_compaction` | boolean | No | Whether to exclude from compaction summarizer input |

### Message Roles

```typescript
type MessageRole = "system" | "user" | "assistant" | "tool_call" | "tool_result" | "extension_block";
```

| Role | Description | Translator relevance |
|------|-------------|---------------------|
| `user` | User-typed messages | **Yes** — primary user content |
| `assistant` | LLM responses (text content) | **Yes** — primary assistant content |
| `tool_call` | LLM-requested tool invocation | **Yes** — collapse into summary |
| `tool_result` | Tool execution output | **Yes** — collapse into summary |
| `system` | System messages (including compaction records) | **Selective** — detect compaction events |
| `extension_block` | Extension-produced messages (e.g., memory recall/capture) | **No** — skip in translator |

**Critical difference from Claude Code:** Notor uses **dedicated roles** for tool calls and results (`tool_call`, `tool_result`) rather than embedding them as content blocks within `user`/`assistant` messages. Each tool call and each tool result is its own JSONL line with its own `_type: "message"`.

### Content Field Format

The `content` field can be either:
1. **Plain string** — most common for `user` and `assistant` messages
2. **ContentBlock array** — for messages with images, documents, or custom extension blocks

```typescript
type ContentBlock =
  | { type: "text"; text: string }
  | { type: "image"; media_type: ImageMediaType; data: string; width?: number; height?: number }
  | { type: "document"; media_type: "application/pdf"; data: string; page_count?: number }
  | { type: "custom_block"; kind: string; data: Record<string, unknown>; fallback_text?: string; estimated_wire_tokens?: number; loading?: boolean };
```

For the translator: use `typeof content === "string" ? content : content.filter(b => b.type === "text").map(b => b.text).join("\n")` to extract text.

### ToolCall Schema

```json
{
  "role": "tool_call",
  "content": "",
  "tool_call": {
    "id": "tooluse_JHlCmvR9LP15REfVj0Qu2R",
    "tool_name": "search_vault",
    "parameters": { "query": "pandas TypeScript" },
    "status": "pending"
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Provider-assigned tool call ID (for correlation with results) |
| `tool_name` | string | Name of the tool invoked |
| `parameters` | Record<string, unknown> | Tool parameters as key-value pairs |
| `status` | `"pending"` \| `"approved"` \| `"rejected"` \| `"success"` \| `"error"` | Current status |

### ToolResult Schema

```json
{
  "role": "tool_result",
  "content": "",
  "tool_result": {
    "tool_name": "search_vault",
    "success": true,
    "result": { "total_matches": 0, "files": [] },
    "duration_ms": 169,
    "tool_call_id": "tooluse_JHlCmvR9LP15REfVj0Qu2R"
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `tool_name` | string | Name of the tool that was invoked |
| `success` | boolean | Whether execution succeeded |
| `result` | string \| Record<string, unknown> | Tool output (can be string or structured JSON) |
| `error` | string \| null | Error message if execution failed |
| `duration_ms` | number | Execution time in milliseconds |
| `tool_call_id` | string | Matches `id` on the corresponding ToolCall |
| `content_blocks` | ContentBlock[] | Optional media output from tool execution |
| `sub_agent_metadata` | object \| null | Sub-agent execution metadata (see below) |

**Sub-agent metadata** (present only on `use_subagent` tool results):
```json
{
  "sub_agent_metadata": {
    "jsonl_filename": "20260430_043827_62afbf59..._subagent_495b4938....jsonl",
    "token_usage": { "input": 1658, "output": 73 },
    "iteration_count": 2,
    "stop_reason": "completed",
    "profile_name": "notor-help"
  }
}
```

### Tool Call / Result Pairing

Tool calls and results are **paired** via `tool_call.id` ↔ `tool_result.tool_call_id`. They always appear as adjacent JSONL lines: a `tool_call` message immediately followed by its `tool_result` message.

### Compaction Records

When the conversation is compacted (summarized to reclaim context window space), a `system` role message is appended with the `content` field containing a JSON-serialized `CompactionRecord`:

```json
{
  "_type": "message",
  "role": "system",
  "content": "{\"id\":\"...\",\"conversation_id\":\"...\",\"type\":\"compaction\",\"timestamp\":\"2026-03-10T07:45:08.602Z\",\"token_count_at_compaction\":1160600,\"context_window_limit\":200000,\"threshold\":0.8,\"summary\":\"The user asked...\",...}"
}
```

Detection: `role === "system"` and `JSON.parse(content).type === "compaction"`. The translator should be aware that messages before a compaction record may have been summarized and replaced in the active context window, but they are all preserved in the JSONL file.

### Per-Message Timestamps

Every message has a `timestamp` field — **ISO 8601 with millisecond precision, UTC** (e.g., `"2026-03-10T04:15:28.932Z"`). This is the same format as Claude Code.

The translator can use per-message timestamps for the `previously_processed` flag: compare each message's `timestamp` to `last_processed`. Only messages with timestamps > `last_processed` get `previously_processed: false`.

### Persona/Workflow Metadata (Per-Conversation, Not Per-Message)

Persona and workflow metadata are stored **on the conversation header only**, not on individual messages:
- `workflow_path` — vault-relative path to the workflow note
- `workflow_name` — display name of the workflow
- `persona_name` — active persona name (null for default)
- `is_background` — true for event-triggered (background) workflow executions

The translator extracts these from line 1 of the JSONL file. Individual messages carry `is_workflow_message: true` to flag the opening workflow instructions message, which can be used to filter out verbose workflow instruction text from extraction.

### Sub-Agent Conversation Files

Sub-agent conversations are stored as separate JSONL files with `_subagent_` in the filename. Their header uses `_type: "sub_agent_conversation"` (not `"conversation"`), so they are automatically excluded from `listConversations()`.

Header schema:
```json
{
  "_type": "sub_agent_conversation",
  "id": "343b904a-...",
  "parent_conversation_id": "62afbf59-...",
  "sub_agent_name": "notor-help",
  "provider_id": "bedrock",
  "model_id": "global.anthropic.claude-opus-4-6-v1",
  "total_input_tokens": 1658,
  "total_output_tokens": 73,
  "iteration_count": 2,
  "stop_reason": "completed",
  "created_at": "2026-04-30T04:42:29.299Z"
}
```

The translator should **skip sub-agent files** (identify via `isSubAgentFilename()` check: `filename.includes("_subagent_")`). The sub-agent's output is already captured in the parent conversation's `tool_result` for the `use_subagent` tool call.

### Extension Block Messages

Messages with `role: "extension_block"` carry plugin extension data (e.g., memory recall results, memory capture records). Example:
```json
{
  "role": "extension_block",
  "content": [{ "type": "custom_block", "kind": "memory_recalled", "data": {...} }],
  "source_extension": "Memory Search (auto-inject)",
  "exclude_from_compaction": false
}
```

The translator should **skip extension_block messages** — they are internal plugin state, not user/assistant conversation content.

### Complete Conversation Example (Annotated)

```
Line 1: {"_type":"conversation","id":"...","created_at":"2026-03-10T04:15:13.521Z",...,"title":"Is there a pandas equivalent..."}
Line 2: {"_type":"message","role":"user","content":"Is there a `pandas` equivalent in the world of TypeScript?","timestamp":"2026-03-10T04:15:28.932Z",...}
Line 3: {"_type":"message","role":"tool_call","content":"","tool_call":{"tool_name":"search_vault","parameters":{"query":"pandas TypeScript"},"status":"pending"},"timestamp":"2026-03-10T04:15:32.526Z",...}
Line 4: {"_type":"message","role":"tool_result","content":"","tool_result":{"tool_name":"search_vault","success":true,"result":{"total_matches":0,"files":[]},"tool_call_id":"tooluse_JHlCmvR9LP15REfVj0Qu2R"},"timestamp":"2026-03-10T04:15:32.698Z",...}
Line 5: {"_type":"message","role":"assistant","content":"No existing notes on this topic...","timestamp":"2026-03-10T04:15:46.659Z","input_tokens":20275,"output_tokens":509,...}
```

**Decision:**

1. **History path discovery:** Read `{vault}/.obsidian/plugins/notor/data.json`, parse JSON, extract `history_path` field. Resolve relative to vault root. Default: `{vault}/.obsidian/plugins/notor/history/`.

2. **File discovery:** List all `*.jsonl` files in the history directory. Filter out sub-agent files (filenames containing `_subagent_`). Each remaining file is one conversation.

3. **Parsing strategy:** Read line 1 as conversation header. Read remaining lines as messages. Parse each line as JSON. Use `_type` field to discriminate header vs. message lines.

4. **Per-message timestamps are available and should be used.** Same strategy as Claude Code: compare each message's `timestamp` to `last_processed`. Only messages with timestamps > `last_processed` get `previously_processed: false`.

5. **Persona/workflow metadata extraction:** Read `workflow_name`, `workflow_path`, `persona_name`, and `is_background` from the conversation header (line 1). These are per-conversation, not per-message. The translator should surface these in its output so the consolidation pipeline can use them for routing decisions.

6. **Tool call/result collapsing:** Pair adjacent `tool_call` + `tool_result` messages via `tool_call.id` ↔ `tool_result.tool_call_id`. Generate a summary like `"[Tool: search_vault -> success: 0 matches]"`. This mirrors the Claude Code translator's approach but is simpler since Notor uses dedicated roles rather than content blocks.

7. **Skip extension_block messages.** They are internal plugin state (memory recall, memory capture) and not relevant to knowledge extraction.

8. **Skip system messages** that are compaction records. The messages before compaction are still present in the JSONL file (the file is append-only), so the translator processes all non-system messages regardless.

9. **Content extraction:** For each message, check `typeof content === "string"`. If string, use directly. If array (ContentBlock[]), filter to `type: "text"` blocks and join with newline. Skip image/document/custom_block content blocks.

10. **Sub-agent conversations should be skipped in MVP.** The sub-agent's output is already captured in the parent conversation's tool result.

---

## R-5: Claude Code Hook Registration ✅

**Question:** How to programmatically register a `user_prompt_submit` hook in Claude Code?

**Areas to Investigate:**
- Hook configuration file location and format
- Registration API (file edit? CLI command? JSON schema?)
- How multiple hooks at the same trigger point coexist
- Context provided to the hook at runtime (env vars, stdin, args)
- How the hook's stdout is consumed (prepended to system context? shown to user?)
- First-message detection: what signals are available to determine if this is the first message in a conversation?

**Prototype Task:** Register a test hook that prints "Hello from multi-kb" on first message only.

**Findings:**

### Hook Configuration Location

Hooks are configured in JSON settings files under the top-level `"hooks"` key:

| Location | Scope |
|----------|-------|
| `~/.claude/settings.json` | Global (all projects) |
| `~/.claude/settings.local.json` | Global (not checked in) |
| `<project>/.claude/settings.json` | Per-project |
| `<project>/.claude/settings.local.json` | Per-project (not checked in) |

**Recommended for multi-kb:** Use `~/.claude/settings.json` (global scope) since multi-kb hooks should fire for all projects.

### Hook JSON Schema

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "multi-kb hook --harness claude-code",
            "timeout": 10
          }
        ]
      }
    ]
  }
}
```

**Event-level entry** (each element in the `UserPromptSubmit` array):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `matcher` | string | No (omit = match all) | Regex pattern; `"*"` for all |
| `hooks` | array | Yes | Array of hook actions (all run in **parallel**) |

**Hook action** (each element in the inner `hooks` array):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | `"command"` or `"prompt"` | Yes | Execution type |
| `command` | string | Yes (if command) | Shell command to execute |
| `timeout` | integer (seconds) | No | Default: 60s. Max: 600s. |

### Available Hook Events

| Event | When | Supports prompt hooks? |
|-------|------|----------------------|
| **`UserPromptSubmit`** | When user submits a prompt | Yes |
| `PreToolUse` | Before tool executes | Yes |
| `PostToolUse` | After tool completes | No |
| `Stop` / `SubagentStop` | Agent considers stopping | Yes |
| `SessionStart` / `SessionEnd` | Session lifecycle | No (command only) |
| `PreCompact` | Before context compaction | No |
| `Notification` | Notifications sent | No |

### Multiple Hook Coexistence

Hooks use a **two-level array**:
1. **Outer array**: Multiple matcher groups per event. All entries whose `matcher` matches the context fire.
2. **Inner `hooks` array**: Multiple actions per matcher group. All run **in parallel**.

Adding a multi-kb hook alongside existing hooks is safe — append a new entry to the outer array.

### Runtime Context (stdin)

`UserPromptSubmit` hooks receive JSON on stdin:

```json
{
  "session_id": "abc123",
  "transcript_path": "/path/to/transcript.txt",
  "cwd": "/current/working/dir",
  "permission_mode": "ask",
  "hook_event_name": "UserPromptSubmit",
  "user_prompt": "The actual text the user typed"
}
```

**Environment variables:**
- `$CLAUDE_PROJECT_DIR` — Project root path

### Hook Output Format

**IMPORTANT CHANGE:** Hook output is **not** raw Markdown to stdout as the spec assumed. It is structured JSON:

```json
{
  "continue": true,
  "suppressOutput": false,
  "systemMessage": "Injected context text here"
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `continue` | bool | `true` | If false, halt processing |
| `suppressOutput` | bool | `false` | If true, hide output from transcript |
| `systemMessage` | string | — | Message injected into Claude's system context |

**Impact:** The CLI must output a JSON object with the `systemMessage` field containing the formatted Markdown, not raw Markdown to stdout. The spec's description of "raw Markdown to stdout" is incorrect.

**Exit codes:**
- `0`: Success. stdout parsed for structured output.
- `2`: Blocking error. stderr fed back to Claude.
- Other: Non-blocking error.

### First-Message Detection

No explicit `is_first_message` field exists in the hook input. Detection strategies:

1. **Transcript-based (recommended):** Check `transcript_path` for prior `user` entries. If the transcript has no prior user messages, this is the first message:
   ```bash
   # Read JSON from stdin, extract transcript_path
   # If transcript file is empty or has no prior "role":"user" entries → first message
   ```

2. **Session state file:** Create a flag file keyed by `session_id`. If absent, it's the first message; touch the flag and proceed. Requires cleanup logic.

3. **SessionStart + env:** Use a `SessionStart` hook to write to `$CLAUDE_ENV_FILE`, then check the env var in `UserPromptSubmit`. More complex but avoids transcript parsing.

**Recommended approach:** Transcript-based detection. The `transcript_path` is reliably provided, and checking for prior user messages is deterministic. The Go binary reads the transcript file, counts `user`-type lines, and if count ≤ 1 (current prompt only), treats it as first message.

### Hooks Load at Session Start Only

Changing hook configuration requires restarting Claude Code. This is fine for `multi-kb setup` — hooks are registered once and loaded on next session start.

**Decision:**

1. **Registration target:** Write to `~/.claude/settings.json` under `hooks.UserPromptSubmit`. Read the existing file, parse JSON, append a new entry to the `UserPromptSubmit` array (create if absent), write back.

2. **Idempotency:** Before appending, check if an entry with a command containing `multi-kb hook` already exists. If so, update it rather than duplicating.

3. **Hook command:** `multi-kb hook --harness claude-code`. The CLI binary must be on PATH (documented in setup).

4. **Timeout:** Set to 10 seconds (slightly above the 8-second hook timeout in the CLI config, to avoid Claude Code killing the process before the CLI's internal timeout fires).

5. **Output format:** Return `{"systemMessage": "<formatted markdown>"}` on stdout. The spec's references to "raw Markdown to stdout" need updating — the Markdown content goes inside the `systemMessage` field of a JSON object.

6. **First-message guard:** Use transcript-based detection. The hook reads the `transcript_path` from stdin JSON, checks for prior user entries, and exits with code 0 and no output if this is not the first message.

7. **Input parsing:** The hook reads the user's prompt from stdin JSON (`user_prompt` field), not from args or env vars. The Go binary must parse JSON from stdin.

8. **Exit code semantics:** Exit 0 with empty stdout (or `{}`) for non-first-message cases. Exit 0 with `{"systemMessage": "..."}` for first-message injection. Exit non-0/non-2 for errors (non-blocking).

9. **Settings file editing:** Use read-modify-write with JSON parsing (not string concatenation). Handle the case where `settings.json` doesn't exist yet, or where `hooks` key doesn't exist, or where `UserPromptSubmit` key doesn't exist.

---

## R-6: Notor Hook Registration ✅

**Question:** How to programmatically register a conversation-start hook in Notor?

**Areas to Investigate:**
- Hook configuration mechanism (Obsidian plugin settings? Config file?)
- Registration format
- How multiple hooks at the same trigger point coexist
- Context provided to the hook at runtime
- How hook output is injected into the conversation system context

**Prototype Task:** Register a test hook that injects a test string at conversation start.

**Source:** [github.com/zachmueller/notor](https://github.com/zachmueller/notor) — TypeScript Obsidian plugin, MIT licensed, not yet in the Obsidian community plugin registry (installed via BRAT or from source).

**Findings:**

### Two Hook Mechanisms in Notor

Notor has two distinct hook mechanisms. Understanding both is critical for choosing the right approach:

#### 1. Shell Command Hooks (Phase 3 — Settings UI)

Configured in the Obsidian plugin settings UI. Support four lifecycle events:

| Event | Execution Model | Stdout Captured? |
|-------|----------------|-----------------|
| `pre_send` | **Blocking**, sequential, awaited | **Yes** — injected as user message |
| `on_tool_call` | Fire-and-forget, sequential | No |
| `on_tool_result` | Fire-and-forget, sequential | No |
| `after_completion` | Fire-and-forget, sequential | No |

**`pre_send` is the only shell hook event that captures stdout and injects it into the conversation.** There is no shell hook event for `on_conversation_start`.

#### 2. User Automations (Phase 5 — Markdown files in vault)

Defined as Markdown files in `{vault}/notor/automations/`. Support all lifecycle events plus vault events, including `on_conversation_start`. These are TypeScript/JavaScript code running inside the Obsidian runtime with access to the full plugin API (`utils`, `obsidian`, `libs`, `context`).

| Trigger | Blocking? | Notes |
|---------|----------|-------|
| `on_conversation_start` | Optional (configurable) | Only meaningful trigger for conversation-start injection |
| `pre_send` | Always blocking | Return value injected into conversation |
| `on_tool_call` / `on_tool_result` | No | Fire-and-forget |
| `after_completion` | No | Fire-and-forget |
| `on_schedule` | No | Cron-triggered |
| Vault events (`on_save`, etc.) | No | File-triggered |

### Hook Configuration Storage

**Shell hooks** are stored in Notor's Obsidian plugin settings at `{vault}/.obsidian/plugins/notor/data.json` under the `hooks` key:

```json
{
  "hooks": {
    "pre_send": [
      {
        "id": "uuid-v4",
        "event": "pre_send",
        "command": "multi-kb hook --harness notor",
        "label": "multi-kb knowledge injection",
        "enabled": true,
        "action_type": "execute_command"
      }
    ],
    "on_tool_call": [],
    "on_tool_result": [],
    "after_completion": []
  },
  "hook_timeout": 10
}
```

`data.json` is Obsidian's standard plugin settings file (each plugin has one at `{vault}/.obsidian/plugins/{plugin-id}/data.json`). It is a flat JSON file that the plugin reads via `this.loadData()` and writes via `this.saveData()`.

**User automations** are Markdown files in `{vault}/notor/automations/`:

```markdown
---
notor-type: automation
notor-trigger: on_conversation_start
notor-display-name: multi-kb knowledge injection
notor-blocking: true
notor-blocking-emit-kind: multi_kb_recall
notor-blocking-timeout: 10000
notor-automation-order: 100
---

\```ts
// TypeScript code running inside Obsidian
const result = await utils.executeShellCommand("multi-kb hook --harness notor", {
  timeout: 8000,
  env: { NOTOR_FIRST_MESSAGE: context.firstMessage }
});
// Inject result into conversation via chatBlocks.emit()
\```
```

### Approach A: `pre_send` Shell Hook (Recommended for MVP)

Use the `pre_send` shell hook event with a first-message guard, exactly analogous to the Claude Code `UserPromptSubmit` approach.

**How it works:**

1. Hook fires on **every user message** (not just conversation start)
2. Hook receives context via `NOTOR_*` environment variables:
   - `NOTOR_CONVERSATION_ID` — conversation UUID
   - `NOTOR_HOOK_EVENT` — `"pre_send"`
   - `NOTOR_TIMESTAMP` — ISO 8601 timestamp
3. Hook command runs with CWD set to the vault root
4. Hook stdout (if non-empty) is injected as a **separate user message** with `is_hook_injection: true`, rendered as a collapsible UI element
5. The hook message appears **before the user's actual message** in the conversation, so the LLM sees the injected context on its first turn

**First-message detection:** The `pre_send` hook does NOT receive the user's message text or conversation history as input. The environment variables include only `NOTOR_CONVERSATION_ID` and `NOTOR_TIMESTAMP`. To determine if this is the first message:

- **Option 1 (recommended):** Read the conversation history file from the Notor history directory. The CLI knows the vault root (CWD = vault root) and the history path. Look up the conversation by `NOTOR_CONVERSATION_ID`. If no history file exists yet or the file has no prior non-hook user messages, this is the first message.
- **Option 2:** Maintain a CLI-side session state file tracking conversation IDs that have already been processed. If the conversation ID is new, proceed with injection.

**Limitation:** The `pre_send` hook does NOT receive the user's first message text in environment variables. The hook would need to either:
1. Read the message from the Notor history file (which may not yet be written at `pre_send` time, since the message hasn't been dispatched yet)
2. Accept that the first-message text is not available for keyword derivation, and use a generic recall query instead
3. **Use stdin** — the hook-events.ts code shows `PreSendContext` includes only `conversationId` and `timestamp`, NOT the user's message. This is a significant limitation.

**Impact on multi-kb:** Without the user's first message, the hook cannot perform query-based recall. This makes `pre_send` shell hooks unsuitable as the primary injection mechanism — recall requires the user's query text.

### Approach B: User Automation with `on_conversation_start` (Recommended)

Use a user automation file that triggers on `on_conversation_start`, which provides the user's first message in the context.

**How it works:**

1. Create a Markdown automation file at `{vault}/notor/automations/multi-kb-recall.md`
2. The automation triggers on `on_conversation_start`, which fires after the first non-hook user message is submitted
3. The `ConversationStartContext` provides:
   - `context.conversationId` — conversation UUID
   - `context.firstMessage` — the user's first message text
   - `context.timestamp` — ISO 8601 timestamp
4. The automation runs the shell command `multi-kb hook --harness notor` with the first message passed as an environment variable or via stdin
5. The automation is configured as **blocking** with a timeout, so it completes before the LLM's first turn
6. The automation injects its output via `chatBlocks.emit()`, which creates an `extension_block` message visible to the LLM

**Advantages:**
- Receives the user's first message (`context.firstMessage`) — enables query-based recall
- Only fires on conversation start — no first-message guard needed
- Blocking mode ensures injection completes before LLM dispatch
- Native timeout support (`notor-blocking-timeout`)

**How to pass the first message to the CLI:** The automation calls `utils.executeShellCommand()` with the first message passed via stdin pipe or environment variable. The exact mechanism:

```typescript
const result = await utils.executeShellCommand(
  `echo '${JSON.stringify(context.firstMessage)}' | multi-kb hook --harness notor`,
  { timeout: 8000 }
);
```

Or, more robustly, using the shell's stdin redirection:

```typescript
const proc = await utils.executeShellCommand("multi-kb hook --harness notor", {
  timeout: 8000,
  stdin: context.firstMessage
});
```

**How output is injected:** Blocking `on_conversation_start` automations can emit blocks via `chatBlocks.emit(kind, data, opts)`. These become `extension_block` messages in the conversation transcript, visible to the LLM on its first turn. The architecture docs confirm: "Blocks emitted during blocking automations (pre_send, blocking on_conversation_start) land before the session snapshot and are visible to the LLM on the current turn."

### Approach C: Hybrid — Automation Wrapping Shell Hook

Create a thin automation file that wraps the shell command with proper context passing:

```markdown
---
notor-type: automation
notor-trigger: on_conversation_start
notor-display-name: multi-kb knowledge injection
notor-blocking: true
notor-blocking-emit-kind: multi_kb_recall
notor-blocking-timeout: 10000
notor-automation-order: 100
---

\```ts
const input = JSON.stringify({ first_message: context.firstMessage, conversation_id: context.conversationId });
const result = await utils.executeShellCommand(`echo '${input.replace(/'/g, "'\\''")}' | multi-kb hook --harness notor`, { timeout: 8000 });
if (result && result.trim()) {
  chatBlocks.emit("multi_kb_recall", { content: result.trim() }, { fallback_text: result.trim() });
}
\```
```

This gives the CLI the first message via stdin (as JSON), invokes the same `multi-kb hook --harness notor` command, and injects the output as an extension block.

### Multiple Hooks Coexistence

**Shell hooks (pre_send):** The `hooks.pre_send` array supports multiple entries. All enabled hooks execute sequentially in array order. Adding a multi-kb hook alongside existing hooks is safe — it's an array append.

**User automations (on_conversation_start):** Multiple automations can exist at the same trigger point. Execution order is determined by `notor-automation-order` (ascending) then filename alphabetically. Blocking automations are awaited sequentially. Multiple blocking automations coexist — they execute one after another before the LLM's first turn.

### Registration: How to Write the Automation File

**Programmatic registration** requires creating a Markdown file at `{vault}/notor/automations/multi-kb-recall.md`. The CLI needs:

1. **The vault path** — already configured in `config.yaml` as the source directory
2. **Write access** to `{vault}/notor/automations/` — the CLI must create this directory if it doesn't exist (Notor creates it automatically on first use, but it may not exist yet)
3. **File creation** — write the automation Markdown file with frontmatter + code fence

**Idempotency:** Check if `{vault}/notor/automations/multi-kb-recall.md` already exists. If so, overwrite it (update in place). The filename is the unique key.

**No Obsidian restart required:** Notor's extension watcher monitors the `notor/automations/` directory for file changes and reloads automatically.

### Hook Output Format

**For the shell command hook approach (Approach A — `pre_send`):** The hook writes raw text to stdout. Notor captures stdout and injects it as a separate user message with `is_hook_injection: true`. The output is NOT JSON-wrapped (unlike Claude Code). Raw Markdown to stdout.

**For the automation approach (Approach B/C — `on_conversation_start`):** The automation calls `chatBlocks.emit(kind, data, opts)` to inject an extension block. The `fallback_text` field provides the text representation visible to the LLM. The output is a structured block, not raw stdout.

**For the CLI's perspective:** In both approaches, `multi-kb hook --harness notor` outputs raw Markdown to stdout (no JSON wrapper). The difference is in how the harness consumes it:
- **Claude Code:** CLI must wrap output in `{"systemMessage": "..."}` JSON
- **Notor (pre_send):** CLI writes raw Markdown to stdout; Notor injects as-is
- **Notor (automation):** The automation TypeScript code reads the CLI's stdout and calls `chatBlocks.emit()` with it

### Context Injection Placement

**pre_send hooks:** Hook stdout becomes a **separate user message** (`is_hook_injection: true`) inserted **before** the user's actual message. The LLM sees it as user-provided context. It appears as a collapsible element in the UI.

**on_conversation_start automations:** Blocking automation output becomes an **extension_block message** inserted after the first user message but **before the LLM's first response**. The block is part of the session snapshot, so the LLM sees it in its context window.

Both approaches result in the injected content being visible to the LLM on its first turn. The key difference is metadata (hook injection vs. extension block) and where in the message sequence it appears.

### Global Hook Timeout

Default: 10 seconds (configurable via `hook_timeout` in plugin settings). This is the maximum time a shell hook can run before being terminated. For automations, the timeout is per-automation via `notor-blocking-timeout` (in milliseconds).

**Decision:**

1. **Use Approach C (hybrid automation wrapping shell command) as the primary mechanism.** This provides the critical `context.firstMessage` for query-based recall while keeping the CLI's hook logic (`multi-kb hook --harness notor`) identical across harnesses. The automation file is a thin adapter between Notor's TypeScript runtime and the CLI's shell interface.

2. **Registration target:** Write a Markdown automation file to `{vault}/notor/automations/multi-kb-recall.md`. The CLI creates the `notor/automations/` directory if needed. No modification to `data.json` required.

3. **Idempotency:** Check if the file exists. If so, overwrite it (the filename is the unique key). Re-running setup produces the same result.

4. **Hook command:** `multi-kb hook --harness notor`. The CLI binary must be on PATH (documented in setup).

5. **Input format:** The automation passes the user's first message to the CLI via stdin as JSON: `{"first_message": "<text>", "conversation_id": "<uuid>"}`. The CLI's Notor hook handler reads stdin JSON (same pattern as Claude Code, different field names). The CLI must handle both Claude Code's stdin schema (`user_prompt`, `transcript_path`, etc.) and Notor's schema (`first_message`, `conversation_id`) based on the `--harness` flag.

6. **Output format:** The CLI writes raw Markdown to stdout (no JSON wrapper). The automation reads the CLI's stdout and calls `chatBlocks.emit("multi_kb_recall", { content: result }, { fallback_text: result })` to inject it as an extension block visible to the LLM.

7. **Timeout:** Set `notor-blocking-timeout: 10000` (10 seconds) on the automation. The CLI's internal timeout remains 8 seconds. This gives the CLI 8 seconds to complete, with a 2-second buffer before Notor kills the automation.

8. **No first-message guard needed.** The `on_conversation_start` trigger only fires on the first message — unlike `pre_send` (which fires on every message) and unlike Claude Code's `UserPromptSubmit` (which also fires on every message).

9. **Automation order:** Set `notor-automation-order: 100` to run after Notor's built-in `memory-search` automation (which has a lower default order). This ensures Notor's own memory system runs first, and multi-kb knowledge injection follows.

10. **Coexistence:** The automation file does not interfere with any existing hooks or automations. Multiple `on_conversation_start` automations execute sequentially in order. No risk of overwriting user configuration.

11. **Spec impact:** The spec says hook output is "raw Markdown to stdout" injected into "system context." This is approximately correct for Notor: the automation produces an extension block visible to the LLM, which functions as injected context. The CLI's output format (raw Markdown to stdout) is correct for Notor (no JSON wrapper needed, unlike Claude Code's `{"systemMessage": "..."}`).

12. **Settings file location for reference:** Notor stores all plugin settings in `{vault}/.obsidian/plugins/notor/data.json`. The `hooks` key contains the shell hook arrays. However, multi-kb does NOT need to modify `data.json` — the automation file approach is entirely file-based and does not touch plugin settings.

13. **Automation file template:** The CLI should write the following file during setup:

```markdown
---
notor-type: automation
notor-trigger: on_conversation_start
notor-display-name: multi-kb knowledge injection
notor-blocking: true
notor-blocking-emit-kind: multi_kb_recall
notor-blocking-timeout: 10000
notor-automation-order: 100
---

\```ts
const input = JSON.stringify({
  first_message: context.firstMessage,
  conversation_id: context.conversationId,
  timestamp: context.timestamp
});
const result = await utils.executeShellCommand(
  "multi-kb hook --harness notor",
  { timeout: 8000, stdin: input }
);
if (result && result.trim()) {
  chatBlocks.emit("multi_kb_recall", { content: result.trim() }, {
    fallback_text: result.trim()
  });
}
\```
```

**Note:** The exact `utils.executeShellCommand` stdin API needs verification against the Notor source. If stdin piping is not supported, the alternative is to pass the input as a command-line argument or via a temporary file. The most robust fallback is environment variable injection: set `NOTOR_FIRST_MESSAGE` in the shell environment before invoking the CLI.

---

## R-7: Crockford Base32 UID Generation ✅

**Question:** Best approach for generating 16-character Crockford base32 UIDs in Go?

**Status:** Resolved

**Areas to Investigate:**
- Existing Go libraries for Crockford base32 (vs. standard base32)
- Crockford base32 alphabet: `0123456789ABCDEFGHJKMNPQRSTVWXYZ` (excludes I, L, O, U)
- Input: 10 bytes from `crypto/rand` = 80 bits → 16 Crockford base32 characters (5 bits each)
- Collision probability at scale (80 bits ≈ 1.2 × 10²⁴ possible values — more than sufficient)

**Prototype Task:** Implement and test the function. Verify output is always exactly 16 chars, valid alphabet, and passes round-trip decode.

**Findings:**

### Crockford Base32 Alphabet

The Crockford Base32 alphabet uses 32 symbols: `0123456789ABCDEFGHJKMNPQRSTVWXYZ`. It deliberately excludes four letters that are visually ambiguous: **I** (confused with 1), **L** (confused with 1), **O** (confused with 0), and **U** (accidental obscenity). The canonical form is uppercase.

### Encoding Algorithm

10 random bytes (80 bits) map to exactly 16 Crockford Base32 characters (5 bits per character, 80/5 = 16). No padding is needed — the math is exact.

**Bit-buffer approach (recommended for Go):**

```go
const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func GenerateUID() (string, error) {
    b := make([]byte, 10)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    return EncodeCrockford(b), nil
}

func EncodeCrockford(data []byte) string {
    out := make([]byte, 16)
    buf := 0
    bits := 0
    pos := 0
    for _, b := range data {
        buf = (buf << 8) | int(b)
        bits += 8
        for bits >= 5 {
            bits -= 5
            out[pos] = alphabet[(buf>>bits)&0x1F]
            pos++
        }
    }
    return string(out)
}
```

The bit-buffer approach processes one byte at a time, accumulating bits into an integer and extracting 5-bit groups as they become available. For exactly 10 input bytes, this produces exactly 16 output characters with 0 remaining bits.

An alternative 5-byte chunk approach (process 5 bytes → 8 chars in two groups) produces identical output and may be slightly faster, but the bit-buffer approach is clearer and more general.

### No Third-Party Library Needed

Existing Go packages for Crockford Base32 (e.g., `github.com/richardlehane/crock32`) add unnecessary dependencies for what is a ~15-line function. The encoding is simple enough to implement inline with `crypto/rand` for entropy.

The Go standard library's `encoding/base32` does NOT support Crockford encoding — it uses RFC 4648 (different alphabet, padding characters). A custom `NewEncoding` with the Crockford alphabet would work but adds complexity for no benefit over the direct bit-manipulation approach.

### Collision Probability

80 bits of entropy provides ~1.2 × 10²⁴ possible values. The birthday paradox threshold is ~2⁴⁰ ≈ 1 trillion UIDs before a 50% collision probability. For a knowledge base system generating hundreds to thousands of notes, collision is astronomically unlikely.

### Authoritative Test Vectors

These vectors are shared between Go (CLI R-7) and Node.js (CDK R-5) to ensure cross-implementation compatibility:

| Input (hex) | Input (bytes) | Output |
|---|---|---|
| `00000000000000000000` | `[0x00 × 10]` | `0000000000000000` |
| `FFFFFFFFFFFFFFFFFFFF` | `[0xFF × 10]` | `ZZZZZZZZZZZZZZZZ` |
| `00010203040506070809` | `[0x00..0x09]` | `000G40R40M30E209` |
| `DEADBEEFCAFEBABE0042` | `[0xDE, 0xAD, ...]` | `VTPVXVYAZTXBW022` |
| `48656C6C6F576F726C64` | `"HelloWorld"` | `91JPRV3FAXQQ4V34` |

### Decoding (Optional)

Crockford Base32 defines case-insensitive decoding with error correction (mapping `I`/`i`/`l` → `1`, `O`/`o` → `0`). Decoding is NOT needed for MVP — UIDs are generated locally and never decoded back to bytes. If needed later, the mapping table is trivial to implement.

**Decision:**

1. **Zero-dependency implementation.** Use the bit-buffer encoding algorithm with `crypto/rand` for 10 bytes of entropy. No third-party library needed. Place in `internal/submit/uid.go`.

2. **Canonical uppercase output.** All UIDs are uppercase Crockford Base32. The Go implementation should produce uppercase directly (using the uppercase alphabet constant), not encode then uppercase.

3. **Export `EncodeCrockford()` separately from `GenerateUID()`.** This enables deterministic testing: `EncodeCrockford(knownBytes)` should produce the test vectors above. `GenerateUID()` calls `EncodeCrockford()` with random bytes.

4. **Test vectors shared with CDK R-5.** The 5 test vectors above must pass in both the Go and Node.js implementations. This ensures UIDs generated by the Lambda (CDK) and by the CLI are format-compatible (though they are never correlated — each side generates independent UIDs).

5. **No decoding in MVP.** UIDs are write-once identifiers. No need for Crockford decode or error correction.

---

## R-8: Cross-Platform Cron Registration ✅

**Question:** How to safely register scheduled tasks on macOS/Linux (crontab) and Windows (Task Scheduler)?

**macOS/Linux — crontab:**
- Read existing crontab: `crontab -l`
- Append entry with a marker comment (e.g., `# multi-kb scheduled run`)
- Write back: `crontab -` (pipe new content)
- Idempotency: check for marker comment before appending
- Removal: filter out lines with marker comment

**Windows — Task Scheduler:**
- Use `schtasks.exe /Create` with appropriate flags
- XML task definition for more control
- Idempotency: check for existing task by name before creating
- Removal: `schtasks.exe /Delete /TN "multi-kb-run" /F`

**Areas to Investigate:**
- Does `crontab -l` fail on empty crontab? (Yes on some systems — handle gracefully)
- Windows permissions requirements (does it need admin?)
- How to parse the cron expression back for `multi-kb status` next-run display

**Prototype Task:** Implement register/unregister/check on macOS. Implement register/unregister/check on Windows (if available).

**Findings:**

### macOS/Linux -- crontab

#### 1. Reading Existing Crontab (`crontab -l`)

**Behavior on empty crontab varies by platform:**

- **Linux (most distros):** `crontab -l` prints `no crontab for <user>` to **stderr** and exits with code **1** when no crontab exists.
- **macOS:** `crontab -l` prints `crontab: no crontab for <user>` to **stderr** and exits with code **1**.
- **Some systems (e.g., older FreeBSD):** May return exit code 0 with empty output.

**Go handling pattern:**

```go
cmd := exec.Command("crontab", "-l")
out, err := cmd.Output()
if err != nil {
    // Check if it's just "no crontab" (exit code 1)
    if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
        // No existing crontab -- treat as empty
        out = []byte{}
    } else {
        return fmt.Errorf("failed to read crontab: %w", err)
    }
}
existing := string(out)
```

The key insight: exit code 1 with stderr containing "no crontab" is the normal case for a user who has never set a crontab. The Go code must handle this gracefully rather than treating it as a fatal error.

#### 2. Safe Appending Pattern

The canonical pattern for appending without clobbering is:

```bash
(crontab -l 2>/dev/null; echo "*/30 * * * * /path/to/multi-kb run --config ~/.multi-kb/config.yaml # multi-kb scheduled run") | crontab -
```

**In Go, this translates to:**

```go
func installCrontab(cronExpr, binaryPath, configPath string) error {
    // 1. Read existing crontab
    existing, err := readCrontab()
    if err != nil {
        return err
    }

    // 2. Build the new entry
    marker := "# multi-kb scheduled run"
    entry := fmt.Sprintf("%s %s run --config %s >> ~/.multi-kb/logs/cron.log 2>&1 %s",
        cronExpr, binaryPath, configPath, marker)

    // 3. Check idempotency -- remove old entry if present
    lines := strings.Split(strings.TrimRight(existing, "\n"), "\n")
    var filtered []string
    for _, line := range lines {
        if !strings.Contains(line, marker) {
            filtered = append(filtered, line)
        }
    }

    // 4. Append new entry
    filtered = append(filtered, entry)
    newCrontab := strings.Join(filtered, "\n") + "\n"

    // 5. Write back via crontab -
    cmd := exec.Command("crontab", "-")
    cmd.Stdin = strings.NewReader(newCrontab)
    if out, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("failed to write crontab: %s: %w", string(out), err)
    }
    return nil
}
```

**Critical:** `crontab -` reads the **entire** new crontab from stdin and **replaces** the current crontab atomically. The read-modify-write pattern (read existing, modify in memory, write back) is the only safe approach. Never use `echo "..." >> /var/spool/cron/...` -- crontab files are not directly user-writable and the path varies by OS.

#### 3. Marker Comment Reliability

Using `# multi-kb scheduled run` as a trailing comment is **highly reliable** for idempotency:

- Cron treats everything after `#` as a comment on that line (standard POSIX behavior).
- The marker is embedded in the cron entry line itself, not on a separate line, which avoids issues with line reordering.
- String matching with `strings.Contains(line, marker)` is deterministic and fast.
- Alternative approach: use a **dedicated comment line above the entry** (e.g., `# BEGIN multi-kb` / `# END multi-kb` block markers). This is more complex but supports multi-line entries. For a single cron entry, the inline marker is sufficient.

**Recommendation:** Use the inline marker `# multi-kb scheduled run` appended to the cron entry line. This is the simplest approach and matches patterns used by tools like `certbot` and `conda`.

#### 4. Idempotency

**Before appending, filter out any existing line containing the marker:**

```go
func isMultiKBEntry(line string) bool {
    return strings.Contains(line, "# multi-kb scheduled run")
}
```

This handles both the "first install" case (no existing marker) and the "update schedule" case (marker exists, replace with new expression). The read-filter-append-write pattern guarantees exactly one multi-kb entry in the crontab regardless of how many times setup is run.

#### 5. Removal

**Filter out lines with the marker and write back:**

```go
func uninstallCrontab() error {
    existing, err := readCrontab()
    if err != nil {
        return err
    }

    lines := strings.Split(strings.TrimRight(existing, "\n"), "\n")
    var filtered []string
    for _, line := range lines {
        if !strings.Contains(line, "# multi-kb scheduled run") {
            filtered = append(filtered, line)
        }
    }

    newCrontab := strings.Join(filtered, "\n")
    if len(filtered) > 0 {
        newCrontab += "\n"
    }

    cmd := exec.Command("crontab", "-")
    cmd.Stdin = strings.NewReader(newCrontab)
    if out, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("failed to write crontab: %s: %w", string(out), err)
    }
    return nil
}
```

**Edge case:** If removing the multi-kb entry results in an empty crontab, pipe an empty string (or just a newline) to `crontab -`. Some systems accept this; others may require `crontab -r` to fully remove the crontab. The safest approach: if `filtered` is empty, call `crontab -r` instead of piping an empty string.

#### 6. Cron Expression Format

| Schedule | Expression | Description |
|----------|-----------|-------------|
| Every 30 minutes | `*/30 * * * *` | Runs at :00 and :30 of every hour |
| Every hour | `0 * * * *` | Runs at :00 of every hour |
| Every 2 hours | `0 */2 * * *` | Runs at :00 every 2 hours |
| Daily at midnight | `0 0 * * *` | Runs once at 00:00 |
| Daily at 8 AM | `0 8 * * *` | Runs once at 08:00 |

The five fields are: `minute hour day-of-month month day-of-week`. The `*/N` syntax means "every N units" and is POSIX-standard.

For the CLI's configurable schedule, the user provides a frequency (e.g., "every 30 minutes") during setup, and the CLI maps it to the appropriate cron expression. The cron expression is stored in `config.yaml` for reference and used directly in the crontab entry.

#### 7. PATH Issues in Cron

**Cron runs with a minimal PATH** -- typically `/usr/bin:/bin` on Linux and `/usr/bin:/bin:/usr/sbin:/sbin` on macOS. The `multi-kb` binary will almost certainly NOT be on the cron PATH unless the user installed it to `/usr/local/bin`.

**Solution: Always use the absolute path to the binary in the cron entry.**

**Determining the binary's absolute path from within Go:**

```go
import (
    "os"
    "path/filepath"
)

func getBinaryPath() (string, error) {
    exe, err := os.Executable()
    if err != nil {
        return "", fmt.Errorf("could not determine executable path: %w", err)
    }
    // Resolve symlinks to get the canonical path
    resolved, err := filepath.EvalSymlinks(exe)
    if err != nil {
        return "", fmt.Errorf("could not resolve symlink: %w", err)
    }
    return resolved, nil
}
```

`os.Executable()` returns the path to the currently running binary. `filepath.EvalSymlinks()` resolves any symlinks (important for Homebrew installations where the binary is symlinked from a Cellar path). The resolved absolute path is used in the crontab entry.

**Caveat:** If the user moves or deletes the binary after setup, the cron entry will fail. This is acceptable -- `multi-kb status` can detect this by checking if the path in the crontab entry exists.

#### 8. Output Redirection

**Cron entries should redirect output to prevent cron from sending mail:**

```
*/30 * * * * /usr/local/bin/multi-kb run --config ~/.multi-kb/config.yaml >> ~/.multi-kb/logs/cron.log 2>&1 # multi-kb scheduled run
```

- `>> ~/.multi-kb/logs/cron.log` -- appends stdout to the log file (using `>>` not `>` to preserve history)
- `2>&1` -- redirects stderr to the same log file
- Without redirection, cron sends email to the user's local mailbox (usually `/var/mail/<user>`), which is rarely checked and fills up silently

**The CLI should ensure `~/.multi-kb/logs/` exists** before registering the crontab entry (create it during setup).

**Note:** `~` expansion works in crontab on both Linux and macOS because the shell that cron invokes expands it. However, for maximum portability, consider using the expanded absolute path (e.g., `/Users/zmueller/.multi-kb/logs/cron.log`). The Go code can expand `~` at registration time using `os.UserHomeDir()`.

#### 9. macOS-Specific Considerations

**Is crontab supported on modern macOS?** Yes, but with caveats:

- **cron is fully functional** on macOS Ventura, Sonoma, Sequoia, and later. It is NOT deprecated, despite Apple's preference for launchd.
- **Apple recommends launchd** (`launchctl` + plist files) for persistent scheduled tasks. However, crontab remains a supported, working mechanism for user-level cron jobs.
- **First-time crontab use on macOS:** The system may prompt for "Full Disk Access" or "Automation" permissions in System Preferences > Security & Privacy. If the terminal app (Terminal.app, iTerm2) has appropriate permissions, crontab works without issue. This is a one-time permission grant.
- **macOS power management:** Cron jobs do NOT wake a sleeping Mac. If the Mac is asleep when the cron job is scheduled to run, the job is simply skipped -- no catch-up execution occurs. This is acceptable for multi-kb (the next run will pick up any missed work).
- **launchd alternative:** A launchd plist could be used instead of crontab on macOS, offering power-aware scheduling (wake from sleep to run) and more reliable execution. However, this would require a macOS-specific code path. For MVP, crontab is sufficient -- the simplicity of a cross-platform cron approach outweighs the power management benefit.

**Recommendation:** Use crontab for MVP on both macOS and Linux. The `multi-kb status` output should note if the system was recently asleep (detectable by comparing wall clock time to expected cron intervals), but this is a post-MVP enhancement.

#### 10. Go Implementation

**Shell out via `os/exec`** to `crontab -l` and `crontab -`. This is the standard approach used by Go CLIs that manage crontab entries.

**Complete implementation pattern:**

```go
package scheduler

import (
    "fmt"
    "os/exec"
    "strings"
)

const cronMarker = "# multi-kb scheduled run"

// readCrontab reads the current user's crontab.
// Returns empty string if no crontab exists.
func readCrontab() (string, error) {
    cmd := exec.Command("crontab", "-l")
    out, err := cmd.Output()
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
            return "", nil // No existing crontab
        }
        return "", fmt.Errorf("reading crontab: %w", err)
    }
    return string(out), nil
}

// writeCrontab replaces the current user's crontab with the given content.
func writeCrontab(content string) error {
    cmd := exec.Command("crontab", "-")
    cmd.Stdin = strings.NewReader(content)
    if out, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("writing crontab: %s: %w", string(out), err)
    }
    return nil
}

// Install adds or updates the multi-kb cron entry.
func Install(cronExpr, binaryPath, configPath, logPath string) error {
    existing, err := readCrontab()
    if err != nil {
        return err
    }

    entry := fmt.Sprintf("%s %s run --config %s >> %s 2>&1 %s",
        cronExpr, binaryPath, configPath, logPath, cronMarker)

    lines := strings.Split(strings.TrimRight(existing, "\n"), "\n")
    var filtered []string
    for _, line := range lines {
        if line == "" && len(filtered) == 0 {
            continue // Skip leading empty lines from split
        }
        if !strings.Contains(line, cronMarker) {
            filtered = append(filtered, line)
        }
    }
    filtered = append(filtered, entry)

    return writeCrontab(strings.Join(filtered, "\n") + "\n")
}

// Uninstall removes the multi-kb cron entry.
func Uninstall() error {
    existing, err := readCrontab()
    if err != nil {
        return err
    }

    lines := strings.Split(strings.TrimRight(existing, "\n"), "\n")
    var filtered []string
    for _, line := range lines {
        if !strings.Contains(line, cronMarker) {
            filtered = append(filtered, line)
        }
    }

    if len(filtered) == 0 {
        // No entries left -- remove crontab entirely
        cmd := exec.Command("crontab", "-r")
        cmd.Run() // Ignore error (may fail if already removed)
        return nil
    }

    return writeCrontab(strings.Join(filtered, "\n") + "\n")
}

// IsInstalled checks if a multi-kb cron entry exists and returns the cron expression if found.
func IsInstalled() (bool, string, error) {
    existing, err := readCrontab()
    if err != nil {
        return false, "", err
    }

    for _, line := range strings.Split(existing, "\n") {
        if strings.Contains(line, cronMarker) {
            // Extract the cron expression (first 5 space-separated fields)
            fields := strings.Fields(line)
            if len(fields) >= 5 {
                cronExpr := strings.Join(fields[:5], " ")
                return true, cronExpr, nil
            }
        }
    }
    return false, "", nil
}
```

---

### Windows -- Task Scheduler

#### 1. `schtasks.exe` Usage

**Creating a task that runs every 30 minutes:**

```
schtasks.exe /Create /SC MINUTE /MO 30 /TN "multi-kb-run" /TR "\"C:\Users\user\bin\multi-kb.exe\" run --config \"%USERPROFILE%\.multi-kb\config.yaml\"" /F
```

Key parameters:
- `/SC MINUTE` -- schedule type is minute-based
- `/MO 30` -- modifier: every 30 minutes
- `/TN "multi-kb-run"` -- task name (unique identifier)
- `/TR "..."` -- the command to run (fully qualified path). Paths with spaces must be quoted with escaped inner quotes.
- `/F` -- force: create or overwrite if the task already exists (provides idempotency)

**Other schedule examples:**

| Schedule | Command |
|----------|---------|
| Every 30 min | `/SC MINUTE /MO 30` |
| Every hour | `/SC HOURLY /MO 1` |
| Every 2 hours | `/SC HOURLY /MO 2` |
| Daily at midnight | `/SC DAILY /ST 00:00` |
| Daily at 8 AM | `/SC DAILY /ST 08:00` |

#### 2. Permissions

**Creating a task for the current user does NOT require admin/elevated permissions.** The current user can create tasks that run under their own account without elevation.

- `/RU` parameter defaults to the current user if not specified.
- `/RL LIMITED` (default) runs with standard user privileges -- this is what multi-kb needs.
- `/RL HIGHEST` would require the user to be an administrator and would trigger UAC elevation.
- Running as `SYSTEM` requires admin privileges.

**For multi-kb:** No special permissions are needed. The task runs as the current user with standard privileges.

**Note:** `schtasks.exe` may prompt for the user's password on some Windows versions when creating a task. Use `/NP` (no password stored) to avoid this -- the task then runs only when the user is logged in, which is acceptable for multi-kb (a personal tool).

#### 3. Idempotency

**Option A (recommended): Use `/F` flag on create.** The `/F` flag suppresses warnings if the task already exists and overwrites it. This makes create idempotent -- running it multiple times produces the same result.

```
schtasks.exe /Create /SC MINUTE /MO 30 /TN "multi-kb-run" /TR "..." /F
```

**Option B: Check existence first, then create or change.**

Check existence:
```
schtasks.exe /Query /TN "multi-kb-run" 2>NUL
```

- **Exit code 0:** Task exists.
- **Exit code 1 (ERROR_FILE_NOT_FOUND):** Task does not exist. The error message is printed to stderr: `ERROR: The system cannot find the file specified.`

Then create (if not found) or change (if found):
```
schtasks.exe /Change /TN "multi-kb-run" /TR "..." /SC MINUTE /MO 30
```

**Recommendation:** Use `/F` on create. It is simpler and achieves the same result without needing a separate existence check.

#### 4. Removal

```
schtasks.exe /Delete /TN "multi-kb-run" /F
```

- `/F` suppresses the confirmation prompt ("Are you sure? Y/N").
- If the task does not exist, the command exits with code 1 and prints an error to stderr. The Go code should treat exit code 1 as "task not found" and return success (already removed).

#### 5. Task Name Convention

**Use `multi-kb-run`** as the task name. Convention notes:

- Task names are case-insensitive on Windows.
- Names can include backslashes to organize into folders: `\multi-kb\run` would create the task in a `multi-kb` folder in Task Scheduler. However, folder creation adds complexity. A flat name is simpler for MVP.
- The task name is the unique identifier -- no marker comment needed (unlike crontab).

#### 6. Working Directory

`schtasks.exe /Create` does **not** have a direct parameter for working directory. Options:

- **Wrap in a cmd.exe call:** `/TR "cmd /c cd /d \"%USERPROFILE%\" && multi-kb.exe run --config ..."`
- **Use XML task definition:** XML allows setting `<WorkingDirectory>` explicitly.
- **Not needed for multi-kb:** The CLI reads all paths from config and uses absolute paths internally. No working directory dependency.

**Recommendation:** Do not set a working directory. Use absolute paths for everything (binary path, config path, log path).

#### 7. User Context

By default, the task runs under the current user's account. Key behaviors:

- The task runs **whether or not the user is logged in** (unless `/IT` is specified for interactive-only).
- `/NP` prevents password prompting but restricts execution to when the user is logged in.
- For multi-kb: use the default (current user, runs whether logged in or not). If password prompting is problematic, fall back to `/NP`.

#### 8. Go Implementation

**Shell out to `schtasks.exe` via `os/exec`:**

```go
package scheduler

import (
    "fmt"
    "os/exec"
    "strings"
)

const taskName = "multi-kb-run"

// Install creates or updates the Windows scheduled task.
func Install(scheduleType, modifier, binaryPath, configPath, logPath string) error {
    // Build the command string with output redirection
    // Note: schtasks /TR doesn't support >> natively, so wrap in cmd.exe
    tr := fmt.Sprintf(`cmd /c ""%s" run --config "%s" >> "%s" 2>&1"`,
        binaryPath, configPath, logPath)

    cmd := exec.Command("schtasks.exe",
        "/Create",
        "/SC", scheduleType,
        "/MO", modifier,
        "/TN", taskName,
        "/TR", tr,
        "/F",    // Force overwrite if exists
        "/RL", "LIMITED",
    )
    if out, err := cmd.CombinedOutput(); err != nil {
        return fmt.Errorf("creating scheduled task: %s: %w", string(out), err)
    }
    return nil
}

// Uninstall removes the Windows scheduled task.
func Uninstall() error {
    cmd := exec.Command("schtasks.exe",
        "/Delete",
        "/TN", taskName,
        "/F",
    )
    out, err := cmd.CombinedOutput()
    if err != nil {
        // Ignore "task not found" errors
        if strings.Contains(string(out), "cannot find") {
            return nil
        }
        return fmt.Errorf("deleting scheduled task: %s: %w", string(out), err)
    }
    return nil
}

// IsInstalled checks if the scheduled task exists.
func IsInstalled() (bool, error) {
    cmd := exec.Command("schtasks.exe",
        "/Query",
        "/TN", taskName,
    )
    err := cmd.Run()
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
            return false, nil // Task not found
        }
        return false, fmt.Errorf("querying scheduled task: %w", err)
    }
    return true, nil
}

// GetSchedule retrieves the schedule details for the task in XML format.
func GetSchedule() (string, error) {
    cmd := exec.Command("schtasks.exe",
        "/Query",
        "/TN", taskName,
        "/XML",
    )
    out, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("querying task XML: %w", err)
    }
    return string(out), nil
}
```

**Windows output redirection caveat:** `schtasks.exe /TR` does not natively support shell redirection (`>>`, `2>&1`). The workaround is wrapping the command in `cmd /c "..."`. This is the standard approach used by Windows tools that need output capture from scheduled tasks.

---

### Cross-Platform Architecture

#### 1. Cron Expression Parsing for `multi-kb status` Next-Run Display

**Recommended library: `github.com/robfig/cron/v3`**

This is the de facto standard Go library for cron expression parsing, used by Kubernetes CronJobs, HashiCorp Nomad, and many other production systems.

**Key API for next-occurrence computation:**

```go
import (
    "fmt"
    "time"

    "github.com/robfig/cron/v3"
)

// ComputeNextRun parses a standard 5-field cron expression and returns
// the next occurrence after the given time.
func ComputeNextRun(cronExpr string, after time.Time) (time.Time, error) {
    schedule, err := cron.ParseStandard(cronExpr)
    if err != nil {
        return time.Time{}, fmt.Errorf("invalid cron expression %q: %w", cronExpr, err)
    }
    return schedule.Next(after), nil
}

// Usage example:
// next, _ := ComputeNextRun("*/30 * * * *", time.Now())
// fmt.Printf("Next run: %s\n", next.Format("2006-01-02 15:04"))
// Output: "Next run: 2026-05-01 14:30"
```

**The `Schedule` interface:**

```go
type Schedule interface {
    Next(time.Time) time.Time
}
```

`Next()` returns the next activation time strictly after the given time. This is exactly what `multi-kb status` needs to compute the next scheduled run.

**Parser details:**
- `cron.ParseStandard()` accepts standard 5-field cron expressions (minute hour dom month dow)
- Also accepts descriptors: `@hourly`, `@daily`, `@weekly`, `@monthly`, `@every 30m`
- The parser returns a `*SpecSchedule` implementing `Schedule`, which uses bit-packed field representations for efficient next-time computation
- No seconds field by default (6-field expressions require `cron.NewParser(cron.Second | ...)`)

**For Windows Task Scheduler:** There is no direct cron expression to parse. The approach for `multi-kb status` is:

- Store the cron expression in `config.yaml` as the canonical schedule representation (even on Windows, the CLI translates the cron expression to `schtasks` parameters at registration time).
- `multi-kb status` always uses the cron expression from config to compute next run, regardless of OS.
- Alternatively, parse the task XML from `schtasks /Query /TN "multi-kb-run" /XML` to extract the `<Triggers>` element, but this is complex and unnecessary if the cron expression is stored in config.

**Recommendation:** Store the cron expression in `config.yaml`. Use `robfig/cron/v3` to compute next occurrence on all platforms. Do not attempt to read back the schedule from the OS scheduler.

#### 2. Build Tags for Cross-Platform Compilation

**File structure:**

```
internal/scheduler/
    scheduler.go          # Common interface and types (no build constraint)
    scheduler_unix.go     # Unix (macOS + Linux) crontab implementation
    scheduler_windows.go  # Windows Task Scheduler implementation
```

**Common interface (`scheduler.go`):**

```go
package scheduler

// Schedule represents a registered OS-level scheduled task.
type Schedule struct {
    CronExpr   string // Standard 5-field cron expression
    BinaryPath string // Absolute path to the multi-kb binary
    ConfigPath string // Absolute path to config.yaml
    LogPath    string // Absolute path to cron log file
}

// Scheduler manages OS-level scheduled task registration.
// Implementation is platform-specific (crontab on Unix, Task Scheduler on Windows).
type Scheduler interface {
    Install(s Schedule) error
    Uninstall() error
    IsInstalled() (bool, string, error) // returns (installed, cronExpr, error)
}
```

**Unix implementation (`scheduler_unix.go`):**

```go
//go:build unix

package scheduler

// ... crontab-based implementation (see macOS/Linux section above)
```

**Windows implementation (`scheduler_windows.go`):**

```go
//go:build windows

package scheduler

// ... schtasks-based implementation (see Windows section above)
```

**Build tag details:**
- `//go:build unix` matches Linux, macOS (darwin), FreeBSD, and other Unix-like systems. This is a **predefined build tag** in Go that covers all POSIX platforms.
- `//go:build windows` matches Windows.
- File naming convention `_unix.go` and `_windows.go` provides an additional implicit constraint that matches the explicit `//go:build` directive. Both layers are used for maximum clarity.
- The `scheduler.go` file with the interface definition has no build constraint, so it compiles on all platforms.
- The `New()` factory function is defined in each platform-specific file and returns the platform-appropriate `Scheduler` implementation. The compiler selects the correct file at build time.

---

### Known Gotchas and Platform-Specific Issues

#### macOS

1. **TCC (Transparency, Consent, and Control):** First crontab edit may trigger a macOS permission dialog. The terminal app must have "Full Disk Access" or at minimum "Developer Tools" access. If denied, `crontab -l` returns a permission error.
2. **Sleep/wake:** Cron jobs missed during sleep are NOT retroactively executed. The next scheduled occurrence runs normally.
3. **SIP (System Integrity Protection):** Does not affect user-level crontab operations. Only system crontab (`/etc/crontab`) and system directories are protected.

#### Linux

1. **cron daemon must be running:** On some minimal Linux installs (containers, minimal VMs), `crond` may not be installed or running. The CLI should verify: `systemctl is-active cron` or `pgrep cron` (but this check is best-effort -- the CLI should not fail setup if it cannot verify cron is active).
2. **SELinux:** On SELinux-enforcing systems, crontab operations may be restricted if the SELinux policy does not allow the user's context to modify cron spool files. This is rare for standard user accounts.
3. **User crontab location:** Varies by distro (`/var/spool/cron/crontabs/<user>` on Debian/Ubuntu, `/var/spool/cron/<user>` on RHEL/CentOS). The `crontab` command abstracts this -- never access the files directly.

#### Windows

1. **Password prompting:** `schtasks /Create` may prompt for the user's password. Use `/NP` if this is problematic, accepting that the task only runs when the user is logged in.
2. **Antivirus interference:** Some endpoint protection software may block `schtasks.exe` from creating tasks for unknown executables. The CLI should provide a clear error message suggesting the user whitelist the binary.
3. **Task Scheduler service:** The "Task Scheduler" Windows service must be running (`Schedule` service). It is enabled by default on all Windows editions but could be disabled by group policy in enterprise environments.
4. **Long path support:** Windows paths may exceed 260 characters. Use `\\?\` prefix for long paths, or ensure install location is short.
5. **Output redirection in `/TR`:** `schtasks` does not interpret shell redirection natively. The `cmd /c "..."` wrapper is required for `>>` and `2>&1` to work.

#### Cross-Platform

1. **Binary relocation:** If the user moves the binary after setup, scheduled tasks will fail silently. `multi-kb status` should validate that the binary path in the scheduled task still exists.
2. **Config file relocation:** Same concern as binary relocation. The config path is baked into the scheduled task command.
3. **Concurrent registration:** If the user runs setup simultaneously from two terminals, the crontab read-modify-write cycle has a TOCTOU race condition. This is extremely unlikely and acceptable for an interactive setup command. No mitigation needed for MVP.

**Decision:**

1. **Use `crontab` on Unix (macOS + Linux) and `schtasks.exe` on Windows.** These are the standard OS-native mechanisms, require no additional dependencies, and are well-understood by users who need to inspect or modify the scheduled tasks.

2. **Platform abstraction via Go build tags.** Define a `Scheduler` interface in `internal/scheduler/scheduler.go`, with platform-specific implementations in `scheduler_unix.go` (`//go:build unix`) and `scheduler_windows.go` (`//go:build windows`). The `New()` factory function returns the platform-appropriate implementation.

3. **Absolute binary path via `os.Executable()` + `filepath.EvalSymlinks()`.** Resolve the full canonical path to the multi-kb binary at setup time and embed it in the crontab entry / schtasks command. This avoids PATH issues in cron's minimal environment.

4. **Cron expression as canonical schedule representation.** Store the cron expression in `config.yaml` regardless of OS. On Unix, use it directly in the crontab entry. On Windows, translate it to `schtasks` parameters at registration time. `multi-kb status` uses the stored cron expression to compute next run via `robfig/cron/v3`.

5. **`github.com/robfig/cron/v3` for next-run computation.** Use `cron.ParseStandard(expr)` to parse the 5-field expression, then `schedule.Next(time.Now())` to compute the next occurrence. This is used by `multi-kb status` on all platforms.

6. **Idempotency via marker comment (Unix) and `/F` flag (Windows).** On Unix, the inline `# multi-kb scheduled run` marker identifies the multi-kb entry for update/removal. On Windows, the `/F` flag on `schtasks /Create` overwrites the existing task by name. Both approaches ensure repeated setup runs produce the same result.

7. **Output redirection to `~/.multi-kb/logs/cron.log`.** Append stdout and stderr to a log file to prevent cron mail on Unix and capture errors for debugging. Expand `~` to the absolute home directory path at registration time.

8. **No launchd support for MVP.** macOS crontab is fully functional and provides cross-platform consistency with Linux. launchd plist support (for power-aware scheduling) is a post-MVP enhancement.

9. **Error handling:** Handle empty crontab (exit code 1 on `crontab -l`), task-not-found on uninstall (exit code 1 on `schtasks /Delete`), and permission errors with user-friendly messages guiding the user to resolve the issue.

10. **Cron expression choices offered during setup:** Default to `*/30 * * * *` (every 30 minutes). Also offer `0 * * * *` (hourly) and `0 */2 * * *` (every 2 hours) as presets. Advanced users can provide a custom cron expression. The expression is validated via `cron.ParseStandard()` before registration.
