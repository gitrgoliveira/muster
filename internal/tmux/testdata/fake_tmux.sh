#!/bin/sh
# fake_tmux.sh - records argv to $FAKE_TMUX_RECORD_FILE and optionally
# returns canned output from $FAKE_TMUX_OUTPUT_FILE.
# Set FAKE_TMUX_EXIT to control exit code (default 0).

# Delimiter used to record argv. Using a TAB (not a space) preserves argument
# boundaries: "$*" space-joins, which collapses the boundary between adjacent
# args and mangles args that themselves contain spaces (e.g. the `sh -c
# '<command with spaces>'` wrapper), hiding quoting/escaping regressions. Tab-
# joining keeps one line per invocation while making the argv recoverable.
tab=$(printf '\t')

# Record the invocation (one tab-delimited line per call).
if [ -n "$FAKE_TMUX_RECORD_FILE" ]; then
    ( IFS="$tab"; printf '%s\n' "$*" ) >> "$FAKE_TMUX_RECORD_FILE"
fi

# Return canned output if provided.
if [ -n "$FAKE_TMUX_OUTPUT_FILE" ] && [ -f "$FAKE_TMUX_OUTPUT_FILE" ]; then
    cat "$FAKE_TMUX_OUTPUT_FILE"
fi

# Support per-subcommand output files: FAKE_TMUX_OUTPUT_<CMD>
# e.g., FAKE_TMUX_OUTPUT_list-sessions for `tmux list-sessions`
CMD="${1}"
VAR_NAME="FAKE_TMUX_OUTPUT_$(echo "$CMD" | tr '[:lower:]-' '[:upper:]_')"
# printenv reads the var by (computed) name without eval — avoids the
# eval-based indirection, which would be a command-injection footgun if $1
# ever carried unexpected content.
SUBCMD_OUT="$(printenv "$VAR_NAME" 2>/dev/null || true)"
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
    # Same tab-delimited recording as the main record file (preserves argv
    # boundaries rather than space-collapsing via "$*").
    ( IFS="$tab"; printf '%s\n' "$*" ) > "$FAKE_TMUX_CALLS_DIR/call_$COUNT"
fi

exit "${FAKE_TMUX_EXIT:-0}"
