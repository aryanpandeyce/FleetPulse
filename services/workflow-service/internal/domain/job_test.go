package domain

import (
	"testing"
)

func TestJob_IsTerminal(t *testing.T) {
	job := &Job{State: StateQueued}
	if job.IsTerminal() {
		t.Fatalf("expected queued job not to be terminal")
	}

	job.State = StateRunning
	if job.IsTerminal() {
		t.Fatalf("expected running job not to be terminal")
	}

	for _, s := range []string{StateSucceeded, StateFailed, StateCancelled} {
		job.State = s
		if !job.IsTerminal() {
			t.Fatalf("expected %s to be terminal", s)
		}
	}
}
