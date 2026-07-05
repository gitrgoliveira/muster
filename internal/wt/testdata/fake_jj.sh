#!/bin/sh
# fake_jj.sh — canned responses for wt jj backend tests.
# Does NOT make real jj calls.
#
# Environment variables:
#   FAKE_JJ_EXIT          - exit code (default 0)
#   FAKE_JJ_STDOUT        - if set, printed verbatim instead of defaults
#   FAKE_JJ_RECORD_FILE   - if set, append argv to this file (tab-delimited)
#
# Supported argv patterns (matched by $1 [$2 [...]]):
#   --version                    → "jj 0.42.0"
#   root                         → prints FAKE_JJ_ROOT or "." (exit FAKE_JJ_EXIT)
#   workspace add <path>         → "Created workspace in <path>"
#   diff --summary               → lines like "M file.txt\nA new.go\nD old.txt"
#   diff --git [path]            → minimal git-format diff

# Record invocation, tab-delimited.
tab=$(printf '\t')
if [ -n "$FAKE_JJ_RECORD_FILE" ]; then
    ( IFS="$tab"; printf '%s\n' "$*" ) >> "$FAKE_JJ_RECORD_FILE"
fi

# Custom output override.
if [ -n "$FAKE_JJ_STDOUT" ]; then
    printf '%s' "$FAKE_JJ_STDOUT"
    exit "${FAKE_JJ_EXIT:-0}"
fi

EXIT="${FAKE_JJ_EXIT:-0}"

case "$1" in
    --version)
        echo "jj 0.42.0"
        ;;
    root)
        if [ "$EXIT" -ne 0 ]; then
            echo 'Error: There is no jj repo in "."' >&2
            exit "$EXIT"
        fi
        echo "${FAKE_JJ_ROOT:-.}"
        ;;
    workspace)
        case "$2" in
            add)
                echo "Created workspace in \"$3\""
                ;;
            forget)
                echo "Forgot workspace $3"
                ;;
            *)
                echo "workspace: unknown subcommand $2" >&2
                exit 1
                ;;
        esac
        ;;
    bookmark)
        case "$2" in
            set)
                echo "bookmark set: $3"
                ;;
            create)
                echo "bookmark create: $3"
                ;;
            *)
                echo "bookmark: unknown subcommand $2" >&2
                exit 1
                ;;
        esac
        ;;
    git)
        case "$2" in
            export)
                echo "Exporting bookmarks to git"
                ;;
            remote)
                echo "git remote $3 $4 $5"
                ;;
            *)
                echo "git: unknown subcommand $2" >&2
                exit 1
                ;;
        esac
        ;;
    describe)
        echo "Working copy now at: (described)"
        ;;
    new)
        echo "Working copy now at: (empty)"
        ;;
    diff)
        case "$2" in
            --summary)
                if [ -n "$FAKE_JJ_DIFF_SUMMARY" ]; then
                    printf '%s' "$FAKE_JJ_DIFF_SUMMARY"
                else
                    printf 'M modified.txt\nA new.go\nD deleted.txt\n'
                fi
                ;;
            --git)
                if [ -n "$FAKE_JJ_DIFF_GIT" ]; then
                    printf '%s' "$FAKE_JJ_DIFF_GIT"
                else
                    printf 'diff --git a/file.txt b/file.txt\nindex 0000000..1111111 100644\n--- a/file.txt\n+++ b/file.txt\n@@ -1,1 +1,1 @@\n-old\n+new\n'
                fi
                ;;
            *)
                echo "diff: unknown flag $2" >&2
                exit 1
                ;;
        esac
        ;;
    status)
        if [ -n "$FAKE_JJ_STATUS" ]; then
            printf '%s' "$FAKE_JJ_STATUS"
        else
            printf 'Working copy changes:\nM modified.txt\n'
        fi
        ;;
    *)
        echo "fake_jj: unknown subcommand $1" >&2
        exit 1
        ;;
esac

exit "$EXIT"
