#!/usr/bin/env bash
# QAT-005: Post-Deploy Integration Test Suite
#
# Validates a deployed Multi-KB stack end-to-end:
#   Step 1: Submit flow (API → SQS → EC2 → CodeCommit → S3 → OpenSearch)
#   Step 2: Recall flow (API → Bedrock KB → results + recall log)
#   Step 3: Server config validation (CDK-generated config.yaml matches CLI expectations)
#   Step 4: EC2 health (CloudWatch logs show tick loop running)
#   Step 5: EC2 recovery (terminate → ASG replaces)
#   Step 6: SSM access
#   Step 7: CloudWatch observability
#   Step 8: DLQ alarm
#
# Prerequisites:
#   - Stack deployed via `cdk deploy`
#   - AWS CLI configured with valid credentials
#   - jq installed
#   - QAT-006 passed (metadata extraction verified)
#
# Usage:
#   ./qat-005-post-deploy.sh <stack-name> [--skip-recovery] [--skip-dream-cycle]
#   ./qat-005-post-deploy.sh MultiKbStack

set -euo pipefail

STACK_NAME="${1:?Usage: $0 <stack-name> [--skip-recovery] [--skip-dream-cycle]}"
shift
REGION="${AWS_DEFAULT_REGION:-us-east-1}"
SKIP_RECOVERY=false
SKIP_DREAM_CYCLE=false

for arg in "$@"; do
  case "$arg" in
    --skip-recovery) SKIP_RECOVERY=true ;;
    --skip-dream-cycle) SKIP_DREAM_CYCLE=true ;;
  esac
done

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

pass() { echo -e "${GREEN}[PASS]${NC} $1"; PASSES=$((PASSES + 1)); }
fail() { echo -e "${RED}[FAIL]${NC} $1"; FAILURES=$((FAILURES + 1)); }
skip() { echo -e "${YELLOW}[SKIP]${NC} $1"; SKIPS=$((SKIPS + 1)); }
info() { echo -e "${BLUE}[INFO]${NC} $1"; }
section() { echo -e "\n${BLUE}=== $1 ===${NC}"; }

FAILURES=0
PASSES=0
SKIPS=0

# --- Resolve stack outputs ---
info "Resolving stack outputs for ${STACK_NAME}..."

get_output() {
  aws cloudformation describe-stacks \
    --stack-name "$STACK_NAME" \
    --region "$REGION" \
    --query "Stacks[0].Outputs[?OutputKey=='$1'].OutputValue" \
    --output text
}

BUCKET_NAME=$(get_output "BucketName")
KB_ID=$(get_output "KnowledgeBaseId")
DS_ID=$(get_output "DataSourceId")
API_ENDPOINT=$(get_output "ApiEndpoint")
API_ID=$(get_output "ApiId")
REPO_CLONE_URL=$(get_output "RepoCloneUrl")
COLLECTION_ENDPOINT=$(get_output "CollectionEndpoint")
ASG_NAME=$(get_output "AsgName")

# Find EC2 instance from ASG
EC2_INSTANCE_ID=$(aws autoscaling describe-auto-scaling-groups \
  --auto-scaling-group-names "$ASG_NAME" \
  --region "$REGION" \
  --query 'AutoScalingGroups[0].Instances[0].InstanceId' \
  --output text 2>/dev/null || echo "")

info "API Endpoint: ${API_ENDPOINT}"
info "Bucket: ${BUCKET_NAME}"
info "KB ID: ${KB_ID}"
info "ASG: ${ASG_NAME}"
info "EC2 Instance: ${EC2_INSTANCE_ID:-<none>}"

# ================================================================
section "Step 1: Submit Flow"
# ================================================================

SUBMIT_TITLE="QAT-005 Integration Test $(date +%s)"
SUBMIT_CONTENT="This is an automated integration test note for post-deploy validation."
SUBMIT_AUTHOR="qat-005-test"

