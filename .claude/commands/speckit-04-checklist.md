# Checklist Workflow

Generate a custom checklist for the current feature based on user requirements.

## Description

This workflow creates quality validation checklists that serve as "unit tests for requirements writing." These checklists validate the quality, clarity, and completeness of requirements in specific domains rather than testing implementation.

## Core Concept: "Unit Tests for English"

**CRITICAL UNDERSTANDING:** Checklists are **UNIT TESTS FOR REQUIREMENTS WRITING** - they validate the quality, clarity, and completeness of requirements in a given domain.

**NOT for verification/testing:**

- NOT "Verify the button clicks correctly"
- NOT "Test error handling works"
- NOT "Confirm the API returns 200"
- NOT checking if code/implementation matches the spec

**FOR requirements quality validation:**

- "Are visual hierarchy requirements defined for all card types?" (completeness)
- "Is 'prominent display' quantified with specific sizing/positioning?" (clarity)
- "Are hover state requirements consistent across all interactive elements?" (consistency)
- "Are accessibility requirements defined for keyboard navigation?" (coverage)
- "Does the spec define what happens when logo image fails to load?" (edge cases)

## Usage

Run this command to:

- Generate domain-specific quality checklists (UX, API, security, performance, etc.)
- Validate requirement completeness and clarity
- Ensure specifications are ready for implementation
- Create systematic quality gates for requirements review

## Workflow Steps

### Step 1: Setup and Context Loading

Determine the spec directory location and load feature context:

**Determine Spec Directory:**

```bash
# Get current branch
BRANCH=$(git branch --show-current)

# Extract feature name from branch (e.g., feature/my-feature → my-feature)
# Spec directory: specs/{feature-name}/
```

**Load feature specification and check for existing checklists:**

```bash
# Verify spec.md exists
test -f "{spec-dir}/spec.md" && echo "Found spec" || echo "ERROR: spec.md not found"

# Load feature specification
cat "{spec-dir}/spec.md"

# Check for existing checklists
find "{spec-dir}/checklists" -name "*.md" 2>/dev/null || echo "No existing checklists"
```

**Establish {spec-dir} Reference:**
Throughout this workflow, `{spec-dir}` refers to the spec directory determined above based on your branch. All file operations will use this as the base path.

### Step 2: Clarify Checklist Intent

Derive up to THREE contextual clarifying questions based on user request and extracted signals:

**Question Generation Algorithm:**

1. **Extract Signals:** Feature domain keywords (auth, latency, UX, API), risk indicators ("critical", "must", "compliance"), stakeholder hints ("QA", "review", "security team"), explicit deliverables ("a11y", "rollback", "contracts")
2. **Cluster Signals:** Group into candidate focus areas (max 4) ranked by relevance
3. **Identify Audience & Timing:** Author, reviewer, QA, release gate
4. **Detect Missing Dimensions:** Scope breadth, depth/rigor, risk emphasis, exclusion boundaries, measurable acceptance criteria

**Question Archetypes:**

- **Scope Refinement:** "Should this include integration touchpoints with X and Y or stay limited to local module correctness?"
- **Risk Prioritization:** "Which of these potential risk areas should receive mandatory gating checks?"
- **Depth Calibration:** "Is this a lightweight pre-commit sanity list or a formal release gate?"
- **Audience Framing:** "Will this be used by the author only or peers during PR review?"
- **Boundary Exclusion:** "Should we explicitly exclude performance tuning items this round?"
- **Scenario Gap:** "No recovery flows detected—are rollback/partial failure paths in scope?"

**Defaults when interaction not possible:**

- **Depth:** Standard
- **Audience:** Reviewer (PR) if code-related; Author otherwise
- **Focus:** Top 2 relevance clusters

Skip questions individually if already unambiguous in user request. Present Q1/Q2/Q3 with compact option tables where appropriate.

### Step 3: Understand User Request

Combine user input with clarifying answers to determine:

- **Checklist Theme:** Security, UX, API, performance, etc.
- **Focus Areas:** Specific aspects to emphasize
- **Depth Level:** Lightweight vs. comprehensive validation
- **Target Audience:** Author, reviewer, QA team, release gate
- **Must-Have Items:** Explicitly mentioned requirements by user

