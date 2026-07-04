package orchestrator

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
