#!/bin/sh
# fake_tmux.sh - records argv to $FAKE_TMUX_RECORD_FILE and optionally
# returns canned output from $FAKE_TMUX_OUTPUT_FILE.
# Set FAKE_TMUX_EXIT to control exit code (default 0).

# Record the invocation.
if [ -n "$FAKE_TMUX_RECORD_FILE" ]; then
    echo "$*" >> "$FAKE_TMUX_RECORD_FILE"
fi

# Return canned output if provided.
if [ -n "$FAKE_TMUX_OUTPUT_FILE" ] && [ -f "$FAKE_TMUX_OUTPUT_FILE" ]; then
    cat "$FAKE_TMUX_OUTPUT_FILE"
fi

# Support per-subcommand output files: FAKE_TMUX_OUTPUT_<CMD>
# e.g., FAKE_TMUX_OUTPUT_list-sessions for `tmux list-sessions`
CMD="${1}"
VAR_NAME="FAKE_TMUX_OUTPUT_$(echo "$CMD" | tr '[:lower:]-' '[:upper:]_')"
eval "SUBCMD_OUT=\$$VAR_NAME"
if [ -n "$SUBCMD_OUT" ]; then
    printf '%s' "$SUBCMD_OUT"
fi

# Support per-call output via FAKE_TMUX_CALLS_DIR
# Each call creates a file with the argv for inspection
if [ -n "$FAKE_TMUX_CALLS_DIR" ]; then
    COUNT_FILE="$FAKE_TMUX_CALLS_DIR/count"
    COUNT=0
    if [ -f "$COUNT_FILE" ]; then
        COUNT=$(cat "$COUNT_FILE")
    fi
    COUNT=$((COUNT + 1))
    echo "$COUNT" > "$COUNT_FILE"
    echo "$*" > "$FAKE_TMUX_CALLS_DIR/call_$COUNT"
fi

exit "${FAKE_TMUX_EXIT:-0}"