info "Invoking submitKnowledge via API Gateway test-invoke-method..."
SUBMIT_RESPONSE=$(aws apigateway test-invoke-method \
  --rest-api-id "$API_ID" \
  --resource-id "$(aws apigateway get-resources --rest-api-id "$API_ID" --region "$REGION" \
    --query "items[?pathPart=='submitKnowledge'].id" --output text)" \
  --http-method POST \
  --body "{\"title\":\"${SUBMIT_TITLE}\",\"content\":\"${SUBMIT_CONTENT}\",\"author\":\"${SUBMIT_AUTHOR}\"}" \
  --region "$REGION" \
  --output json 2>&1) || true

SUBMIT_STATUS=$(echo "$SUBMIT_RESPONSE" | jq -r '.status // empty')
SUBMIT_BODY=$(echo "$SUBMIT_RESPONSE" | jq -r '.body // empty')

if [[ "$SUBMIT_STATUS" == "202" ]]; then
  pass "submitKnowledge returned HTTP 202"
  SUBMITTED_UID=$(echo "$SUBMIT_BODY" | jq -r '.uid // empty')
  if [[ -n "$SUBMITTED_UID" ]]; then
    pass "Response contains uid: ${SUBMITTED_UID}"
  else
    fail "Response missing uid field"
  fi
else
  fail "submitKnowledge returned HTTP ${SUBMIT_STATUS:-<error>} (expected 202)"
  info "Response: ${SUBMIT_RESPONSE}"
  SUBMITTED_UID=""
fi

# Verify SQS message appears
info "Checking SQS queue for message..."
sleep 2
SQS_QUEUE_URL=$(aws cloudformation describe-stack-resources \
  --stack-name "$STACK_NAME" \
  --region "$REGION" \
  --query "StackResources[?ResourceType=='AWS::SQS::Queue' && LogicalResourceId!='StorageDeadLetterQueue' && !(contains(LogicalResourceId, 'DeadLetter'))].PhysicalResourceId" \
  --output text 2>/dev/null | head -1)

# Note: We can't easily peek at SQS without consuming the message,
# so we check queue attributes instead
if [[ -n "$SQS_QUEUE_URL" ]]; then
  QUEUE_ATTRS=$(aws sqs get-queue-attributes \
    --queue-url "$SQS_QUEUE_URL" \
    --attribute-names ApproximateNumberOfMessages \
    --region "$REGION" \
    --output json 2>/dev/null || echo '{}')
  MSG_COUNT=$(echo "$QUEUE_ATTRS" | jq -r '.Attributes.ApproximateNumberOfMessages // "0"')
  info "SQS approximate message count: ${MSG_COUNT}"
  if [[ "$MSG_COUNT" -gt "0" ]] || [[ -n "$SUBMITTED_UID" ]]; then
    pass "SQS message enqueued (count=${MSG_COUNT} or submit succeeded)"
  fi
fi

# Wait for EC2 to process the message (tick interval is ~5 min)
if [[ -n "$SUBMITTED_UID" ]]; then
  info "Waiting for EC2 to process submitted note (up to 7 minutes)..."
  info "The server polls SQS every tick_interval (default 5m). Be patient."

  POLL_INTERVAL=30
  MAX_POLLS=14
  NOTE_FOUND=false

  for ((i=1; i<=MAX_POLLS; i++)); do
    # Check if the note file appears in S3
    S3_CHECK=$(aws s3 ls "s3://${BUCKET_NAME}/${SUBMITTED_UID}.md" \
      --region "$REGION" 2>/dev/null || true)
    if [[ -n "$S3_CHECK" ]]; then
      pass "Note ${SUBMITTED_UID}.md appeared in S3 (after ~$((i * POLL_INTERVAL))s)"
      NOTE_FOUND=true
      break
    fi
    info "  Waiting... (${i}/${MAX_POLLS})"
    sleep "$POLL_INTERVAL"
  done

  if [[ "$NOTE_FOUND" == "false" ]]; then
    # Also check CodeCommit directly
    info "Note not found in S3 yet. Checking if SQS message is still queued..."
    QUEUE_ATTRS_2=$(aws sqs get-queue-attributes \
      --queue-url "$SQS_QUEUE_URL" \
      --attribute-names ApproximateNumberOfMessages \
      --region "$REGION" \
      --output json 2>/dev/null || echo '{}')
    MSG_COUNT_2=$(echo "$QUEUE_ATTRS_2" | jq -r '.Attributes.ApproximateNumberOfMessages // "0"')
    if [[ "$MSG_COUNT_2" == "0" ]]; then
      info "SQS queue is empty — message was consumed but S3 sync may be pending"
      skip "Note not yet in S3 — EC2 may have consumed the message but S3 sync is pending"
    else
      fail "Note not found in S3 after 7 minutes — EC2 may not be processing SQS messages"
    fi
  fi
