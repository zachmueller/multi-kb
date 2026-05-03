#!/usr/bin/env bash
# QAT-006: Bedrock KB Metadata Extraction Verification
#
# Validates the critical assumption that Bedrock KB extracts YAML frontmatter
# fields (uid, title) as queryable metadata in Retrieve API responses.
#
# This is the SINGLE HIGHEST-RISK integration point. If metadata extraction
# does not work, both CDK recallKnowledge Lambda and CLI recall flows need rework.
#
# Prerequisites:
#   - Stack deployed via `cdk deploy`
#   - AWS CLI configured with valid credentials
#   - jq installed
#
# Usage:
#   ./qat-006-metadata-extraction.sh <stack-name>
#   ./qat-006-metadata-extraction.sh MultiKbStack

set -euo pipefail

STACK_NAME="${1:?Usage: $0 <stack-name>}"
REGION="${AWS_DEFAULT_REGION:-us-east-1}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

pass() { echo -e "${GREEN}[PASS]${NC} $1"; }
fail() { echo -e "${RED}[FAIL]${NC} $1"; FAILURES=$((FAILURES + 1)); }
info() { echo -e "${YELLOW}[INFO]${NC} $1"; }

FAILURES=0

# --- Step 1: Resolve stack outputs ---
info "Resolving stack outputs for ${STACK_NAME}..."

get_output() {
  aws cloudformation describe-stacks \
    --stack-name "$STACK_NAME" \
    --region "$REGION" \
    --query "Stacks[0].Outputs[?contains(OutputKey,'$1')].OutputValue | [0]" \
    --output text
}

BUCKET_NAME=$(get_output "BucketName")
KB_ID=$(get_output "KnowledgeBaseId")
DS_ID=$(get_output "DataSourceId")

if [[ -z "$BUCKET_NAME" || -z "$KB_ID" || -z "$DS_ID" ]]; then
  echo "ERROR: Could not resolve required stack outputs (BucketName, KnowledgeBaseId, DataSourceId)"
  echo "  BUCKET_NAME=${BUCKET_NAME:-<missing>}"
  echo "  KB_ID=${KB_ID:-<missing>}"
  echo "  DS_ID=${DS_ID:-<missing>}"
  exit 1
fi

info "Bucket: ${BUCKET_NAME}"
info "Knowledge Base ID: ${KB_ID}"
info "Data Source ID: ${DS_ID}"

# --- Step 2: Upload test note with YAML frontmatter ---
TEST_UID="TEST00000000001"
TEST_TITLE="QAT-006 Metadata Extraction Test Note"
TEST_CONTENT="Bedrock knowledge base metadata extraction verification for multi-kb integration testing."

TEST_NOTE=$(cat <<'NOTEEOF'
---
uid: "TEST00000000001"
title: "QAT-006 Metadata Extraction Test Note"
status: active
author: "qat-006-test"
last-updated: "2026-05-03T00:00:00Z"
last-linked-to:
last-recalled:
consolidated-from-notes:
---

# QAT-006 Metadata Extraction Test Note

Bedrock knowledge base metadata extraction verification for multi-kb integration testing.

This note contains unique content about zebrafish neural pathway reconstruction
using quantum fluorescence microscopy, a topic unlikely to match any other note
in the knowledge base, ensuring clean test isolation.
NOTEEOF
)

info "Uploading test note ${TEST_UID}.md to S3..."
echo "$TEST_NOTE" | aws s3 cp - "s3://${BUCKET_NAME}/${TEST_UID}.md" \
  --content-type "text/markdown" \
  --region "$REGION"
pass "Test note uploaded to s3://${BUCKET_NAME}/${TEST_UID}.md"

# --- Step 3: Trigger data source sync ---
info "Starting ingestion job..."
INGESTION_JOB=$(aws bedrock-agent start-ingestion-job \
  --knowledge-base-id "$KB_ID" \
  --data-source-id "$DS_ID" \
  --region "$REGION" \
  --output json)

JOB_ID=$(echo "$INGESTION_JOB" | jq -r '.ingestionJob.ingestionJobId')
info "Ingestion job started: ${JOB_ID}"

