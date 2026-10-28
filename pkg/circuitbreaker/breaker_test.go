package circuitbreaker

import (
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_Lifecycle(t *testing.T) {
	cb := New(Config{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Cooldown:         50 * time.Millisecond,
	})

	if cb.State() != StateClosed {
		t.Fatalf("expected initial state Closed, got %s", cb.State())
	}

	simErr := errors.New("downstream service failure")

	// 1. First 2 failures -> remains Closed
	for i := 0; i < 2; i++ {
		err := cb.Execute(func() error { return simErr })
		if err != simErr {
			t.Fatalf("expected simErr, got %v", err)
		}
	}
	if cb.State() != StateClosed {
		t.Fatalf("expected Closed after 2 failures, got %s", cb.State())
	}

	// 2. 3rd failure trips circuit Open!
	_ = cb.Execute(func() error { return simErr })
	if cb.State() != StateOpen {
		t.Fatalf("expected Open after 3rd failure, got %s", cb.State())
	}

	// 3. Request rejected immediately while Open without executing fn
	invoked := false
	err := cb.Execute(func() error {
		invoked = true
		return nil
	})
	if err != ErrCircuitOpen || invoked {
		t.Fatalf("expected ErrCircuitOpen without invocation, got err=%v, invoked=%v", err, invoked)
	}

	// 4. Wait for cooldown to expire -> transitions to Half-Open
	time.Sleep(60 * time.Millisecond)
	if cb.State() != StateHalfOpen {
		t.Fatalf("expected HalfOpen after cooldown, got %s", cb.State())
	}

	// 5. Successful probe 1 in Half-Open
	err = cb.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("expected success on half-open probe, got %v", err)
	}

	// 6. Successful probe 2 in Half-Open -> resets circuit to Closed!
	err = cb.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if cb.State() != StateClosed {
		t.Fatalf("expected circuit to close after 2 successes, got %s", cb.State())
	}

	// 7. Test Half-Open failure reopens circuit immediately
	// Trip circuit Open again
	for i := 0; i < 3; i++ {
		_ = cb.Execute(func() error { return simErr })
	}
	time.Sleep(60 * time.Millisecond) // Wait to reach HalfOpen
	_ = cb.Execute(func() error { return simErr })
	if cb.State() != StateOpen {
		t.Fatalf("expected immediate reopen on half-open failure, got %s", cb.State())
	}
}
