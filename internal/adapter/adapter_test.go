package adapter_test

import (
	"context"
	"testing"

	"github.com/gitrgoliveira/muster/internal/adapter"
	"github.com/gitrgoliveira/muster/internal/core"
)

// stubAdapter is a minimal Adapter implementation for registry tests.
type stubAdapter struct {
	id core.AgentID
}

func (s *stubAdapter) ID() core.AgentID { return s.id }
func (s *stubAdapter) Detect(_ context.Context) (adapter.DetectResult, error) {
	return adapter.DetectResult{Installed: true, LoggedIn: true}, nil
}
func (s *stubAdapter) Modes() []adapter.Mode { return nil }
func (s *stubAdapter) Invoke(_ context.Context, _ adapter.InvokeReq) (adapter.Spec, error) {
	return adapter.Spec{}, nil
}
func (s *stubAdapter) Login(_ context.Context) (adapter.LoginFlow, error) {
	return adapter.LoginFlow{}, adapter.ErrNotSupported
}
func (s *stubAdapter) QuotaSource() adapter.QuotaSource { return adapter.QuotaNone }

func TestRegistry_GetAndAll(t *testing.T) {
	reg := adapter.NewRegistry()
	a := &stubAdapter{id: core.AgentClaude}
	reg.Register(a)

	got, ok := reg.Get(core.AgentClaude)
	if !ok {
		t.Fatal("Get(claude) returned false")
	}
	if got.ID() != core.AgentClaude {
		t.Errorf("ID want %q got %q", core.AgentClaude, got.ID())
	}

	_, ok = reg.Get(core.AgentGemini)
	if ok {
		t.Error("Get(gemini) returned true, want false")
	}

	all := reg.All()
	if len(all) != 1 {
		t.Errorf("All() len want 1 got %d", len(all))
	}
}

func TestRegistry_DuplicateRegisterPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for duplicate registration, got none")
		}
	}()
	reg := adapter.NewRegistry()
	reg.Register(&stubAdapter{id: core.AgentClaude})
	reg.Register(&stubAdapter{id: core.AgentClaude}) // should panic
}

func TestErrNotSupported(t *testing.T) {
	a := &stubAdapter{id: core.AgentClaude}
	_, err := a.Login(context.Background())
	if err != adapter.ErrNotSupported {
		t.Errorf("Login want ErrNotSupported got %v", err)
	}
}

func TestNewRegistryWithDefaults(t *testing.T) {
	a1 := &stubAdapter{id: core.AgentClaude}
	a2 := &stubAdapter{id: core.AgentGemini}
	reg := adapter.NewRegistryWithDefaults(a1, a2)

	if _, ok := reg.Get(core.AgentClaude); !ok {
		t.Error("claude should be in registry")
	}
	if _, ok := reg.Get(core.AgentGemini); !ok {
		t.Error("gemini should be in registry")
	}
	if len(reg.All()) != 2 {
		t.Errorf("all want 2 got %d", len(reg.All()))
	}
}
