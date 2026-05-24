package store

// Filter controls which issues Backend.List returns.
type Filter struct {
	Status       []string // nil/empty = no filter on status
	IDs          []string // nil/empty = no filter on ID
	Limit        int      // 0 = unlimited
	TruncateDesc int      // 0 = no truncation; N > 0 = cap Description at N bytes
}
