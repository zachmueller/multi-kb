# Implement Workflow

Execute the implementation plan by processing and executing all tasks defined in tasks.md

## Description

This workflow systematically executes implementation tasks with built-in quality gates, progress tracking, and validation checkpoints. It handles phase-by-phase execution, dependency management, and ensures quality throughout implementation.

## Usage

Run this command to:

- Execute implementation tasks in proper sequence
- Validate quality gates and checkpoints
- Track progress and handle task dependencies
- Manage parallel and sequential task execution

## Workflow Steps

### Step 1: Prerequisites and Setup Validation

Determine the spec directory location and validate readiness for implementation:

**Determine Spec Directory:**

```bash
# Get current branch
BRANCH=$(git branch --show-current)

# Extract feature name from branch (e.g., feature/my-feature → my-feature)
# Spec directory: specs/{feature-name}/
```

**Verify required files exist:**

```bash
# Verify all required artifacts
test -f "{spec-dir}/tasks.md" && echo "Found tasks" || echo "ERROR: tasks.md not found"
test -f "{spec-dir}/plan.md" && echo "Found plan" || echo "ERROR: plan.md not found"
test -f "{spec-dir}/spec.md" && echo "Found spec" || echo "ERROR: spec.md not found"

# Check git repository status
git status --porcelain
```

**Establish {spec-dir} Reference:**
Throughout this workflow, `{spec-dir}` refers to the spec directory determined above based on your branch. All file operations will use this as the base path.

Ensure all prerequisite artifacts are complete and environment is ready.

### Step 2: Checklist Status Validation

Check completion status of all quality checklists before proceeding:

**Checklist Scanning Process:**

```bash
# Scan for checklist files
find . -path "*/checklists/*.md"

# Check each checklist for completion status
# Count total vs completed items for each checklist
```

**Status Analysis:** For each checklist file, count:

- **Total items:** All lines matching `- [ ]` or `- [X]` or `- [x]`
- **Completed items:** Lines matching `- [X]` or `- [x]`
- **Incomplete items:** Lines matching `- [ ]`

**Create Status Table:**

```
| Checklist | Total | Completed | Incomplete | Status |
|-----------|-------|-----------|------------|--------|
| ux.md     | 12    | 12        | 0          | PASS   |
| api.md    | 8     | 5         | 3          | FAIL   |
| security.md| 6    | 6         | 0          | PASS   |
```

**Gate Decision:**

- **PASS:** All checklists have 0 incomplete items → Proceed automatically
- **FAIL:** One or more checklists have incomplete items → Stop and ask user

**If FAIL:** Display incomplete checklist table and ask: "Some checklists are incomplete. Do you want to proceed with implementation anyway? (yes/no)"

- Wait for user response before continuing
- If "no"/"wait"/"stop" → Halt execution with guidance to complete checklists
- If "yes"/"proceed"/"continue" → Continue to Step 3 with warning logged

### Step 3: Load Implementation Context

Load and analyze all implementation artifacts:

**Required Artifacts:**

- **tasks.md:** Complete task list and execution plan
- **plan.md:** Tech stack, architecture, and file structure
- **spec.md:** Requirements and acceptance criteria

**Optional Artifacts:**

- **data-model.md:** Entity definitions and relationships
- **contracts/:** API specifications and test requirements
- **research.md:** Technical decisions and constraints
- **quickstart.md:** Development environment setup

**Context Extraction:**

- Parse task breakdown structure and dependencies
- Extract technology stack and architecture decisions
- Identify file paths and component relationships

### Step 4: Project Setup Verification

Verify and create essential project files based on detected setup:

**Technology Detection:**

```bash
# Check if git repository
git rev-parse --git-dir

# Detect technology stack from plan.md and existing files
# Check for package.json, requirements.txt, pom.xml, etc.
# Scan for Docker, ESLint, Prettier configurations
```

**Ignore File Management:** Based on detected technologies, create or verify ignore files:

**Git Repository (.gitignore):**

- **Node.js/JavaScript/TypeScript:** `node_modules/`, `dist/`, `build/`, `*.log`, `.env*`
- **Python:** `__pycache__/`, `*.pyc`, `.venv/`, `venv/`, `dist/`, `*.egg-info/`
- **Java:** `target/`, `*.class`, `*.jar`, `.gradle/`, `build/`
- **Universal:** `.DS_Store`, `Thumbs.db`, `*.tmp`, `*.swp`

**Creation Logic:**

- If ignore file exists: Verify essential patterns, append missing critical patterns only
- If ignore file missing: Create with full pattern set for detected technology
- Preserve existing custom patterns while adding standard ones

### Step 5: Parse Task Structure and Dependencies

Analyze tasks.md for execution planning:

**Task Structure Extraction:**

- **Task Phases:** Setup, Foundation, Core, Integration, Quality, Polish
- **Task Dependencies:** Sequential vs parallel execution rules
- **Task Details:** ID, description, file paths, acceptance criteria
- **Parallel Markers:** Tasks marked with `[P]` for concurrent execution