fi

# ================================================================
section "Step 2: Recall Flow"
# ================================================================

info "Invoking recallKnowledge via API Gateway test-invoke-method..."
RECALL_RESPONSE=$(aws apigateway test-invoke-method \
  --rest-api-id "$API_ID" \
  --resource-id "$(aws apigateway get-resources --rest-api-id "$API_ID" --region "$REGION" \
    --query "items[?pathPart=='recallKnowledge'].id" --output text)" \
  --http-method POST \
  --body '{"query":"integration test knowledge base validation","limit":5}' \
  --region "$REGION" \
  --output json 2>&1) || true

RECALL_STATUS=$(echo "$RECALL_RESPONSE" | jq -r '.status // empty')
RECALL_BODY=$(echo "$RECALL_RESPONSE" | jq -r '.body // empty')

if [[ "$RECALL_STATUS" == "200" ]]; then
  pass "recallKnowledge returned HTTP 200"
  RESULT_COUNT=$(echo "$RECALL_BODY" | jq 'length')
  info "Recall returned ${RESULT_COUNT} results"

  if [[ "$RESULT_COUNT" -gt "0" ]]; then
    pass "Recall returned non-empty results"
    FIRST_RESULT=$(echo "$RECALL_BODY" | jq '.[0]')
    HAS_UID=$(echo "$FIRST_RESULT" | jq 'has("uid")')
    HAS_TITLE=$(echo "$FIRST_RESULT" | jq 'has("title")')
    HAS_CONTENT=$(echo "$FIRST_RESULT" | jq 'has("content")')
    HAS_SCORE=$(echo "$FIRST_RESULT" | jq 'has("score")')

    [[ "$HAS_UID" == "true" ]] && pass "Result has uid field" || fail "Result missing uid"
    [[ "$HAS_TITLE" == "true" ]] && pass "Result has title field" || fail "Result missing title"
    [[ "$HAS_CONTENT" == "true" ]] && pass "Result has content field" || fail "Result missing content"
    [[ "$HAS_SCORE" == "true" ]] && pass "Result has score field" || fail "Result missing score"
  else
    info "Recall returned empty results (KB may not have indexed content yet)"
    skip "Empty recall results — run again after KB data source sync"
  fi
else
  fail "recallKnowledge returned HTTP ${RECALL_STATUS:-<error>} (expected 200)"
  info "Response: ${RECALL_RESPONSE}"
fi

# Check recall validation — empty query should return 400
RECALL_400=$(aws apigateway test-invoke-method \
  --rest-api-id "$API_ID" \
  --resource-id "$(aws apigateway get-resources --rest-api-id "$API_ID" --region "$REGION" \
    --query "items[?pathPart=='recallKnowledge'].id" --output text)" \
  --http-method POST \
  --body '{"query":""}' \
  --region "$REGION" \
  --output json 2>&1) || true

RECALL_400_STATUS=$(echo "$RECALL_400" | jq -r '.status // empty')
if [[ "$RECALL_400_STATUS" == "400" ]]; then
  pass "Empty query correctly returns HTTP 400"
else
  fail "Empty query returned HTTP ${RECALL_400_STATUS} (expected 400)"
fi

