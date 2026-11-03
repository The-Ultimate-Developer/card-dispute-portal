package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/apperr"
	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/internal/platform/validate"
)

var (
	ErrInvalidCredentials = apperr.Unauthorized("INVALID_CREDENTIALS", "Incorrect email or password.")
	ErrAccountLocked      = apperr.Locked("ACCOUNT_LOCKED",
		"For your security, sign-in has been blocked. Please contact support for assistance.")
	ErrUnauthenticated = apperr.Unauthorized("UNAUTHENTICATED", "Please sign in to continue.")
)

type Config struct {
	MaxFailedAttempts int
	IdleTimeout       time.Duration
	AbsoluteTimeout   time.Duration
}

type LoginResult struct {
	User         User
	SessionToken string
	CSRFToken    string
	ExpiresAt    time.Time
}

type Observer interface {
	ObserveLogin(outcome string)
}

type nopObserver struct{}

func (nopObserver) ObserveLogin(string) {}

type Service struct {
	repo *Repository
	cfg  Config
	log  *slog.Logger
	obs  Observer
}

func NewService(repo *Repository, cfg Config, log *slog.Logger, obs Observer) *Service {
	if obs == nil {
		obs = nopObserver{}
	}
	return &Service{repo: repo, cfg: cfg, log: log, obs: obs}
}

// Login enforces lockout before the password is ever checked:
//  1. a locked account is refused outright, with no password check;
//  2. the attempt is counted atomically first, so parallel guesses cannot
//     slip past the limit;
//  3. unknown emails take the same time and reach the same lockout, so the
//     response never reveals whether an account exists.
func (s *Service) Login(ctx context.Context, email, password, ip string) (LoginResult, error) {
	email = strings.TrimSpace(email)

	creds, err := s.repo.findCredentials(ctx, email)
	found := err == nil
	if err != nil && !errors.Is(err, errNoRows) {
		return LoginResult{}, err
	}

	if found && creds.Locked {
		s.audit(ctx, auditEvent{Event: EventLoginBlocked, Email: email, UserID: creds.ID, IP: ip})
		s.obs.ObserveLogin("blocked")
		return LoginResult{}, ErrAccountLocked
	}

	attempt, err := s.repo.claimAttempt(ctx, email)
	if err != nil {
		return LoginResult{}, err
	}
	if attempt > s.cfg.MaxFailedAttempts {
		s.audit(ctx, auditEvent{Event: EventLoginBlocked, Email: email, UserID: creds.ID, IP: ip})
		s.obs.ObserveLogin("blocked")
		return LoginResult{}, ErrAccountLocked
	}

	valid := false
	if found {
		if valid, err = VerifyPassword(creds.PasswordHash, password); err != nil {
			return LoginResult{}, err
		}
	} else {
		equaliseTiming(password)
	}

	if valid {
		res, err := s.startSession(ctx, creds.User, ip)
		if err == nil {
			s.obs.ObserveLogin("success")
		}
		return res, err
	}

	s.audit(ctx, auditEvent{Event: EventLoginFailed, Email: email, UserID: creds.ID, IP: ip})
	if attempt < s.cfg.MaxFailedAttempts {
		s.obs.ObserveLogin("invalid_credentials")
		return LoginResult{}, ErrInvalidCredentials
	}

	if found {
		if err := s.repo.lockUser(ctx, creds.ID, "Too many failed sign-in attempts"); err != nil {
			return LoginResult{}, err
		}
	}
	s.audit(ctx, auditEvent{Event: EventAccountLocked, Email: email, UserID: creds.ID, IP: ip})
	s.obs.ObserveLogin("locked_out")
	return LoginResult{}, ErrAccountLocked
}

func (s *Service) startSession(ctx context.Context, u User, ip string) (LoginResult, error) {
	if err := s.repo.clearAttempts(ctx, u.Email); err != nil {
		return LoginResult{}, err
	}

	token, csrf := randomToken(), randomToken()
	expiresAt := time.Now().Add(s.cfg.AbsoluteTimeout)
	if err := s.repo.createSession(ctx, newSession{
		TokenHash: hashToken(token), UserID: u.ID, CSRFToken: csrf, ExpiresAt: expiresAt,
	}); err != nil {
		return LoginResult{}, err
	}

	s.audit(ctx, auditEvent{Event: EventLoginSucceeded, Email: u.Email, UserID: u.ID, IP: ip})
	return LoginResult{User: u, SessionToken: token, CSRFToken: csrf, ExpiresAt: expiresAt}, nil
}

func (s *Service) Authenticate(ctx context.Context, sessionToken string) (Principal, error) {
	p, err := s.repo.touchSession(ctx, hashToken(sessionToken), s.cfg.IdleTimeout)
	if errors.Is(err, errNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	return p, err
}

func (s *Service) Logout(ctx context.Context, p Principal, ip string) error {
	if err := s.repo.deleteSession(ctx, p.SessionHash); err != nil {
		return err
	}
	s.audit(ctx, auditEvent{Event: EventLogout, UserID: p.UserID, IP: ip})
	return nil
}

func (s *Service) CurrentUser(ctx context.Context, p Principal) (User, error) {
	u, err := s.repo.getUser(ctx, p.UserID)
	if errors.Is(err, errNoRows) {
		return User{}, ErrUnauthenticated
	}
	return u, err
}

func (s *Service) LockedAccounts(ctx context.Context) ([]LockedAccount, error) {
	return s.repo.lockedAccounts(ctx)
}

func (s *Service) Unlock(ctx context.Context, actor Principal, userID, ip string) error {
	notLocked := apperr.NotFound("ACCOUNT_NOT_LOCKED", "No locked account with this ID.")
	if !validate.UUID(userID) {
		return notLocked
	}
	email, err := s.repo.unlockUser(ctx, userID)
	if errors.Is(err, errNoRows) {
		return notLocked
	}
	if err != nil {
		return err
	}
	s.audit(ctx, auditEvent{Event: EventAccountUnlocked, Email: email, UserID: userID, ActorID: actor.UserID, IP: ip})
	return nil
}

func (s *Service) RunJanitor(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := s.repo.deleteExpiredSessions(ctx, s.cfg.IdleTimeout)
			if err != nil {
				s.log.WarnContext(ctx, "session cleanup failed", "error", err)
				continue
			}
			if n > 0 {
				s.log.InfoContext(ctx, "expired sessions removed", "count", n)
			}
		}
	}
}

// audit is best-effort: a failed audit write is logged loudly but does not
// change the outcome the user sees.
func (s *Service) audit(ctx context.Context, e auditEvent) {
	if err := s.repo.recordEvent(ctx, e); err != nil {
		s.log.ErrorContext(ctx, "audit write failed", "event", e.Event, "error", err)
	}
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Only the hash is stored, so a database leak does not hand out live sessions.
func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
