package core

type BeadType string

const (
	TypeFeature BeadType = "feature"
	TypeBug     BeadType = "bug"
	TypeTask    BeadType = "task"
	TypeEpic    BeadType = "epic"
	TypeChore   BeadType = "chore"
)

func (t BeadType) Valid() bool {
	switch t {
	case TypeFeature, TypeBug, TypeTask, TypeEpic, TypeChore:
		return true
	}
	return false
}

type Column string

const (
	ColBacklog   Column = "backlog"
	ColScheduled Column = "scheduled"
	ColRunning   Column = "running"
	ColReview    Column = "review"
	ColDone      Column = "done"
)

func (c Column) Valid() bool {
	switch c {
	case ColBacklog, ColScheduled, ColRunning, ColReview, ColDone:
		return true
	}
	return false
}

type StepStatus string

const (
	StepPending StepStatus = "pending"
	StepActive  StepStatus = "active"
	StepDone    StepStatus = "done"
	StepFailed  StepStatus = "failed"
)

func (s StepStatus) Valid() bool {
	switch s {
	case StepPending, StepActive, StepDone, StepFailed:
		return true
	}
	return false
}

type AgentID string

const (
	AgentClaude   AgentID = "claude"
	AgentGemini   AgentID = "gemini"
	AgentOpenCode AgentID = "opencode"
	AgentCodex    AgentID = "codex"
)

func (a AgentID) Valid() bool {
	switch a {
	case AgentClaude, AgentGemini, AgentOpenCode, AgentCodex:
		return true
	}
	return false
}

type Mode string

const (
	ModePlan   Mode = "plan"
	ModeBuild  Mode = "build"
	ModeReview Mode = "review"
	ModeAgent  Mode = "agent"
	ModeApply  Mode = "apply"
	ModeYolo   Mode = "yolo"
)

func (m Mode) Valid() bool {
	switch m {
	case ModePlan, ModeBuild, ModeReview, ModeAgent, ModeApply, ModeYolo:
		return true
	}
	return false
}

type VCS string

const (
	VCSGit VCS = "git"
	VCSJJ  VCS = "jj"
)

func (v VCS) Valid() bool {
	return v == "" || v == VCSGit || v == VCSJJ
}

type Priority int // 0..4, 0 = critical, 4 = icebox

func (p Priority) Valid() bool { return p >= 0 && p <= 4 }

type Estimate string

const (
	EstXS Estimate = "XS"
	EstS  Estimate = "S"
	EstM  Estimate = "M"
	EstL  Estimate = "L"
)

type EventKind string

const (
	EvOpened     EventKind = "opened"
	EvScheduled  EventKind = "scheduled"
	EvClaimed    EventKind = "claimed"
	EvStarted    EventKind = "started"
	EvPaused     EventKind = "paused"
	EvSplit      EventKind = "split"
	EvReview     EventKind = "review"
	EvComment    EventKind = "comment"
	EvApproved   EventKind = "approved"
	EvClosed     EventKind = "closed"
	EvReopened   EventKind = "reopened"
	EvRequeued   EventKind = "requeued"
	EvBlocked    EventKind = "blocked"
	EvUnblocked  EventKind = "unblocked"
	EvFailed     EventKind = "failed"
	EvDiscovered EventKind = "discovered"
)

func (k EventKind) Valid() bool {
	switch k {
	case EvOpened, EvScheduled, EvClaimed, EvStarted, EvPaused, EvSplit,
		EvReview, EvComment, EvApproved, EvClosed, EvReopened, EvRequeued,
		EvBlocked, EvUnblocked, EvFailed, EvDiscovered:
		return true
	}
	return false
}

type NowPlayingKind string

const (
	NPKTool    NowPlayingKind = "tool"
	NPKThought NowPlayingKind = "thought"
	NPKOutput  NowPlayingKind = "output"
)

func (k NowPlayingKind) Valid() bool {
	switch k {
	case NPKTool, NPKThought, NPKOutput:
		return true
	}
	return false
}

type LogKind string

const (
	LogSystem  LogKind = "system"
	LogTool    LogKind = "tool"
	LogThought LogKind = "thought"
	LogOutput  LogKind = "output"
)

func (k LogKind) Valid() bool {
	switch k {
	case LogSystem, LogTool, LogThought, LogOutput:
		return true
	}
	return false
}

type FileStatus string

const (
	FileAdded    FileStatus = "A"
	FileModified FileStatus = "M"
	FileDeleted  FileStatus = "D"
)

func (s FileStatus) Valid() bool {
	switch s {
	case FileAdded, FileModified, FileDeleted:
		return true
	}
	return false
}

// PermissionMode controls the autonomy level passed to the claude CLI via
// --permission-mode. Distinct from Mode (plan/agent/…): Mode is the
// invocation profile; PermissionMode is the autonomy level.
// Spike-verified (claude 2.1.145): these are the exact allowed values.
type PermissionMode string

const (
	PermDefault           PermissionMode = "default"
	PermAcceptEdits       PermissionMode = "acceptEdits"
	PermDontAsk           PermissionMode = "dontAsk"
	PermBypassPermissions PermissionMode = "bypassPermissions"
	PermPlan              PermissionMode = "plan"
	PermAuto              PermissionMode = "auto"
)

// Valid returns true if the permission mode is in the allow-list.
func (p PermissionMode) Valid() bool {
	switch p {
	case PermDefault, PermAcceptEdits, PermDontAsk, PermBypassPermissions, PermPlan, PermAuto:
		return true
	}
	return false
}
