# Contract: Approval Web Server API

**Source:** CLI spec FR-9

## Overview

On-demand local web server launched by `multi-kb approve`. Binds to `localhost` on an auto-selected port. Serves embedded HTML/CSS/JS assets and a REST API for managing the pending approval queue.

## Endpoints

### `GET /`

Serves the single-page approval UI (embedded HTML/CSS/JS).

**Response:** `200 OK` with `Content-Type: text/html`

---

### `GET /api/notes`

Returns all pending notes from `~/.multi-kb/pending/`.

**Response:** `200 OK`

```json
[
  {
    "filename": "20260501T103000-a3f7b2c1.json",
    "title": "DynamoDB Global Tables Configuration",
    "content": "## Overview\nGlobal Tables provide...",
    "author": "zmueller",
    "target_kbs": ["my-team-kb", "architecture-kb"],
    "source_conversation": "~/.claude/projects/my-project/abc123.jsonl",
    "extracted_at": "2026-05-01T10:30:00Z"
  }
]
```

Returns an empty array `[]` when no pending notes exist.

---

### `POST /api/notes/:filename/approve`

Approves a note for a specific target KB. Optionally accepts edited title/content.

**Request:**

```json
{
  "target_kb": "my-team-kb",
  "title": "DynamoDB Global Tables Configuration",
  "content": "## Overview\nGlobal Tables provide... (edited)"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `target_kb` | string | yes | Must be in the note's `target_kbs` array |
| `title` | string | yes | Original or edited title |
| `content` | string | yes | Original or edited content |

**Behavior:**
1. Submits the note (with title/content from request body) to the specified target KB:
   - Local KB: writes `<UID>.md` file with frontmatter + content
   - Remote KB: calls `submitKnowledge` API
2. Removes `target_kb` from the pending file's `target_kbs` array
3. Deletes the pending file if `target_kbs` is now empty
4. Edits apply to the pending file for remaining targets

**Response:** `200 OK`

```json
{
  "remaining_targets": ["architecture-kb"]
}
```

Or if no targets remain:

```json
{
  "remaining_targets": []
}
```

**Error Responses:**
- `404` — Pending file not found
- `400` — `target_kb` not in note's `target_kbs` array
- `502` — Submission to target KB failed (error details in body)

---

### `POST /api/notes/:filename/reject`

Rejects a note for a specific target KB.

**Request:**

```json
{
  "target_kb": "my-team-kb"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `target_kb` | string | yes | Must be in the note's `target_kbs` array |

**Behavior:**
1. Removes `target_kb` from the pending file's `target_kbs` array
2. Deletes the pending file if `target_kbs` is now empty
3. No submission to any KB

**Response:** `200 OK`

```json
{
  "remaining_targets": ["architecture-kb"]
}
```

**Error Responses:**
- `404` — Pending file not found
- `400` — `target_kb` not in note's `target_kbs` array

## Server Lifecycle

- **Startup:** Auto-selects available port, prints URL to terminal, opens default browser
- **Idle timeout:** Shuts down after configurable period (default: 5 minutes) with no browser activity
- **All resolved:** Shuts down immediately when all pending notes are resolved
- **Manual shutdown:** Ctrl+C in terminal always terminates immediately
- **No authentication:** Localhost-only, short-lived server — no auth required

## Activity Detection

The server tracks "browser activity" as any HTTP request to any endpoint. The idle timer resets on each request.