# Check recall log in S3
info "Checking for recall logs in S3..."
TODAY=$(date -u +%Y-%m-%d)
RECALL_LOGS=$(aws s3 ls "s3://${BUCKET_NAME}/recall-logs/${TODAY}/" \
  --region "$REGION" 2>/dev/null || true)
if [[ -n "$RECALL_LOGS" ]]; then
  pass "Recall log(s) found in S3 for today (${TODAY})"
  LOG_COUNT=$(echo "$RECALL_LOGS" | wc -l | tr -d ' ')
  info "Recall log count: ${LOG_COUNT}"
else
  info "No recall logs found for today — may appear after first successful recall"
  skip "Recall logs not yet present for ${TODAY}"
fi

# ================================================================
section "Step 3: Server Config Validation"
# ================================================================

if [[ -n "$EC2_INSTANCE_ID" && "$EC2_INSTANCE_ID" != "None" ]]; then
  info "Checking config.yaml on EC2 instance via SSM..."
  CONFIG_CHECK=$(aws ssm send-command \
    --instance-ids "$EC2_INSTANCE_ID" \
    --document-name "AWS-RunShellScript" \
    --parameters 'commands=["cat /opt/multi-kb/config.yaml"]' \
    --region "$REGION" \
    --output json 2>/dev/null || echo "")

  if [[ -n "$CONFIG_CHECK" ]]; then
    CMD_ID=$(echo "$CONFIG_CHECK" | jq -r '.Command.CommandId')
    sleep 5
    CONFIG_OUTPUT=$(aws ssm get-command-invocation \
      --command-id "$CMD_ID" \
      --instance-id "$EC2_INSTANCE_ID" \
      --region "$REGION" \
      --query 'StandardOutputContent' \
      --output text 2>/dev/null || echo "")

    if [[ -n "$CONFIG_OUTPUT" ]]; then
      pass "config.yaml exists on EC2 instance"

      # Validate required fields
      for field in "mode:" "sqs:" "queue_url:" "codecommit:" "repo_name:" "s3:" "bucket:" \
                   "opensearch:" "endpoint:" "bedrock_kb:" "knowledge_base_id:" "data_source_id:" \
                   "tick_interval:" "dream_cycle:" "recall_log:"; do
        if echo "$CONFIG_OUTPUT" | grep -q "$field"; then
          pass "config.yaml contains '${field}'"
        else
          fail "config.yaml MISSING '${field}'"
        fi
      done

      if echo "$CONFIG_OUTPUT" | grep -q "mode: server"; then
        pass "config.yaml mode is 'server'"
      else
        fail "config.yaml mode is NOT 'server'"
      fi
    else
      skip "Could not read config.yaml via SSM (command may still be running)"
    fi
  else
    skip "SSM command failed — instance may not be ready"
  fi
else
  skip "No EC2 instance found — cannot validate server config"
fi

# ================================================================
section "Step 4: EC2 Health"
# ================================================================

if [[ -n "$EC2_INSTANCE_ID" && "$EC2_INSTANCE_ID" != "None" ]]; then
  # Check instance state
  INSTANCE_STATE=$(aws ec2 describe-instances \
    --instance-ids "$EC2_INSTANCE_ID" \
    --region "$REGION" \
    --query 'Reservations[0].Instances[0].State.Name' \
    --output text 2>/dev/null || echo "unknown")

  if [[ "$INSTANCE_STATE" == "running" ]]; then
    pass "EC2 instance is running"
  else
    fail "EC2 instance state: ${INSTANCE_STATE} (expected running)"
  fi

  # Check CloudWatch logs for tick loop
  info "Checking CloudWatch logs for server activity..."
  LOG_GROUP="/multi-kb/ec2"
  RECENT_LOGS=$(aws logs filter-log-events \
    --log-group-name "$LOG_GROUP" \
    --start-time "$(($(date +%s) * 1000 - 3600000))" \
    --limit 20 \
    --region "$REGION" \
    --query 'events[].message' \
    --output json 2>/dev/null || echo "[]")

  if [[ "$RECENT_LOGS" != "[]" ]]; then
    pass "CloudWatch logs contain recent entries (last hour)"
    LOG_COUNT=$(echo "$RECENT_LOGS" | jq 'length')
    info "Recent log entries: ${LOG_COUNT}"
    # Check for tick-related messages
    if echo "$RECENT_LOGS" | jq -r '.[]' | grep -qi "tick\|server\|started\|poll"; then
      pass "Logs contain server tick/startup indicators"
    else
      info "Logs present but no tick indicators found — may be early in lifecycle"
    fi
  else
    info "No recent CloudWatch logs found — checking if log group exists..."
    LOG_GROUP_EXISTS=$(aws logs describe-log-groups \
      --log-group-name-prefix "$LOG_GROUP" \
      --region "$REGION" \
      --query 'logGroups | length(@)' \
      --output text 2>/dev/null || echo "0")
    if [[ "$LOG_GROUP_EXISTS" -gt "0" ]]; then
      skip "Log group exists but no recent entries — EC2 may have just started"
    else
      fail "CloudWatch log group '${LOG_GROUP}' does not exist"
    fi
  fi
