package auction

import (
	"testing"
	"time"
)

func TestCanTransitionDraftToOpen(t *testing.T) {
	now := time.Now().UTC()
	if !CanTransition(StateDraft, StateOpen, now, now.Add(-time.Minute), now.Add(time.Minute)) {
		t.Fatal("expected transition to open")
	}
}

func TestCanTransitionRejectInvalid(t *testing.T) {
	now := time.Now().UTC()
	if CanTransition(StateClosed, StateOpen, now, now.Add(-time.Minute), now.Add(time.Minute)) {
		t.Fatal("unexpected transition allowed")
	}
}

func TestIsOpen(t *testing.T) {
	now := time.Now().UTC()
	if !IsOpen(StateOpen, now, now.Add(-time.Minute), now.Add(time.Minute)) {
		t.Fatal("expected open auction")
	}
}
