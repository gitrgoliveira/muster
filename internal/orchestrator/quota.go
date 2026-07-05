package orchestrator

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// QuotaUsage records the token and cost consumption for an agent run.
// Known is false when quota data is unavailable (e.g. the on-disk session
// record is missing or unparseable); all numeric fields are zero in that case.
// The reader that populates this struct lands in US5 (T061).
type QuotaUsage struct {
	Known        bool
	InputTokens  int64
	OutputTokens int64
	CostUSD      float64
}

// claudeSessionDir returns the path to the per-project Claude Code session
// directory for the given worktree path and home directory.
//
// Claude Code stores its per-session JSONL transcripts at:
//
//	<home>/.claude/projects/<encoded-cwd>/
//
// The encoding algorithm (spike-verified, claude 2.1.193):
//   - Replace every "/." with "--"  (handles hidden dirs like .claude)
//   - Replace every "/" with "-"
//
// Example: /Users/alice/repos/proj/.claude/worktrees/abc-123
//
//	→ -Users-alice-repos-proj--claude-worktrees-abc-123
func claudeSessionDir(worktree, home string) string {
	encoded := strings.ReplaceAll(worktree, "/.", "--")
	encoded = strings.ReplaceAll(encoded, "/", "-")
	return filepath.Join(home, ".claude", "projects", encoded)
}

// sessionUsage is the JSON shape of the "usage" field inside an assistant
// message in Claude Code's on-disk JSONL transcript.
type sessionUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// sessionMessage holds the parts of a JSONL transcript line we care about.
type sessionMessage struct {
	Type    string `json:"type"`
	Message struct {
		Usage sessionUsage `json:"usage"`
	} `json:"message"`
}

// ReadSessionQuota reads the most-recently-modified *.jsonl file in dir,
// sums all assistant-turn token counts, and returns a QuotaUsage.
//
// Best-effort contract:
//   - Missing dir, empty dir, garbled file, or no assistant turns → Known:false.
//   - Never returns an error; failures degrade to Known:false.
//   - CostUSD is always 0.0 — Claude Code does not write costUSD into the JSONL
//     transcript for interactive (non -p) sessions (spike-verified).
func ReadSessionQuota(dir string) QuotaUsage {
	// Find the most-recently-modified .jsonl in dir.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return QuotaUsage{}
	}
	var latestPath string
	var latestMod int64 // Unix nanoseconds
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if ns := info.ModTime().UnixNano(); ns > latestMod {
			latestMod = ns
			latestPath = filepath.Join(dir, e.Name())
		}
	}
	if latestPath == "" {
		return QuotaUsage{}
	}

	return parseQuotaFromJSONL(latestPath)
}

// parseQuotaFromJSONL reads the JSONL at path, sums all assistant-turn usage
// fields, and returns a QuotaUsage. Unknown/garbled lines are skipped.
// If no valid assistant turns are found, returns Known:false.
func parseQuotaFromJSONL(path string) QuotaUsage {
	f, err := os.Open(path)
	if err != nil {
		return QuotaUsage{}
	}
	defer f.Close()

	var totalIn, totalOut int64
	found := false
	scanner := bufio.NewScanner(f)
	// Increase scan buffer for long lines (large context windows produce long JSON).
	buf := make([]byte, 4*1024*1024) // 4 MiB
	scanner.Buffer(buf, cap(buf))
	for scanner.Scan() {
		var msg sessionMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue // skip garbled lines
		}
		if msg.Type != "assistant" {
			continue
		}
		totalIn += msg.Message.Usage.InputTokens
		totalOut += msg.Message.Usage.OutputTokens
		found = true
	}

	// Degrade to unknown on any scan error (e.g. line exceeding buffer).
	// Best-effort must never report partial/wrong numbers.
	if err := scanner.Err(); err != nil {
		return QuotaUsage{}
	}
	if !found {
		return QuotaUsage{}
	}
	return QuotaUsage{
		Known:        true,
		InputTokens:  totalIn,
		OutputTokens: totalOut,
		// CostUSD intentionally 0: interactive sessions do not write cost to
		// the JSONL transcript (spike R8). Not fabricating a number.
	}
}

// ReadSessionQuotaForWorktree derives the Claude Code session directory for
// the given worktree path and home directory, then delegates to ReadSessionQuota.
//
// This is the primary entry point used by the orchestrator's finishRun path.
// homeDir should be os.UserHomeDir(); it is a parameter to allow test injection.
func ReadSessionQuotaForWorktree(worktree, homeDir string) QuotaUsage {
	dir := claudeSessionDir(worktree, homeDir)
	return ReadSessionQuota(dir)
}
