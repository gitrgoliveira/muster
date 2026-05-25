package store

import "strings"

// Filter controls which issues Backend.List returns.
type Filter struct {
	Status       []string // nil/empty = no filter on status
	IDs          []string // nil/empty = no filter on ID
	Limit        int      // 0 = unlimited
	TruncateDesc int      // 0 = no truncation; N > 0 = cap Description at N bytes
}

// MatchesFilter reports whether iss passes all criteria in f.
func MatchesFilter(iss Issue, f Filter) bool {
	if len(f.Status) > 0 {
		found := false
		for _, s := range f.Status {
			if strings.EqualFold(iss.Status, s) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(f.IDs) > 0 {
		found := false
		for _, id := range f.IDs {
			if iss.ID == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
