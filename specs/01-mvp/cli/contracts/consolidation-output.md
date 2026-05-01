# Contract: Dream Cycle Consolidation LLM Output

**Source:** CLI spec FR-8, FR-12 (Phase 3); CDK spec FR-10 (Phase 3)

## Overview

The dream cycle Phase 3 consolidation step sends each batch (one or more pending notes + related active notes) to an LLM for evaluation. The LLM decides what action to take for each pending note in the batch: promote it as-is, merge it into an existing note, split it into multiple notes, or consolidate multiple notes into a new one.

Phase 3 logic is shared between client mode (local KB) and server mode (CodeCommit KB). Only the Phase 1/2 grouping and search backends differ.

## Input

### System Prompt

A hardcoded consolidation prompt (versioned per CLI release) that instructs the LLM to:

1. Evaluate each pending note against the related active notes
2. Detect duplicates, overlaps, and opportunities for consolidation
3. Return a JSON object specifying actions to apply
4. Preserve content quality — never discard information silently

### User Message

A structured payload containing:

```
## Pending Notes (to evaluate)

### Note: <UID>
**Title:** <title>
**Author:** <author>
<content>

### Note: <UID>
...

## Related Active Notes (for context)

### Note: <UID>
**Title:** <title>
**Author:** <author>
<content>

### Note: <UID>
...
```

For local dream cycles (client mode), the "Pending Notes" section contains exactly one note (singleton batch). For server dream cycles, it may contain up to 10 similar pending notes grouped by Phase 1.

The "Related Active Notes" section contains up to 10 notes found by Phase 2 (git grep for local, OpenSearch similarity for server).

## Output

### Success: Valid JSON Object

```json
{
  "actions": [
    {
      "type": "keep",
      "source_uid": "01H5KXYZ9ABCDE",
      "reason": "Novel insight about DynamoDB TTL behavior not covered by existing notes"
    },
    {
      "type": "merge",
      "source_uid": "01H5KXYZ9FGHIJ",
      "target_uid": "01H3ABCD1KLMNO",
      "merged_content": "## Combined content\n\nOriginal active note content plus the new insight from the pending note...",
      "merged_title": "DynamoDB Global Tables configuration and permissions",
      "reason": "Pending note covers same topic as existing active note; combined for completeness"
    },
    {
      "type": "split",
      "source_uid": "01H5KXYZ9PQRST",
      "new_notes": [
        {
          "title": "Go context propagation patterns",
          "content": "## Pattern\n\nAlways pass context.Context as first parameter..."
        },
        {
          "title": "Go error wrapping conventions",
          "content": "## Convention\n\nUse fmt.Errorf with %w verb..."
        }
      ],
      "reason": "Pending note contained two distinct topics that should be separate notes"
    },
    {
      "type": "consolidate",
      "source_uids": ["01H5KXYZ9UVWXY", "01H5KXYZ9ZABCD"],
      "consolidated_note": {
        "title": "S3 lifecycle policy configuration",
        "content": "## Overview\n\nCombined knowledge from multiple overlapping notes about S3 lifecycle policies..."
      },
      "reason": "Two pending notes cover the same S3 lifecycle topic from different angles; merged into one comprehensive note"
    }
  ]
}
```

### Action Schema

Every pending note UID in the batch MUST appear in exactly one action. The LLM must not skip any pending note.

#### `keep`

Promote the pending note to `status: active` unchanged.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | `"keep"` | yes | Action type |
| `source_uid` | string | yes | UID of the pending note to promote |
| `reason` | string | yes | Brief explanation of why this note is kept as-is |

**Applied changes:**
- Set `status: active` on the source note
- Update `last-updated` to current timestamp

#### `merge`