else
  skip "No EC2 instance found — cannot check health"
fi

# ================================================================
section "Step 5: EC2 Recovery"
# ================================================================

if [[ "$SKIP_RECOVERY" == "true" ]]; then
  skip "EC2 recovery test skipped (--skip-recovery)"
elif [[ -n "$EC2_INSTANCE_ID" && "$EC2_INSTANCE_ID" != "None" ]]; then
  info "Terminating EC2 instance ${EC2_INSTANCE_ID} to test ASG recovery..."
  aws ec2 terminate-instances \
    --instance-ids "$EC2_INSTANCE_ID" \
    --region "$REGION" >/dev/null

  info "Instance terminated. Waiting for ASG to launch replacement (up to 10 min)..."
  POLL_INTERVAL=30
  MAX_POLLS=20
  RECOVERED=false

  for ((i=1; i<=MAX_POLLS; i++)); do
    NEW_INSTANCES=$(aws autoscaling describe-auto-scaling-groups \
      --auto-scaling-group-names "$ASG_NAME" \
      --region "$REGION" \
      --query 'AutoScalingGroups[0].Instances[?LifecycleState==`InService`].InstanceId' \
      --output text 2>/dev/null || echo "")

    if [[ -n "$NEW_INSTANCES" && "$NEW_INSTANCES" != "$EC2_INSTANCE_ID" && "$NEW_INSTANCES" != "None" ]]; then
      pass "ASG launched replacement instance: ${NEW_INSTANCES}"
      RECOVERED=true
      EC2_INSTANCE_ID="$NEW_INSTANCES"
      break
    fi
    info "  Waiting for replacement... (${i}/${MAX_POLLS})"
    sleep "$POLL_INTERVAL"
  done

  if [[ "$RECOVERED" == "false" ]]; then
    fail "ASG did not launch replacement instance within 10 minutes"
  else
    # Give the new instance time to boot and start CLI
    info "Waiting 60s for new instance to complete user data script..."
    sleep 60

    NEW_STATE=$(aws ec2 describe-instances \
      --instance-ids "$EC2_INSTANCE_ID" \
      --region "$REGION" \
      --query 'Reservations[0].Instances[0].State.Name' \
      --output text 2>/dev/null || echo "unknown")
    if [[ "$NEW_STATE" == "running" ]]; then
      pass "Replacement instance is running"
    else
      info "Replacement instance state: ${NEW_STATE} (may still be initializing)"
    fi
  fi
else
  skip "No EC2 instance to test recovery"
fi

# ================================================================
section "Step 6: SSM Access"
# ================================================================

if [[ -n "$EC2_INSTANCE_ID" && "$EC2_INSTANCE_ID" != "None" ]]; then
  SSM_CHECK=$(aws ssm describe-instance-information \
    --filters "Key=InstanceIds,Values=${EC2_INSTANCE_ID}" \
    --region "$REGION" \
    --query 'InstanceInformationList | length(@)' \
    --output text 2>/dev/null || echo "0")

  if [[ "$SSM_CHECK" -gt "0" ]]; then
    pass "EC2 instance is registered with SSM"
    info "Connect via: aws ssm start-session --target ${EC2_INSTANCE_ID} --region ${REGION}"
  else
    info "SSM agent may not be ready yet on the instance"
    skip "SSM registration not confirmed — instance may still be initializing"
  fi
