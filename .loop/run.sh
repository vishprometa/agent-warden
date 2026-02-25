#!/bin/bash
# =============================================================================
# AgentWarden Self-Improvement Loop
# =============================================================================
# Usage:
#   ./run.sh                    # 10 iterations, 3 hour limit
#   ./run.sh 20                 # 20 iterations, 3 hour limit
#   ./run.sh 20 5               # 20 iterations, 5 hour limit
#   ./run.sh --resume           # Resume from where it left off (reads state.md)
#   ./run.sh --dry-run          # Show what would run without executing
#
# What it does:
#   Each iteration invokes `claude -p` with a structured prompt that:
#   1. Reads the current state file (.loop/state.md)
#   2. Picks the next task
#   3. Implements it, runs go build + go test
#   4. Updates the state file
#   5. Exits — then the loop starts the next iteration
#
# Logs: .loop/logs/iteration-{N}-{timestamp}.md
# State: .loop/state.md (persists across iterations, tracks all progress)
# =============================================================================

set -euo pipefail

# --- Config ---
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
STATE_FILE="$SCRIPT_DIR/state.md"
SYSTEM_PROMPT_FILE="$SCRIPT_DIR/system-prompt.md"
LOG_DIR="$SCRIPT_DIR/logs"

MAX_ITERATIONS="${1:-10}"
MAX_HOURS="${2:-3}"
DRY_RUN=false

# Parse flags
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    --resume) ;; # Just continue from state.md (default behavior)
  esac
done

# Strip flags from positional args
if [[ "$MAX_ITERATIONS" == --* ]]; then MAX_ITERATIONS=10; fi
if [[ "$MAX_HOURS" == --* ]]; then MAX_HOURS=3; fi

START_TIME=$(date +%s)
MAX_SECONDS=$((MAX_HOURS * 3600))

mkdir -p "$LOG_DIR"

# --- Load env vars from .env if present ---
ENV_FILE="$PROJECT_DIR/.env"
if [ -f "$ENV_FILE" ]; then
  set -a
  source "$ENV_FILE"
  set +a
fi

# --- Helpers ---
elapsed_seconds() {
  echo $(( $(date +%s) - START_TIME ))
}

elapsed_human() {
  local secs=$(elapsed_seconds)
  printf "%dh %dm %ds" $((secs/3600)) $((secs%3600/60)) $((secs%60))
}

time_remaining() {
  local remaining=$(( MAX_SECONDS - $(elapsed_seconds) ))
  if [ "$remaining" -le 0 ]; then echo "0"; else echo "$remaining"; fi
}

# Get current iteration from state file
current_iteration() {
  grep -oE 'Iteration: [0-9]+' "$STATE_FILE" 2>/dev/null | grep -oE '[0-9]+' || echo "0"
}

# Count remaining tasks
remaining_tasks() {
  grep -c '^\- \[ \]' "$STATE_FILE" 2>/dev/null || echo "0"
}

# --- Pre-flight checks ---
if [ ! -f "$STATE_FILE" ]; then
  echo "ERROR: State file not found at $STATE_FILE"
  echo "Run this from the .loop/ directory or ensure state.md exists."
  exit 1
fi

if [ ! -f "$SYSTEM_PROMPT_FILE" ]; then
  echo "ERROR: System prompt not found at $SYSTEM_PROMPT_FILE"
  exit 1
fi

if ! command -v claude &>/dev/null; then
  echo "ERROR: 'claude' CLI not found. Install Claude Code first."
  exit 1
fi

echo "=============================================="
echo "  AgentWarden Self-Improvement Loop"
echo "=============================================="
echo "  Project:        $PROJECT_DIR"
echo "  Max iterations: $MAX_ITERATIONS"
echo "  Max hours:      $MAX_HOURS"
echo "  Remaining tasks: $(remaining_tasks)"
echo "  Starting from:  iteration $(current_iteration)"
echo "  Dry run:        $DRY_RUN"
echo "=============================================="
echo ""

if [ "$DRY_RUN" = true ]; then
  echo "[DRY RUN] Would execute $MAX_ITERATIONS iterations."
  echo "[DRY RUN] System prompt: $SYSTEM_PROMPT_FILE"
  echo "[DRY RUN] State file: $STATE_FILE"
  echo ""
  echo "--- State file contents ---"
  cat "$STATE_FILE"
  exit 0
fi

# --- Main Loop ---
for i in $(seq 1 "$MAX_ITERATIONS"); do
  # Check time limit
  if [ "$(time_remaining)" -le 0 ]; then
    echo ""
    echo ">>> TIME LIMIT REACHED ($(elapsed_human)). Stopping."
    break
  fi

  # Check if all tasks are done
  if [ "$(remaining_tasks)" -eq 0 ]; then
    echo ""
    echo ">>> ALL TASKS COMPLETED! Stopping."
    break
  fi

  ITER_NUM=$(($(current_iteration) + 1))
  TIMESTAMP=$(date +%Y%m%d-%H%M%S)
  LOG_FILE="$LOG_DIR/iteration-${ITER_NUM}-${TIMESTAMP}.md"

  echo "----------------------------------------------"
  echo "  Iteration $i/$MAX_ITERATIONS (state iter: $ITER_NUM)"
  echo "  Time elapsed: $(elapsed_human)"
  echo "  Tasks remaining: $(remaining_tasks)"
  echo "  Log: $LOG_FILE"
  echo "----------------------------------------------"

  # Build the iteration prompt
  STATE_CONTENT=$(cat "$STATE_FILE")

  PROMPT=$(cat <<PROMPT_EOF
## Current State

$STATE_CONTENT

## Your Task for This Iteration

Read the state above. Pick the next unchecked task ([ ]) from the highest priority group.
Implement it, run \`go build ./...\` and \`go test ./...\`, and update .loop/state.md.

Remember:
- ONE task per iteration
- Always run go build ./... AND go test ./...
- Update state.md: mark [x], add to Completed section, log test results
- Increment the Iteration counter in state.md
- If blocked, add a note and pick the next task instead
- If fixing CI, push the changes and check workflow status with \`gh run list\`
PROMPT_EOF
)

  # Run Claude in print mode
  (
    unset CLAUDECODE  # Allow nested invocation
    cd "$PROJECT_DIR"
    claude -p \
      --append-system-prompt-file "$SYSTEM_PROMPT_FILE" \
      --max-turns 30 \
      --dangerously-skip-permissions \
      "$PROMPT" 2>&1
  ) | tee "$LOG_FILE"

  CLAUDE_EXIT=$?

  echo ""
  echo "  >> Iteration $ITER_NUM finished (exit: $CLAUDE_EXIT)"
  echo "  >> Log saved: $LOG_FILE"

  if [ "$CLAUDE_EXIT" -ne 0 ]; then
    echo "  >> WARNING: Claude exited with code $CLAUDE_EXIT"
    echo "  >> Check $LOG_FILE for details"
  fi

  # Brief pause between iterations
  sleep 3
done

# --- Summary ---
echo ""
echo "=============================================="
echo "  Loop Complete"
echo "=============================================="
echo "  Total time: $(elapsed_human)"
echo "  Iterations run: $i"
echo "  Tasks remaining: $(remaining_tasks)"
echo "  Logs: $LOG_DIR/"
echo "=============================================="
echo ""
echo "Review the state file:"
echo "  cat $STATE_FILE"
echo ""
echo "Review all logs:"
echo "  ls -la $LOG_DIR/"
