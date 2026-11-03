package auth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/testdb"
)

const password = "Correct-Horse-1"

type fixture struct {
	svc    *Service
	repo   *Repository
	userID string
	email  string
}

func setup(t *testing.T) fixture {
	t.Helper()
	db := testdb.New(t)
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	email := "thandi@example.com"
	id := testdb.CreateUser(t, db, email, string(RoleCustomer), hash)
	repo := NewRepository(db)
	svc := NewService(repo, Config{MaxFailedAttempts: 3, IdleTimeout: 15 * time.Minute, AbsoluteTimeout: 8 * time.Hour},
		slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	return fixture{svc: svc, repo: repo, userID: id, email: email}
}

func (f fixture) login(pw string) error {
	_, err := f.svc.Login(context.Background(), f.email, pw, "127.0.0.1")
	return err
}

func TestLoginSucceedsAndSessionAuthenticates(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	res, err := f.svc.Login(ctx, "  THANDI@example.com ", password, "127.0.0.1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if res.User.ID != f.userID || res.CSRFToken == "" || res.SessionToken == "" {
		t.Fatalf("unexpected result: %+v", res)
	}

	p, err := f.svc.Authenticate(ctx, res.SessionToken)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if p.UserID != f.userID || p.Role != RoleCustomer || p.CSRFToken != res.CSRFToken {
		t.Fatalf("unexpected principal: %+v", p)
	}
}

func TestThirdFailureLocksAccount(t *testing.T) {
	f := setup(t)

	for i := 1; i <= 2; i++ {
		if err := f.login("wrong"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: got %v, want invalid credentials", i, err)
		}
	}
	if err := f.login("wrong"); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("attempt 3: got %v, want locked", err)
	}
	if err := f.login(password); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("correct password after lock: got %v, want locked", err)
	}
}

func TestSuccessResetsFailureCount(t *testing.T) {
	f := setup(t)

	_ = f.login("wrong")
	_ = f.login("wrong")
	if err := f.login(password); err != nil {
		t.Fatalf("login: %v", err)
	}
	for i := 1; i <= 2; i++ {
		if err := f.login("wrong"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("counter should have reset; attempt %d got %v", i, err)
		}
	}
}

// 20 wrong guesses fired at once must still evaluate at most 3 passwords.
func TestConcurrentGuessesCannotExceedLimit(t *testing.T) {
	f := setup(t)

	var wg sync.WaitGroup
	results := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- f.login("wrong")
		}()
	}
	wg.Wait()
	close(results)

	invalid, locked := 0, 0
	for err := range results {
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			invalid++
		case errors.Is(err, ErrAccountLocked):
			locked++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if invalid != 2 || locked != 18 {
		t.Fatalf("got %d invalid / %d locked, want 2 / 18", invalid, locked)
	}
}

func TestUnknownEmailBehavesLikeRealAccount(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	for i := 1; i <= 2; i++ {
		if _, err := f.svc.Login(ctx, "nobody@example.com", "x", ""); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d: got %v", i, err)
		}
	}
	if _, err := f.svc.Login(ctx, "nobody@example.com", "x", ""); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("attempt 3: got %v, want the same lockout as a real account", err)
	}
}

func TestLockRevokesExistingSessions(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	res, err := f.svc.Login(ctx, f.email, password, "")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		_ = f.login("wrong")
	}
	if _, err := f.svc.Authenticate(ctx, res.SessionToken); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("session should be revoked after lock, got %v", err)
	}
}

func TestSupportUnlockRestoresAccess(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = f.login("wrong")
	}

	locked, err := f.svc.LockedAccounts(ctx)
	if err != nil || len(locked) != 1 || locked[0].ID != f.userID {
		t.Fatalf("locked accounts: %+v %v", locked, err)
	}

	if err := f.svc.Unlock(ctx, Principal{UserID: f.userID}, f.userID, ""); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if err := f.login(password); err != nil {
		t.Fatalf("login after unlock: %v", err)
	}
	if err := f.svc.Unlock(ctx, Principal{}, f.userID, ""); apperrCode(err) != "ACCOUNT_NOT_LOCKED" {
		t.Fatalf("unlocking an unlocked account: got %v", err)
	}
}

func TestIdleSessionExpires(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	res, err := f.svc.Login(ctx, f.email, password, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.db.Exec(ctx, `UPDATE sessions SET last_seen_at = now() - interval '16 minutes'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Authenticate(ctx, res.SessionToken); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("idle session should be rejected, got %v", err)
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	res, _ := f.svc.Login(ctx, f.email, password, "")
	p, _ := f.svc.Authenticate(ctx, res.SessionToken)
	if err := f.svc.Logout(ctx, p, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Authenticate(ctx, res.SessionToken); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("session should be gone after logout, got %v", err)
	}
}

func TestAuditTrailIsAppendOnly(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_ = f.login("wrong")

	var n int
	if err := f.repo.db.QueryRow(ctx, `SELECT count(*) FROM auth_events WHERE event = 'LOGIN_FAILED'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("expected one LOGIN_FAILED event, got %d (%v)", n, err)
	}
	if _, err := f.repo.db.Exec(ctx, `DELETE FROM auth_events`); err == nil {
		t.Fatal("deleting audit events must be refused by the database")
	}
}
