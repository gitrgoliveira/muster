package bdshell

// SPIKE T002 (verified against real bd 1.1.0, 2026-07-07) — memories contract.
// Implemented in US5 (T053); this block pins the shapes the wrapper builds to.
//
//	bd remember "<text>"          -> derives a stable kebab-slug key from the text.
//	                                 stdout: `Remembered [<key>]: <text>` (create).
//	                                 So a KEYLESS create returns a retrievable key —
//	                                 the handler need NOT generate one (closes F8).
//	bd remember --key <K> "<v>"   -> `Remembered [<K>]: <v>` (create) or
//	                                 `Updated [<K>]: <v>` (upsert on an existing key).
//	bd recall <key>               -> the value.
//	bd forget <key>               -> `Forgot [<key>]: <v>` (ok) or
//	                                 `No memory with key "<key>"` (not found).
//	bd memories [term] --json     -> a JSON OBJECT {"<key>": "<value>", ...} (a MAP,
//	                                 not an array); [term] filters.
//
// Scoping: bd resolves the memory store from BEADS_DIR (the bdshell CLI already
// sets it), so muster's memories are scoped to its --beads-dir DB. Arg safety
// (T055): pass key/value AFTER a `--` separator so a leading '-' is not read as a
// flag. The DELETE-not-found case must be surfaced as a typed not-found, not a
// false success.
//
// Additional verified behaviors (2026-07-07):
//   - `bd remember [--key K] --json -- <v>` returns {"action","key",...} — the
//     derived key is in the JSON, so a keyless create still yields a key.
//   - `bd memories [q] --json` returns a {key:value} object that ALSO contains a
//     "schema_version": <int> meta entry — it MUST be filtered out.
//   - `bd forget <missing>` exits 0 and prints `No memory with key "<k>"`, so
//     not-found is detected from stdout, not the exit code.

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrMemoryNotFound is returned by Forget when the key does not exist (bd exits
// 0 in that case, so it is detected from output).
var ErrMemoryNotFound = errors.New("memory not found")

// memoriesMetaKey is the non-memory meta entry bd includes in `memories --json`.
const memoriesMetaKey = "schema_version"

// rememberResult models the JSON of `bd remember --json`.
type rememberResult struct {
	Action string `json:"action"`
	Key    string `json:"key"`
}

// Remember upserts a memory. An empty key lets bd derive one from the value;
// the (derived or given) key is returned.
func (c *CLI) Remember(ctx context.Context, key, value string) (string, error) {
	args := []string{"remember", "--json"}
	if key != "" {
		args = append(args, "--key", key)
	}
	args = append(args, "--", value)
	var res rememberResult
	if err := c.RunJSON(ctx, &res, args...); err != nil {
		return "", err
	}
	if res.Key == "" {
		res.Key = key
	}
	// A memory with no key is unusable — subsequent recall/delete would be
	// ambiguous. If bd omitted the derived key and the caller gave none, fail
	// loudly rather than surfacing an empty-keyed memory.
	if res.Key == "" {
		return "", fmt.Errorf("bd remember returned no key for the memory")
	}
	return res.Key, nil
}

// Recall returns a single memory's value by key.
func (c *CLI) Recall(ctx context.Context, key string) (string, error) {
	res, err := c.Run(ctx, "recall", "--", key)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Stdout), nil
}

// Forget deletes a memory. A missing key yields ErrMemoryNotFound. bd signals
// not-found via a "No memory with key" message; depending on version this comes
// with a zero OR non-zero exit, so we check both the success output and a
// CLIError's stderr.
func (c *CLI) Forget(ctx context.Context, key string) error {
	res, err := c.Run(ctx, "forget", "--", key)
	if err != nil {
		var ce *CLIError
		if errors.As(err, &ce) && strings.Contains(ce.Stderr, "No memory with key") {
			return ErrMemoryNotFound
		}
		return err
	}
	// A successful delete prints `Forgot [<key>]: <value>`. Anchor on that prefix
	// so a memory whose *value* contains the not-found sentinel can't be
	// misreported as not-found.
	if strings.HasPrefix(strings.TrimSpace(res.Stdout), "Forgot") {
		return nil
	}
	if strings.Contains(res.Stdout, "No memory with key") || strings.Contains(res.Stderr, "No memory with key") {
		return ErrMemoryNotFound
	}
	return nil
}

// Memories lists (optionally filtered by query) all memories as key→value,
// filtering out bd's schema_version meta entry.
func (c *CLI) Memories(ctx context.Context, query string) (map[string]string, error) {
	args := []string{"memories", "--json"}
	if query != "" {
		args = append(args, "--", query)
	}
	var raw map[string]any
	if err := c.RunJSON(ctx, &raw, args...); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if k == memoriesMetaKey {
			continue
		}
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out, nil
}