# Poll for completion (max 10 minutes)
POLL_INTERVAL=15
MAX_POLLS=40
POLL_COUNT=0

while true; do
  POLL_COUNT=$((POLL_COUNT + 1))
  if [[ $POLL_COUNT -gt $MAX_POLLS ]]; then
    fail "Ingestion job timed out after $((MAX_POLLS * POLL_INTERVAL)) seconds"
    break
  fi

  JOB_STATUS=$(aws bedrock-agent get-ingestion-job \
    --knowledge-base-id "$KB_ID" \
    --data-source-id "$DS_ID" \
    --ingestion-job-id "$JOB_ID" \
    --region "$REGION" \
    --query 'ingestionJob.status' \
    --output text)

  info "Ingestion status: ${JOB_STATUS} (poll ${POLL_COUNT}/${MAX_POLLS})"

  case "$JOB_STATUS" in
    COMPLETE)
      pass "Ingestion job completed successfully"
      break
      ;;
    FAILED)
      FAILURE_REASONS=$(aws bedrock-agent get-ingestion-job \
        --knowledge-base-id "$KB_ID" \
        --data-source-id "$DS_ID" \
        --ingestion-job-id "$JOB_ID" \
        --region "$REGION" \
        --query 'ingestionJob.failureReasons' \
        --output json)
      fail "Ingestion job failed: ${FAILURE_REASONS}"
      break
      ;;
    *)
      sleep "$POLL_INTERVAL"
      ;;
  esac
done

# --- Step 4: Call Retrieve API ---
info "Calling Bedrock Retrieve API..."

# Wait a brief moment for index refresh after ingestion
sleep 5

RETRIEVE_RESPONSE=$(aws bedrock-agent-runtime retrieve \
  --knowledge-base-id "$KB_ID" \
  --retrieval-query '{"text": "zebrafish neural pathway quantum fluorescence microscopy"}' \
  --region "$REGION" \
  --output json 2>&1) || true

if [[ -z "$RETRIEVE_RESPONSE" ]] || echo "$RETRIEVE_RESPONSE" | jq -e '.retrievalResults | length == 0' >/dev/null 2>&1; then
  fail "Retrieve returned no results — the test note may not have been indexed"
  info "Response: ${RETRIEVE_RESPONSE}"
else
  pass "Retrieve returned results"
  RESULT_COUNT=$(echo "$RETRIEVE_RESPONSE" | jq '.retrievalResults | length')
  info "Result count: ${RESULT_COUNT}"
fi

