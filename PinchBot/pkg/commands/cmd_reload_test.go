package commands

import (
	"context"
	"errors"
	"testing"
)

func TestReloadCommand_Unavailable(t *testing.T) {
	ex := NewExecutor(NewRegistry(BuiltinDefinitions()), &Runtime{})

	var reply string
	res := ex.Execute(context.Background(), Request{
		Text: "/reload",
		Reply: func(text string) error {
			reply = text
			return nil
		},
	})

	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome = %v, want %v", res.Outcome, OutcomeHandled)
	}
	if reply != unavailableMsg {
		t.Fatalf("reply = %q, want %q", reply, unavailableMsg)
	}
}

func TestReloadCommand_Error(t *testing.T) {
	ex := NewExecutor(NewRegistry(BuiltinDefinitions()), &Runtime{
		ReloadConfig: func() error {
			return errors.New("boom")
		},
	})

	var reply string
	res := ex.Execute(context.Background(), Request{
		Text: "/reload",
		Reply: func(text string) error {
			reply = text
			return nil
		},
	})

	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome = %v, want %v", res.Outcome, OutcomeHandled)
	}
	if reply != "Failed to trigger reload: boom" {
		t.Fatalf("reply = %q, want %q", reply, "Failed to trigger reload: boom")
	}
}

func TestReloadCommand_Success(t *testing.T) {
	called := false
	ex := NewExecutor(NewRegistry(BuiltinDefinitions()), &Runtime{
		ReloadConfig: func() error {
			called = true
			return nil
		},
	})

	var reply string
	res := ex.Execute(context.Background(), Request{
		Text: "/reload",
		Reply: func(text string) error {
			reply = text
			return nil
		},
	})

	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome = %v, want %v", res.Outcome, OutcomeHandled)
	}
	if !called {
		t.Fatal("expected ReloadConfig to be called")
	}
	if reply != "Reload triggered." {
		t.Fatalf("reply = %q, want %q", reply, "Reload triggered.")
	}
}
