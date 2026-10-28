// Package auth provides user authentication and seeded credentials for Gateway.
package auth

import (
	"context"
	"errors"
	"sync"

	"github.com/fleetpulse/fleetpulse/pkg/auth"
)

var ErrUserNotFound = errors.New("user not found")

// User represents a user record in the authentication store.
type User struct {
	ID           string
	OrgID        string
	Email        string
	PasswordHash string
	Role         string
}

// UserStore manages user retrieval and password verification.
type UserStore struct {
	mu    sync.RWMutex
	users map[string]*User // email -> user
}

// NewSeededUserStore initializes a user store with standard demo accounts for 2 tenants.
func NewSeededUserStore() *UserStore {
	store := &UserStore{
		users: make(map[string]*User),
	}

	// Helper to hash and store
	addUser := func(id, orgID, email, plainPassword, role string) {
		hash, _ := auth.HashPassword(plainPassword)
		store.users[email] = &User{
			ID:           id,
			OrgID:        orgID,
			Email:        email,
			PasswordHash: hash,
			Role:         role,
		}
	}

	// Tenant 1: Acme Corporation
	orgAcme := "c827b56a-1234-4b55-a22b-8a89201f92e1"
	addUser("usr-acme-admin", orgAcme, "admin@acme.com", "AdminPass123!", auth.RoleAdmin)
	addUser("usr-acme-operator", orgAcme, "operator@acme.com", "OperatorPass123!", auth.RoleOperator)
	addUser("usr-acme-viewer", orgAcme, "viewer@acme.com", "ViewerPass123!", auth.RoleViewer)

	// Tenant 2: Globex Industries
	orgGlobex := "d938c67b-5678-4c66-b33c-9b90312fa3f2"
	addUser("usr-globex-admin", orgGlobex, "admin@globex.com", "AdminPass123!", auth.RoleAdmin)
	addUser("usr-globex-operator", orgGlobex, "operator@globex.com", "OperatorPass123!", auth.RoleOperator)
	addUser("usr-globex-viewer", orgGlobex, "viewer@globex.com", "ViewerPass123!", auth.RoleViewer)

	return store
}

// Authenticate verifies email and password, returning the user if valid.
func (s *UserStore) Authenticate(ctx context.Context, email, password string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, exists := s.users[email]
	if !exists {
		return nil, ErrUserNotFound
	}

	if !auth.CheckPasswordHash(password, user.PasswordHash) {
		return nil, ErrUserNotFound // Avoid timing leaks
	}

	return user, nil
}
