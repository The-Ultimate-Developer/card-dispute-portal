package auth

import (
	"context"
	"time"
)

type Role string

const (
	RoleCustomer Role = "CUSTOMER"
	RoleAgent    Role = "AGENT"
	RoleSupport  Role = "SUPPORT"
	RoleAdmin    Role = "ADMIN"
)

// Staff covers every back-office role.
func (r Role) Staff() bool {
	return r == RoleAgent || r == RoleSupport || r == RoleAdmin
}

type User struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"fullName"`
	Role     Role   `json:"role"`
}

type LockedAccount struct {
	ID         string    `json:"id"`
	Email      string    `json:"email"`
	FullName   string    `json:"fullName"`
	LockedAt   time.Time `json:"lockedAt"`
	LockReason string    `json:"lockReason"`
}

// Principal is the authenticated caller, resolved from the session cookie.
type Principal struct {
	UserID      string
	Role        Role
	CSRFToken   string
	SessionHash []byte
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

type Event string

const (
	EventLoginSucceeded  Event = "LOGIN_SUCCEEDED"
	EventLoginFailed     Event = "LOGIN_FAILED"
	EventLoginBlocked    Event = "LOGIN_BLOCKED"
	EventAccountLocked   Event = "ACCOUNT_LOCKED"
	EventAccountUnlocked Event = "ACCOUNT_UNLOCKED"
	EventLogout          Event = "LOGOUT"
)
