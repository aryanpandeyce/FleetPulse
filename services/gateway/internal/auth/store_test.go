package auth

import (
	"context"
	"testing"
)

func TestSeededUserStore(t *testing.T) {
	store := NewSeededUserStore()
	ctx := context.Background()

	// 1. Successful authentication
	user, err := store.Authenticate(ctx, "admin@acme.com", "AdminPass123!")
	if err != nil {
		t.Fatalf("expected valid auth, got err: %v", err)
	}
	if user.Email != "admin@acme.com" || user.Role != "admin" {
		t.Fatalf("unexpected user: %+v", user)
	}

	// 2. Invalid password
	_, err = store.Authenticate(ctx, "admin@acme.com", "WrongPassword")
	if err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound for bad password, got %v", err)
	}

	// 3. Unknown user
	_, err = store.Authenticate(ctx, "unknown@company.com", "AnyPassword")
	if err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound for unknown user, got %v", err)
	}
}
