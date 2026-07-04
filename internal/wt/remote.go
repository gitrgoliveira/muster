package wt

// BranchName returns the per-bead branch name used by all wt backends.
// Convention: "muster/<beadID>", matching the M3 git backend and
// internal/worktree.branchName.
func BranchName(beadID string) string {
	return "muster/" + beadID
}

// ResolveRemote returns the effective remote name for a push operation.
// When remote is empty the default "origin" is returned; otherwise remote
// is returned unchanged. muster never creates remotes or stores credentials.
func ResolveRemote(remote string) string {
	if remote == "" {
		return "origin"
	}
	return remote
}