Absorb a pending note's content into an existing active note.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | `"merge"` | yes | Action type |
| `source_uid` | string | yes | UID of the pending note to absorb (will be deleted) |
| `target_uid` | string | yes | UID of the existing active note to merge into |
| `merged_content` | string | yes | Full replacement content for the target note (Markdown) |
| `merged_title` | string | yes | Updated title for the target note (may be unchanged) |
| `reason` | string | yes | Brief explanation |

**Constraints:**
- `target_uid` MUST reference a note from the "Related Active Notes" section
- `merged_content` must incorporate information from both source and target

**Applied changes:**
- Replace target note's content and title with `merged_content` / `merged_title`
- Append source UID to target note's `consolidated-from-notes` frontmatter
- Update target note's `last-updated` to current timestamp
- Delete the source (pending) note file

#### `split`

Break a pending note into multiple new active notes.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | `"split"` | yes | Action type |
| `source_uid` | string | yes | UID of the pending note to split (will be deleted) |
| `new_notes` | array | yes | Array of new notes to create (minimum 2) |
| `new_notes[].title` | string | yes | Title for the new note (≤255 chars) |
| `new_notes[].content` | string | yes | Markdown content for the new note |
| `reason` | string | yes | Brief explanation |

**Applied changes:**
- Generate new UIDs for each entry in `new_notes`
- Create new note files with `status: active`, `consolidated-from-notes` referencing the source UID
- Delete the source (pending) note file

#### `consolidate`

Combine multiple pending (or pending + active) notes into a single new note.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | `"consolidate"` | yes | Action type |
| `source_uids` | string[] | yes | UIDs of notes to consolidate (minimum 2; at least one must be pending) |
| `consolidated_note` | object | yes | The new combined note |
| `consolidated_note.title` | string | yes | Title (≤255 chars) |
| `consolidated_note.content` | string | yes | Markdown content |
| `reason` | string | yes | Brief explanation |

**Constraints:**
- At least one `source_uids` entry must be a pending note from the batch
- `source_uids` may include active notes from the "Related Active Notes" section
- Every pending note referenced must be from the current batch

**Applied changes:**
- Generate new UID for the consolidated note
- Create new note file with `status: active`, `consolidated-from-notes` listing all source UIDs
- Delete all source note files (both pending and active sources)

### Distinction: `merge` vs. `consolidate`

| | `merge` | `consolidate` |
|---|---------|---------------|
| **Input** | 1 pending → 1 existing active | N notes (≥1 pending) → 1 new note |
| **Target** | Existing active note (preserves its UID) | New note (new UID) |
| **Source fate** | Pending note deleted | All source notes deleted |
| **Use case** | Pending note adds detail to an existing topic | Multiple notes overlap significantly; best rewritten as one |

## Error Handling

### Retry Logic

| Error Type | Retries | Action After Exhaustion |
|------------|---------|------------------------|
| Bedrock API failure (throttle, timeout) | 3 with exponential backoff | Skip batch; pending notes remain for next cycle |
| Malformed JSON output | 3 (fresh API call each) | Skip batch; pending notes remain |
| Valid JSON but missing pending UIDs | 0 (no retry) | Skip batch; log warning |

### Partial Acceptance

Unlike extraction, consolidation output is **all-or-nothing per batch**. If the JSON is valid but any action references an unknown UID or violates constraints:
- Skip the entire batch
- Log the error
- All pending notes in the batch remain `status: pending` for the next dream cycle

**Rationale:** Actions within a batch may be interdependent (e.g., consolidate references a note that another action merges). Applying partial actions could leave the KB in an inconsistent state.

## Per-Batch Git Commit

After applying all actions for a batch:
1. Stage all file changes (new files, modified files, deleted files)
2. Commit with message: `dream-cycle: <N> actions applied (<keep>K/<merge>M/<split>S/<consolidate>C)`
3. Proceed to next batch

If any action application fails (e.g., file I/O error):
- Already-applied actions in the current batch are committed
- Remaining actions in the batch are skipped
- The batch is logged as partially applied
- Remaining pending notes stay pending for next cycle
