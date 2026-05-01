# Contract: Local KB Recall Ranking

**Source:** CLI spec FR-7, FR-8

## Purpose

Defines the scoring algorithm used to rank local KB recall results for interleaving with remote KB results during hook-based recall (FR-7).

## Keyword Derivation

For hook-based recall, the CLI calls the translation summarization model (`translation.summarization_model_id`) to derive 3–5 search keywords from the user's natural language query.

For dream cycle Phase 2 recall, keywords are derived mechanically from the note's title and key terms (no LLM call).

## Search Method

Local KB recall uses `git grep` against the working tree, filtered to `status: active` notes only. Each keyword is searched independently via a separate `git grep` invocation.

## Scoring Formula

```
score = (whole_word_title_matches * 3) + (whole_word_body_matches * 1)
```

Where:
- **whole_word_title_matches:** Count of keyword matches found in the note's YAML frontmatter `title` field, summed across all keywords
- **whole_word_body_matches:** Count of keyword matches found in the note body (everything below the frontmatter `---` delimiter), summed across all keywords
- Matching uses case-insensitive whole-word regex: `(?i)\b{keyword}\b`
- Each keyword occurrence is counted independently (if "deploy" appears twice in the title, that contributes 6 to the score)

## Tie-Breaking

When two or more notes have the same score, sort by `last_updated` frontmatter timestamp descending (newest first).

## Result Limit

Return the top 10 scored notes (configurable via hook `limit` parameter, default 10).

## Interleaving with Remote Results

Local KB results are interleaved with remote KB results using rank-based interleaving (round-robin by rank position across all KBs). Deduplication by UID is not needed because local KB UIDs are independent of remote KB UIDs.
