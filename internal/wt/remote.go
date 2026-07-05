package wt

import (
	"errors"
	"fmt"
	"regexp"
)

// ErrInvalidRemote is returned by ResolveRemote when the remote name fails
// validation. A leading '-' is always invalid (it would be interpreted as a
// git option). The valid character set is [A-Za-z0-9][A-Za-z0-9._-]*.
var ErrInvalidRemote = errors.New("invalid remote name")

// remoteNameRe matches a valid git remote name: starts with an alphanumeric
// character, followed by zero or more alphanumeric / dot / dash / underscore
// characters. This rejects any name beginning with '-', which git would
// interpret as an option (argument injection prevention).
var remoteNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// BranchName returns the per-bead branch name used by all wt backends.
// Convention: "muster/<beadID>", matching the M3 git backend and
// internal/worktree.branchName.
func BranchName(beadID string) string {
	return "muster/" + beadID
}

// ResolveRemote returns the effective remote name for a push operation and
// validates it against a strict allow-list pattern.
//
// When remote is empty the default "origin" is returned with no error. When
// remote is non-empty it must match ^[A-Za-z0-9][A-Za-z0-9._-]*$ — this
// rejects names beginning with '-' (which git would treat as options), names
// containing spaces, and non-ASCII characters. Any violation returns
// ("", ErrInvalidRemote). muster never creates remotes or stores credentials.
func ResolveRemote(remote string) (string, error) {
	if remote == "" {
		return "origin", nil
	}
	if !remoteNameRe.MatchString(remote) {
		return "", fmt.Errorf("%w: %q", ErrInvalidRemote, remote)
	}
	return remote, nil
}
