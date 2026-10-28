package auth

import (
	"context"
	"testing"
	"time"
)

func TestTokenManagerIssueAndValidate(t *testing.T) {
	secret := "super-secret-test-key-32-chars-long"
	tm := NewTokenManager(secret, "fleetpulse-test", 1*time.Minute, 1*time.Hour)

	userID := "usr-uuid-1234"
	orgID := "org-uuid-5678"
	role := RoleOperator

	token, err := tm.GenerateAccessToken(userID, orgID, role)
	if err != nil {
		t.Fatalf("failed to generate access token: %v", err)
	}

	claims, err := tm.ValidateToken(token)
	if err != nil {
		t.Fatalf("failed to validate valid token: %v", err)
	}

	if claims.UserID != userID || claims.OrgID != orgID || claims.Role != role {
		t.Fatalf("claims mismatch: got (%s, %s, %s)", claims.UserID, claims.OrgID, claims.Role)
	}

	// Test Refresh Token
	refreshToken, err := tm.GenerateRefreshToken(userID, orgID)
	if err != nil {
		t.Fatalf("failed to generate refresh token: %v", err)
	}
	refreshClaims, err := tm.ValidateToken(refreshToken)
	if err != nil {
		t.Fatalf("failed to validate refresh token: %v", err)
	}
	if refreshClaims.Role != "refresh" || refreshClaims.UserID != userID || refreshClaims.OrgID != orgID {
		t.Fatalf("unexpected refresh claims: %+v", refreshClaims)
	}
}

func TestTokenManagerDefaultsAndInvalid(t *testing.T) {
	// Constructor with 0 durations -> defaults to 15m and 7d
	tm := NewTokenManager("secret-key", "", 0, 0)
	if tm.accessExpiry != 15*time.Minute || tm.refreshExpiry != 7*24*time.Hour {
		t.Fatalf("defaults not set: access=%v, refresh=%v", tm.accessExpiry, tm.refreshExpiry)
	}

	// Invalid token formats
	_, err := tm.ValidateToken("not.a.valid.jwt")
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken for garbage string, got %v", err)
	}
	_, err = tm.ValidateToken("")
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken for empty string, got %v", err)
	}
}

func TestTokenManagerExpiredToken(t *testing.T) {
	secret := "super-secret-test-key-32-chars-long"
	tm := NewTokenManager(secret, "fleetpulse-test", 1*time.Millisecond, 1*time.Hour)

	token, err := tm.GenerateAccessToken("u1", "o1", RoleViewer)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	time.Sleep(15 * time.Millisecond)

	_, err = tm.ValidateToken(token)
	if err != ErrExpiredToken {
		t.Fatalf("expected ErrExpiredToken, got %v", err)
	}
}

func TestTokenManagerTamperedToken(t *testing.T) {
	tm1 := NewTokenManager("secret-key-one-which-is-long-enough", "iss", 10*time.Minute, 1*time.Hour)
	tm2 := NewTokenManager("secret-key-two-which-is-long-enough", "iss", 10*time.Minute, 1*time.Hour)

	token, err := tm1.GenerateAccessToken("u1", "o1", RoleAdmin)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	_, err = tm2.ValidateToken(token)
	if err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken for mismatched secret, got %v", err)
	}
}

func TestRoleHierarchy(t *testing.T) {
	if !HasRole(RoleAdmin, RoleAdmin) {
		t.Errorf("admin should satisfy admin")
	}
	if !HasRole(RoleAdmin, RoleOperator) {
		t.Errorf("admin should satisfy operator")
	}
	if !HasRole(RoleAdmin, RoleViewer) {
		t.Errorf("admin should satisfy viewer")
	}

	if HasRole(RoleOperator, RoleAdmin) {
		t.Errorf("operator should NOT satisfy admin")
	}
	if !HasRole(RoleOperator, RoleViewer) {
		t.Errorf("operator should satisfy viewer")
	}

	if HasRole(RoleViewer, RoleOperator) {
		t.Errorf("viewer should NOT satisfy operator")
	}

	if RoleRank("unknown") != 0 {
		t.Errorf("unknown role should have rank 0")
	}
}

func TestPasswordHashing(t *testing.T) {
	pass := "CorrectHorseBatteryStaple123!"
	hash, err := HashPassword(pass)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if !CheckPasswordHash(pass, hash) {
		t.Fatalf("password check failed for correct password")
	}
	if CheckPasswordHash("wrong-password", hash) {
		t.Fatalf("password check succeeded for incorrect password")
	}
}

func TestAuthContextHelpers(t *testing.T) {
	var nilCtx context.Context
	if _, err := TenantFromContext(nilCtx); err != ErrMissingTenant {
		t.Fatalf("expected ErrMissingTenant on nil context")
	}
	if UserFromContext(nilCtx) != "" {
		t.Fatalf("expected empty user on nil context")
	}
	if RoleFromContext(nilCtx) != "" {
		t.Fatalf("expected empty role on nil context")
	}

	ctx := context.Background()
	_, err := TenantFromContext(ctx)
	if err != ErrMissingTenant {
		t.Fatalf("expected ErrMissingTenant on clean context")
	}
	if UserFromContext(ctx) != "" || RoleFromContext(ctx) != "" {
		t.Fatalf("expected empty user and role on empty context")
	}

	ctx = WithAuthContext(ctx, "usr-1", "org-1", RoleAdmin)
	tenant, err := TenantFromContext(ctx)
	if err != nil || tenant != "org-1" {
		t.Fatalf("failed to retrieve tenant: %v", err)
	}
	if UserFromContext(ctx) != "usr-1" {
		t.Fatalf("failed to retrieve user: got %s", UserFromContext(ctx))
	}
	if RoleFromContext(ctx) != RoleAdmin {
		t.Fatalf("failed to retrieve role: got %s", RoleFromContext(ctx))
	}
}