### Step 4: Load Feature Context

Read relevant portions from feature specification:

**Context Loading Strategy:**

- Load only portions relevant to active focus areas (avoid full-file dumping)
- Summarize long sections into concise scenario/requirement bullets
- Use progressive disclosure - add follow-on retrieval only if gaps detected
- If source docs are large, generate interim summary items

**Key Elements to Extract:**

- **spec.md:** Feature requirements, user scenarios, success criteria, edge cases
- **plan.md (if exists):** Technical architecture, security requirements, integration points
- **data-model.md (if exists):** Entity relationships, validation rules

### Step 5: Generate Requirements Quality Checklist

Create systematic "unit tests for requirements" grouped by quality dimensions:

**Category Structure:**

- **Requirement Completeness** (Are all necessary requirements documented?)
- **Requirement Clarity** (Are requirements specific and unambiguous?)
- **Requirement Consistency** (Do requirements align without conflicts?)
- **Acceptance Criteria Quality** (Are success criteria measurable?)
- **Scenario Coverage** (Are all flows/cases addressed?)
- **Edge Case Coverage** (Are boundary conditions defined?)
- **Non-Functional Requirements** (Performance, Security, Accessibility specified?)
- **Dependencies & Assumptions** (Are they documented and validated?)
- **Ambiguities & Conflicts** (What needs clarification?)

**Checklist Generation Process:**

1. **Create Directory:** `{spec-path}/checklists/` if it doesn't exist
2. **Generate Filename:** Use domain-based name (e.g., `ux.md`, `api.md`, `security.md`)
3. **Number Items:** Sequential numbering starting from CHK001
4. **Each Run Creates New File:** Never overwrite existing checklists

### Step 6: Write Effective Checklist Items

Each item must follow the "Unit Tests for English" pattern:

**CORRECT Pattern - Testing Requirements Quality:**

```markdown
- [ ] CHK001 - Are the number and layout of featured episodes explicitly specified? [Completeness, Spec §FR-001]
- [ ] CHK002 - Are hover state requirements consistently defined for all interactive elements? [Consistency, Spec §FR-003]
- [ ] CHK003 - Are navigation requirements clear for all clickable brand elements? [Clarity, Spec §FR-010]
- [ ] CHK004 - Is the selection criteria for related episodes documented? [Gap, Spec §FR-005]
- [ ] CHK005 - Are loading state requirements defined for asynchronous episode data? [Gap]
- [ ] CHK006 - Can "visual hierarchy" requirements be objectively measured? [Measurability, Spec §FR-001]
```

**WRONG Pattern - Testing Implementation:**

```markdown
- [ ] CHK001 - Verify landing page displays 3 episode cards [Spec §FR-001]
- [ ] CHK002 - Test hover states work correctly on desktop [Spec §FR-003]
- [ ] CHK003 - Confirm logo click navigates to home page [Spec §FR-010]
- [ ] CHK004 - Check that related episodes section shows 3-5 items [Spec §FR-005]
```

**Item Structure Requirements:**

- **Question Format:** Ask about requirement quality (not implementation behavior)
- **Quality Dimension:** Include in brackets [Completeness/Clarity/Consistency/etc.]
- **Traceability Reference:** Include spec section `[Spec §X.Y]` or gap marker `[Gap]`
- **Focus on Written Requirements:** What's documented (or missing) in the spec

### Step 7: Traceability and Coverage Requirements

Ensure comprehensive requirement coverage:

**Traceability Standards:**

- **MINIMUM:** ≥80% of items MUST include traceability reference
- **Reference Types:** Spec section `[Spec §X.Y]`, gap markers `[Gap]`, issue types `[Ambiguity]`, `[Conflict]`, `[Assumption]`
- **ID System:** If no requirement ID scheme exists, include: "Is a requirement & acceptance criteria ID scheme established? [Traceability]"

**Scenario Coverage Validation:** Check requirements exist for scenario classes:

