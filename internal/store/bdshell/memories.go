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
