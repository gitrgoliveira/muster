#!/bin/sh
# fake_claude.sh — canned responses for claude adapter tests.
# Does NOT make real API calls.
#
# Environment variables:
#   FAKE_CLAUDE_AUTH_LOGGED_IN  - "true"/"false" for `auth status --json` (default "true")
#   FAKE_CLAUDE_VERSION         - version string (default "2.1.145 (Claude Code)")
#   FAKE_CLAUDE_EXIT            - exit code (default 0)
#   FAKE_CLAUDE_STDOUT          - if set, printed verbatim instead of defaults
#   FAKE_CLAUDE_RECORD_FILE     - if set, append argv to this file

# Record invocation, tab-delimited so argv boundaries survive (matching
# fake_tmux.sh). "$*" space-joins, which would collapse boundaries and mangle an
# arg that contains spaces (e.g. the `sh -c '...'` wrapper), hiding
# quoting/escaping regressions.
tab=$(printf '\t')
if [ -n "$FAKE_CLAUDE_RECORD_FILE" ]; then
    ( IFS="$tab"; printf '%s\n' "$*" ) >> "$FAKE_CLAUDE_RECORD_FILE"
fi

# Custom output override.
if [ -n "$FAKE_CLAUDE_STDOUT" ]; then
    printf '%s\n' "$FAKE_CLAUDE_STDOUT"
    exit "${FAKE_CLAUDE_EXIT:-0}"
fi

# Dispatch on subcommand.
case "$1" in
    --version)
        echo "${FAKE_CLAUDE_VERSION:-2.1.145 (Claude Code)}"
        ;;
    auth)
        case "$2" in
            status)
                LOGGED_IN="${FAKE_CLAUDE_AUTH_LOGGED_IN:-true}"
                printf '{"loggedIn":%s,"authMethod":"claude.ai","apiProvider":"firstParty"}\n' "$LOGGED_IN"
                ;;
            *)
                echo "auth: unknown subcommand $2" >&2
                exit 1
                ;;
        esac
        ;;
    *)
        # In interactive/pane mode, simulate brief output then exit.
        echo "FAKE_CLAUDE_RUN: $*"
        ;;
esac

exit "${FAKE_CLAUDE_EXIT:-0}"