# --- Step 5: Verify metadata fields ---
if [[ $FAILURES -eq 0 ]] || echo "$RETRIEVE_RESPONSE" | jq -e '.retrievalResults | length > 0' >/dev/null 2>&1; then
  info "Checking metadata fields in first result..."

  FIRST_RESULT=$(echo "$RETRIEVE_RESPONSE" | jq '.retrievalResults[0]')

  # Check if metadata exists
  HAS_METADATA=$(echo "$FIRST_RESULT" | jq 'has("metadata")')
  if [[ "$HAS_METADATA" == "true" ]]; then
    pass "Result has metadata field"
  else
    fail "Result does NOT have metadata field"
    info "Full result structure: $(echo "$FIRST_RESULT" | jq 'keys')"
  fi

  # Check for uid in metadata
  METADATA_UID=$(echo "$FIRST_RESULT" | jq -r '.metadata.uid // empty')
  if [[ "$METADATA_UID" == "$TEST_UID" ]]; then
    pass "metadata.uid = '${METADATA_UID}' matches expected '${TEST_UID}'"
  elif [[ -n "$METADATA_UID" ]]; then
    fail "metadata.uid = '${METADATA_UID}' does NOT match expected '${TEST_UID}'"
  else
    # Try alternative paths
    info "metadata.uid not found at expected path, checking alternatives..."
    ALT_UID=$(echo "$FIRST_RESULT" | jq -r '
      .metadata // {} |
      to_entries[] |
      select(.key | test("uid"; "i")) |
      .value
    ' 2>/dev/null || true)
    if [[ -n "$ALT_UID" ]]; then
      info "Found uid at alternative metadata key: ${ALT_UID}"
      fail "metadata.uid not at expected path .metadata.uid — found via alternative key search"
    else
      # Check if metadata is nested in a different structure
      info "Dumping full metadata structure for analysis..."
      echo "$FIRST_RESULT" | jq '.metadata' 2>/dev/null || echo "(no metadata)"
      fail "metadata.uid NOT found in Retrieve response — YAML frontmatter may not be extracted as metadata"
    fi
  fi

  # Check for title in metadata
  METADATA_TITLE=$(echo "$FIRST_RESULT" | jq -r '.metadata.title // empty')
  if [[ "$METADATA_TITLE" == "$TEST_TITLE" ]]; then
    pass "metadata.title = '${METADATA_TITLE}' matches expected"
  elif [[ -n "$METADATA_TITLE" ]]; then
    fail "metadata.title = '${METADATA_TITLE}' does NOT match expected '${TEST_TITLE}'"
  else
    info "metadata.title not found at expected path, checking alternatives..."
    ALT_TITLE=$(echo "$FIRST_RESULT" | jq -r '
      .metadata // {} |
      to_entries[] |
      select(.key | test("title"; "i")) |
      .value
    ' 2>/dev/null || true)
    if [[ -n "$ALT_TITLE" ]]; then
      info "Found title at alternative metadata key: ${ALT_TITLE}"
      fail "metadata.title not at expected path .metadata.title — found via alternative key search"
    else
      echo "$FIRST_RESULT" | jq '.metadata' 2>/dev/null || echo "(no metadata)"
      fail "metadata.title NOT found in Retrieve response"
    fi
  fi

  # Check content field
  CONTENT_TEXT=$(echo "$FIRST_RESULT" | jq -r '.content.text // empty')
  if [[ -n "$CONTENT_TEXT" ]]; then
    pass "content.text is present (${#CONTENT_TEXT} chars)"
  else
    fail "content.text is missing from Retrieve response"
  fi

  # Check score field
  SCORE=$(echo "$FIRST_RESULT" | jq -r '.score // empty')
  if [[ -n "$SCORE" ]]; then
    pass "score is present: ${SCORE}"
  else
    fail "score is missing from Retrieve response"
  fi

  # Dump full response structure for documentation
  info "Full first result structure:"
  echo "$FIRST_RESULT" | jq '.'
fi

# --- Step 6: Cleanup test note ---
info "Cleaning up test note from S3..."
aws s3 rm "s3://${BUCKET_NAME}/${TEST_UID}.md" --region "$REGION" 2>/dev/null || true

# --- Summary ---
echo ""
echo "======================================"
echo "QAT-006 Metadata Extraction Test Summary"
echo "======================================"
if [[ $FAILURES -eq 0 ]]; then
  pass "ALL CHECKS PASSED — Bedrock KB correctly extracts YAML frontmatter as queryable metadata"
  echo ""
  echo "The recallKnowledge Lambda field mapping is validated:"
  echo "  retrievalResults[].metadata.uid   -> uid"
  echo "  retrievalResults[].metadata.title -> title"
  echo "  retrievalResults[].content.text   -> content"
  echo "  retrievalResults[].score          -> score"
else
  fail "${FAILURES} check(s) FAILED"
  echo ""
  echo "ACTION REQUIRED: If metadata extraction does not work as expected,"
  echo "update contracts/recall-knowledge.md field mapping and rework:"
  echo "  - CDK LMB-004 (recallKnowledge Lambda handler)"
  echo "  - CLI HKI-003 (remote recall client)"
  echo "  - CLI HKI-005 (result interleaving)"
  echo ""
  echo "Possible fallbacks:"
  echo "  1. Parse uid/title from content.text (frontmatter is in the text chunk)"
  echo "  2. Use S3 metadata tags instead of YAML frontmatter"
  echo "  3. Store uid in the S3 object key and derive from location.s3Location"
fi

exit $FAILURES
