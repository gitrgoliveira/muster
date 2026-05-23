package core

import "testing"

func TestBeadType_Valid(t *testing.T) {
	valid := []BeadType{TypeFeature, TypeBug, TypeTask, TypeEpic, TypeChore}
	for _, v := range valid {
		if !v.Valid() {
			t.Errorf("BeadType(%q).Valid() = false, want true", v)
		}
	}
	if BeadType("unknown").Valid() {
		t.Error(`BeadType("unknown").Valid() = true, want false`)
	}
}

func TestColumn_Valid(t *testing.T) {
	valid := []Column{ColBacklog, ColScheduled, ColRunning, ColReview, ColDone}
	for _, v := range valid {
		if !v.Valid() {
			t.Errorf("Column(%q).Valid() = false, want true", v)
		}
	}
	if Column("unknown").Valid() {
		t.Error(`Column("unknown").Valid() = true, want false`)
	}
}

func TestPriority_Valid(t *testing.T) {
	for p := Priority(0); p <= 4; p++ {
		if !p.Valid() {
			t.Errorf("Priority(%d).Valid() = false, want true", p)
		}
	}
	if Priority(-1).Valid() {
		t.Error("Priority(-1).Valid() = true, want false")
	}
	if Priority(5).Valid() {
		t.Error("Priority(5).Valid() = true, want false")
	}
}

func TestStepStatus_Valid(t *testing.T) {
	valid := []StepStatus{StepPending, StepActive, StepDone, StepFailed}
	for _, v := range valid {
		if !v.Valid() {
			t.Errorf("StepStatus(%q).Valid() = false, want true", v)
		}
	}
	if StepStatus("unknown").Valid() {
		t.Error(`StepStatus("unknown").Valid() = true, want false`)
	}
}

func TestMode_Valid(t *testing.T) {
	valid := []Mode{ModePlan, ModeBuild, ModeReview, ModeAgent, ModeApply, ModeYolo}
	for _, v := range valid {
		if !v.Valid() {
			t.Errorf("Mode(%q).Valid() = false, want true", v)
		}
	}
	if Mode("unknown").Valid() {
		t.Error(`Mode("unknown").Valid() = true, want false`)
	}
}

func TestAgentID_Valid(t *testing.T) {
	valid := []AgentID{AgentClaude, AgentGemini, AgentOpenCode, AgentCodex}
	for _, v := range valid {
		if !v.Valid() {
			t.Errorf("AgentID(%q).Valid() = false, want true", v)
		}
	}
	if AgentID("unknown").Valid() {
		t.Error(`AgentID("unknown").Valid() = true, want false`)
	}
}

func TestVCS_Valid(t *testing.T) {
	valid := []VCS{VCSGit, VCSJJ}
	for _, v := range valid {
		if !v.Valid() {
			t.Errorf("VCS(%q).Valid() = false, want true", v)
		}
	}
	if VCS("unknown").Valid() {
		t.Error(`VCS("unknown").Valid() = true, want false`)
	}
}

func TestVCS_EmptyStringIsValid(t *testing.T) {
	if !VCS("").Valid() {
		t.Error(`VCS("").Valid() = false, want true`)
	}
}

func TestNowPlayingKind_Valid(t *testing.T) {
	valid := []NowPlayingKind{NPKTool, NPKThought, NPKOutput}
	for _, v := range valid {
		if !v.Valid() {
			t.Errorf("NowPlayingKind(%q).Valid() = false, want true", v)
		}
	}
	if NowPlayingKind("unknown").Valid() {
		t.Error(`NowPlayingKind("unknown").Valid() = true, want false`)
	}
}

func TestLogKind_Valid(t *testing.T) {
	valid := []LogKind{LogSystem, LogTool, LogThought, LogOutput}
	for _, v := range valid {
		if !v.Valid() {
			t.Errorf("LogKind(%q).Valid() = false, want true", v)
		}
	}
	if LogKind("unknown").Valid() {
		t.Error(`LogKind("unknown").Valid() = true, want false`)
	}
}

func TestEventKind_Valid(t *testing.T) {
	valid := []EventKind{
		EvOpened, EvScheduled, EvClaimed, EvStarted, EvPaused, EvSplit,
		EvReview, EvComment, EvApproved, EvClosed, EvReopened, EvRequeued,
		EvBlocked, EvUnblocked, EvFailed, EvDiscovered,
	}
	for _, v := range valid {
		if !v.Valid() {
			t.Errorf("EventKind(%q).Valid() = false, want true", v)
		}
	}
	if EventKind("unknown").Valid() {
		t.Error(`EventKind("unknown").Valid() = true, want false`)
	}
}

func TestFileStatus_Valid(t *testing.T) {
	valid := []FileStatus{FileAdded, FileModified, FileDeleted}
	for _, v := range valid {
		if !v.Valid() {
			t.Errorf("FileStatus(%q).Valid() = false, want true", v)
		}
	}
	if FileStatus("unknown").Valid() {
		t.Error(`FileStatus("unknown").Valid() = true, want false`)
	}
}
