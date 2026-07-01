// Package shellquote provides POSIX shell single-quoting for building sh -c
// command strings safely.
package shellquote

import "strings"

// Single wraps s in single quotes, escaping each embedded single quote via
// the standard POSIX idiom (close-quote, backslash-escaped single quote,
// reopen-quote — see the raw-string literal below for the exact bytes).
func Single(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
