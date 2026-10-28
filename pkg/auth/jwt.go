// Package auth provides cryptographic primitives, JWT token issuing and validation,
// and role-based access control (RBAC) definitions for FleetPulse.
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Standard Role definitions
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
)

var (
	ErrInvalidToken   = errors.New("invalid or malformed token")
	ErrExpiredToken   = errors.New("token has expired")
	ErrForbiddenRole  = errors.New("insufficient permissions for this role")
	ErrMissingTenant  = errors.New("missing tenant organization context")
	ErrAlgMismatch    = errors.New("unexpected signing algorithm")
)

type contextKey string

const (
	tenantKey contextKey = "fp_auth_tenant"
	userKey   contextKey = "fp_auth_user"
	roleKey   contextKey = "fp_auth_role"
)

// Claims represents the standard JWT claims payload for FleetPulse.
type Claims struct {
	UserID string `json:"sub"`
	OrgID  string `json:"org_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// TokenManager signs and validates JWT access and refresh tokens.
type TokenManager struct {
	secretKey     []byte
	issuer        string
	accessExpiry  time.Duration
	refreshExpiry time.Duration
}

// NewTokenManager creates a new JWT token manager with configurable expirations.
func NewTokenManager(secret string, issuer string, accessExpiry, refreshExpiry time.Duration) *TokenManager {
	if accessExpiry <= 0 {
		accessExpiry = 15 * time.Minute
	}
	if refreshExpiry <= 0 {
		refreshExpiry = 7 * 24 * time.Hour
	}
	return &TokenManager{
		secretKey:     []byte(secret),
		issuer:        issuer,
		accessExpiry:  accessExpiry,
		refreshExpiry: refreshExpiry,
	}
}

// GenerateAccessToken signs a short-lived access token with tenant and role claims.
func (tm *TokenManager) GenerateAccessToken(userID, orgID, role string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		OrgID:  orgID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tm.issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tm.accessExpiry)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(tm.secretKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign access token: %w", err)
	}
	return signed, nil
}

// GenerateRefreshToken creates a long-lived refresh token.
func (tm *TokenManager) GenerateRefreshToken(userID, orgID string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		OrgID:  orgID,
		Role:   "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tm.issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tm.refreshExpiry)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(tm.secretKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign refresh token: %w", err)
	}
	return signed, nil
}

// ValidateToken verifies token signature and expiration, pinning the algorithm strictly to HS256.
func (tm *TokenManager) ValidateToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrAlgMismatch
		}
		return tm.secretKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// HashPassword hashes a raw password using bcrypt with standard cost factor 12.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(bytes), nil
}

// CheckPasswordHash verifies a raw password against an existing bcrypt hash.
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// RoleRank returns numeric ordering for roles: admin (3) > operator (2) > viewer (1).
func RoleRank(role string) int {
	switch role {
	case RoleAdmin:
		return 3
	case RoleOperator:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

// HasRole checks whether the user's role meets or exceeds required role level.
func HasRole(userRole, requiredRole string) bool {
	return RoleRank(userRole) >= RoleRank(requiredRole)
}

// Context helpers for attaching and extracting auth claims

// WithAuthContext stores user ID, org ID, and role in context.
func WithAuthContext(ctx context.Context, userID, orgID, role string) context.Context {
	ctx = context.WithValue(ctx, userKey, userID)
	ctx = context.WithValue(ctx, tenantKey, orgID)
	ctx = context.WithValue(ctx, roleKey, role)
	return ctx
}

// TenantFromContext extracts the org ID from context.
func TenantFromContext(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", ErrMissingTenant
	}
	val, ok := ctx.Value(tenantKey).(string)
	if !ok || val == "" {
		return "", ErrMissingTenant
	}
	return val, nil
}

// UserFromContext extracts the user ID from context.
func UserFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	val, ok := ctx.Value(userKey).(string)
	if !ok {
		return ""
	}
	return val
}

// RoleFromContext extracts the user role from context.
func RoleFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	val, ok := ctx.Value(roleKey).(string)
	if !ok {
		return ""
	}
	return val
}