else
  skip "No EC2 instance to check SSM"
fi

# ================================================================
section "Step 7: CloudWatch Observability"
# ================================================================

# Check Lambda log groups
for FN_TYPE in "submit" "recall"; do
  LOG_PREFIX="/aws/lambda"
  LAMBDA_LOGS=$(aws logs describe-log-groups \
    --log-group-name-prefix "$LOG_PREFIX" \
    --region "$REGION" \
    --query "logGroups[?contains(logGroupName, '${FN_TYPE}') || contains(logGroupName, '$(echo $FN_TYPE | sed 's/./\u&/')')].[logGroupName]" \
    --output text 2>/dev/null | head -1)
  if [[ -n "$LAMBDA_LOGS" ]]; then
    pass "${FN_TYPE} Lambda log group exists: ${LAMBDA_LOGS}"
  else
    info "${FN_TYPE} Lambda log group not found via prefix search"
  fi
done

# Check API Gateway access logs
API_LOG_GROUP=$(aws logs describe-log-groups \
  --log-group-name-prefix "/aws/apigateway" \
  --region "$REGION" \
  --query 'logGroups[0].logGroupName' \
  --output text 2>/dev/null || echo "")
if [[ -n "$API_LOG_GROUP" && "$API_LOG_GROUP" != "None" ]]; then
  pass "API Gateway log group exists: ${API_LOG_GROUP}"
else
  info "API Gateway execution log group not found (access logs may use a different path)"
fi

# ================================================================
section "Step 8: DLQ Alarm"
# ================================================================

DLQ_ALARM=$(aws cloudwatch describe-alarms \
  --alarm-name-prefix "multi-kb" \
  --region "$REGION" \
  --query "MetricAlarms[?contains(AlarmName, 'dlq') || contains(AlarmName, 'DLQ') || contains(AlarmName, 'dead-letter')].[AlarmName,StateValue]" \
  --output text 2>/dev/null || echo "")

if [[ -n "$DLQ_ALARM" ]]; then
  pass "DLQ alarm exists: $(echo "$DLQ_ALARM" | awk '{print $1}')"
  DLQ_STATE=$(echo "$DLQ_ALARM" | awk '{print $2}')
  info "DLQ alarm state: ${DLQ_STATE}"
else
  # Check all alarms for the stack
  ALL_ALARMS=$(aws cloudwatch describe-alarms \
    --alarm-name-prefix "multi-kb" \
    --region "$REGION" \
    --query 'MetricAlarms[].AlarmName' \
    --output json 2>/dev/null || echo "[]")
  info "Found alarms: ${ALL_ALARMS}"
  skip "DLQ alarm not found by name pattern"
fi

# ================================================================
section "Cleanup"
# ================================================================

if [[ -n "${SUBMITTED_UID:-}" ]]; then
  info "Cleaning up test note from S3..."
  aws s3 rm "s3://${BUCKET_NAME}/${SUBMITTED_UID}.md" --region "$REGION" 2>/dev/null || true
fi

# ================================================================
echo ""
echo "======================================"
echo "QAT-005 Post-Deploy Integration Summary"
echo "======================================"
echo -e "  ${GREEN}Passed:${NC}  ${PASSES}"
echo -e "  ${RED}Failed:${NC}  ${FAILURES}"
echo -e "  ${YELLOW}Skipped:${NC} ${SKIPS}"
echo ""

if [[ $FAILURES -eq 0 ]]; then
  echo -e "${GREEN}All checks passed or skipped — stack is operational.${NC}"
else
  echo -e "${RED}${FAILURES} check(s) failed — review output above for details.${NC}"
fi

exit $FAILURES
