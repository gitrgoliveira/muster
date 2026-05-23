package core

import "errors"

var (
	ErrTitleRequired   = errors.New("title is required")
	ErrTitleTooLong    = errors.New("title exceeds 255 chars")
	ErrInvalidType     = errors.New("invalid type")
	ErrInvalidColumn   = errors.New("invalid column")
	ErrInvalidPriority = errors.New("invalid priority")
	ErrInvalidVCS      = errors.New("invalid vcs")
	ErrInvalidAgent    = errors.New("invalid agent")
	ErrInvalidMode     = errors.New("invalid mode")
	ErrInvalidID       = errors.New("invalid bead id")
)

// DeriveEstimate maps a token budget to an XS/S/M/L size.
func DeriveEstimate(tokensBudget int) Estimate {
	switch {
	case tokensBudget >= 350_000:
		return EstL
	case tokensBudget >= 180_000:
		return EstM
	case tokensBudget >= 90_000:
		return EstS
	default:
		return EstXS
	}
}

// DeriveAssignee returns the active step's agent, falling back to the last
// step whose status is StepDone, falling back to "".
func DeriveAssignee(steps []Step) AgentID {
	for _, s := range steps {
		if s.Status == StepActive {
			return s.Agent
		}
	}
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Status == StepDone {
			return steps[i].Agent
		}
	}
	return ""
}

// DeriveCommentCount counts comment events in history plus reviewer comments.
func DeriveCommentCount(history []HistoryEvent, reviewer *Reviewer) int {
	n := 0
	for _, h := range history {
		if h.Kind == EvComment {
			n++
		}
	}
	if reviewer != nil {
		n += reviewer.Comments
	}
	return n
}