- **Primary Scenarios:** Happy path user flows
- **Alternate Scenarios:** Alternative user paths and choices
- **Exception/Error Scenarios:** Error conditions and failure modes
- **Recovery Scenarios:** Rollback and error recovery procedures
- **Non-Functional Scenarios:** Performance, security, accessibility

### Step 8: Checklist Structure and Format

Use canonical checklist template structure:

```markdown
# [Domain] Requirements Quality Checklist: [FEATURE NAME]

**Purpose:** Validate [domain] requirements completeness and quality before proceeding to implementation
**Created:** [DATE]
**Feature:** [Link to spec.md]
**Focus:** [Specific focus areas for this checklist]

## Requirement Completeness
- [ ] CHK001 - [Completeness check item] [Completeness, Spec §X.Y]
- [ ] CHK002 - [Another completeness item] [Gap]

## Requirement Clarity
- [ ] CHK003 - [Clarity check item] [Clarity, Spec §X.Y]
- [ ] CHK004 - [Ambiguity resolution item] [Ambiguity, Spec §X.Y]

## Requirement Consistency
- [ ] CHK005 - [Consistency check item] [Consistency, Spec §X.Y]
- [ ] CHK006 - [Cross-reference validation] [Consistency]

## Acceptance Criteria Quality
- [ ] CHK007 - [Measurability check] [Measurability, Spec §X.Y]
- [ ] CHK008 - [Testability validation] [Acceptance Criteria, Spec §X.Y]

## Scenario Coverage
- [ ] CHK009 - [Primary scenario coverage] [Coverage, Spec §X.Y]
- [ ] CHK010 - [Edge case coverage] [Coverage, Gap]

## [Additional Domain-Specific Categories]
- [ ] CHK011 - [Domain-specific item] [Category, Reference]

## Notes
- Items marked incomplete require spec updates before proceeding to implementation
- Focus areas: [List the specific areas emphasized in this checklist]
- Coverage: [Summary of what scenarios/requirements are covered]
```

### Step 9: Content Consolidation and Quality Control

Optimize checklist content for effectiveness:

**Content Management:**

- **Soft Cap:** If raw candidate items > 40, prioritize by risk/impact
- **Merge Duplicates:** Combine near-duplicates checking same requirement aspect
- **Consolidate Edge Cases:** If >5 low-impact edge cases, create single item: "Are edge cases X, Y, Z addressed in requirements? [Coverage]"
- **Global ID Sequence:** Use incrementing CHK### IDs across all items

**ABSOLUTELY PROHIBITED Patterns:**

- Items starting with "Verify", "Test", "Confirm", "Check" + implementation behavior
- References to code execution, user actions, system behavior
- "Displays correctly", "works properly", "functions as expected"
- "Click", "navigate", "render", "load", "execute"
- Test cases, test plans, QA procedures
- Implementation details (frameworks, APIs, algorithms)

**REQUIRED Patterns:**

- "Are [requirement type] defined/specified/documented for [scenario]?"
- "Is [vague term] quantified/clarified with specific criteria?"
- "Are requirements consistent between [section A] and [section B]?"
- "Can [requirement] be objectively measured/verified?"
- "Are [edge cases/scenarios] addressed in requirements?"
- "Does the spec define [missing aspect]?"

### Step 10: Finalize and Report

Complete checklist creation and provide summary:

**Commit Changes:**

Follow the commit standards defined in `.claude/rules/git.md` to commit the new checklist file(s).

**Completion Report:**

- Full path to created checklist file
- Total item count and focus areas
- Coverage summary (what requirement areas are validated)
- Reminder that each run creates a new file
- Next steps for using the checklist

## Dependencies

- Active feature branch with specification
- Completed `/speckit-01-specify` command
- Optional: `/speckit-03-plan` command for technical requirements

## Outputs

- Domain-specific checklist file: `{spec-path}/checklists/[domain].md`
- Requirements quality validation items
- Traceability references to specification sections
- Coverage analysis for requirement completeness

## Next Steps

After running this command:

- Use checklist to validate specification quality
- Address any incomplete items before implementation
- Run additional domain checklists as needed
- Proceed to implementation once quality gates pass