**Dependency Graph Analysis:**

- Build dependency graph from task relationships
- Identify critical path (longest dependency chain)
- Find parallelization opportunities within constraints
- Validate no circular dependencies exist

### Step 6: Phase-by-Phase Implementation Execution

Execute tasks following the structured approach:

**Execution Rules:**

- **Phase Completion:** Complete entire phase before moving to next
- **Dependency Respect:** Sequential tasks run in order, parallel tasks `[P]` can run together
- **TDD Approach:** Execute test setup tasks before implementation tasks where applicable
- **File Coordination:** Tasks affecting same files must run sequentially
- **Validation Checkpoints:** Verify phase completion before proceeding

**Phase Execution Process:**

#### Phase 0: Setup & Environment
- Development environment setup
- Dependency installation and configuration
- Project structure initialization

#### Phase 1: Foundation & Architecture
- Application architecture setup
- Data model implementation
- Database schema creation

#### Phase 2: Core Feature Implementation
- Business logic implementation
- Core feature components
- User interface components

#### Phase 3: Integration & External Services
- API endpoint implementation
- External service integration
- Data flow validation

#### Phase 4: Quality & Testing
- Unit test implementation
- Integration test suite
- Performance validation

#### Phase 5: Security & Compliance
- Security implementation
- Security testing and validation

#### Phase 6: Documentation & Deployment
- Technical documentation
- Deployment preparation
- Final end-to-end validation

### Step 7: Task Execution Management

Handle individual task execution with proper tracking:

**Task Execution Process:**

1. **Pre-Task Validation:** Verify dependencies satisfied
2. **Task Execution:** Execute task following acceptance criteria
3. **Progress Tracking:** Mark task as completed in tasks.md
4. **Validation:** Verify acceptance criteria satisfied
5. **Error Handling:** Handle failures and dependencies

**Progress Tracking:**

- Update tasks.md with completed tasks marked as `[X]`
- Log completion timestamp and validation results
- Track overall progress and phase completion
- Report blocking issues and dependency problems

**Error Handling:**

- **Non-Parallel Task Failure:** Halt execution, report error with context
- **Parallel Task Failure:** Continue with successful tasks, report failed ones
- **Dependency Failure:** Block dependent tasks, suggest resolution
- **Validation Failure:** Roll back if possible, require manual intervention

### Step 8: Progress Tracking and Reporting

Provide comprehensive progress tracking throughout implementation:

**Progress Reporting Format:**

```
Implementation Progress Report
=============================
Feature: [Feature Name]
Branch: [Branch Name]

Phase Progress:
  Setup & Environment (100%)
  Foundation & Architecture (100%)
→ Core Feature Implementation (60%)
  - Integration & External Services (0%)
  - Quality & Testing (0%)
  - Security & Compliance (0%)
  - Documentation & Deployment (0%)

Overall Progress: 32% (18/56 tasks completed)

Current Status:
- Active Phase: Core Feature Implementation
- Blocking Issues: None
- Next Milestone: UI component completion
```

### Step 9: Implementation Completion Validation

Final validation and completion verification:

**Completion Validation Checklist:**

- [ ] All tasks marked as completed `[X]` in tasks.md
- [ ] All phase validation checkpoints passed
- [ ] Quality gates satisfied at all levels
- [ ] Final end-to-end validation successful

**Feature Validation Against Original Specification:**

- [ ] All functional requirements implemented and validated
- [ ] All non-functional requirements satisfied
- [ ] User scenarios work end-to-end as specified
- [ ] Success criteria from spec.md achieved
- [ ] Edge cases and error scenarios handled correctly

**Final Documentation and Handoff:**

Follow the commit standards defined in `.claude/rules/git.md` to commit all implementation changes and updated artifacts.

**Completion Report:**

- Implementation summary with metrics
- Quality validation results
- Deployment readiness assessment
- Post-implementation recommendations

## Quality Guidelines

### Task Execution Best Practices

**Systematic Approach:**

- Execute tasks in dependency order
- Validate acceptance criteria before marking complete
- Maintain clean commit history with meaningful messages
- Document any deviations from original plan

**Error Recovery:**

- Implement graceful error handling
- Provide clear error messages with resolution guidance
- Maintain rollback capabilities where possible
- Document lessons learned from failures

## Dependencies

- Completed task breakdown (`/speckit-05-tasks`)
- Implementation plan with technical architecture
- Quality checklists (recommended but not required)

## Outputs

- Fully implemented feature according to specification
- Updated tasks.md with all tasks marked complete
- Implementation progress reports and validation results
- Deployment-ready codebase

## Next Steps

After running this command:

- Run `/speckit-07-analyze` for cross-artifact consistency validation
- Prepare for deployment using deployment guides
- Conduct final user acceptance testing
- Merge feature branch following project governance
