package core

import "testing"

func TestDeriveEstimate(t *testing.T) {
	cases := []struct {
		budget int
		want   Estimate
	}{
		{0, EstXS},
		{89999, EstXS},
		{90000, EstS},
		{179999, EstS},
		{180000, EstM},
		{349999, EstM},
		{350000, EstL},
	}
	for _, tc := range cases {
		got := DeriveEstimate(tc.budget)
		if got != tc.want {
			t.Errorf("DeriveEstimate(%d) = %q, want %q", tc.budget, got, tc.want)
		}
	}
}

func TestDeriveAssignee(t *testing.T) {
	t.Run("active step's agent wins", func(t *testing.T) {
		steps := []Step{
			{Agent: AgentClaude, Status: StepDone},
			{Agent: AgentGemini, Status: StepActive},
			{Agent: AgentCodex, Status: StepPending},
		}
		got := DeriveAssignee(steps)
		if got != AgentGemini {
			t.Errorf("got %q, want %q", got, AgentGemini)
		}
	})

	t.Run("last done step as fallback", func(t *testing.T) {
		steps := []Step{
			{Agent: AgentClaude, Status: StepDone},
			{Agent: AgentGemini, Status: StepDone},
			{Agent: AgentCodex, Status: StepPending},
		}
		got := DeriveAssignee(steps)
		if got != AgentGemini {
			t.Errorf("got %q, want %q (last done step)", got, AgentGemini)
		}
	})

	t.Run("empty string if no steps", func(t *testing.T) {
		got := DeriveAssignee(nil)
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})

	t.Run("empty string if no active or done steps", func(t *testing.T) {
		steps := []Step{
			{Agent: AgentClaude, Status: StepPending},
			{Agent: AgentGemini, Status: StepFailed},
		}
		got := DeriveAssignee(steps)
		if got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})
}

func TestDeriveCommentCount(t *testing.T) {
	t.Run("count history events with kind==comment", func(t *testing.T) {
		history := []HistoryEvent{
			{Kind: EvOpened},
			{Kind: EvComment},
			{Kind: EvStarted},
			{Kind: EvComment},
		}
		got := DeriveCommentCount(history, nil)
		if got != 2 {
			t.Errorf("got %d, want 2", got)
		}
	})

	t.Run("add reviewer.Comments if reviewer non-nil", func(t *testing.T) {
		history := []HistoryEvent{
			{Kind: EvComment},
		}
		reviewer := &Reviewer{Agent: AgentClaude, Comments: 3}
		got := DeriveCommentCount(history, reviewer)
		if got != 4 {
			t.Errorf("got %d, want 4", got)
		}
	})

	t.Run("nil reviewer doesn't panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("DeriveCommentCount panicked with nil reviewer: %v", r)
			}
		}()
		history := []HistoryEvent{
			{Kind: EvComment},
		}
		got := DeriveCommentCount(history, nil)
		if got != 1 {
			t.Errorf("got %d, want 1", got)
		}
	})

	t.Run("empty history with nil reviewer", func(t *testing.T) {
		got := DeriveCommentCount(nil, nil)
		if got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})
}
