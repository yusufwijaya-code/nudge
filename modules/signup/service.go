package signup

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"nudge/modules/registration/registration_dto"
	"nudge/modules/registration/registration_service"
)

var (
	// ErrGroupUnavailable: the Google Group step failed (the operator token is
	// dead, the group is not reachable, or Google refused the add). Nothing was
	// created; the visitor must ask an admin.
	ErrGroupUnavailable = errors.New("signup: google group unavailable")
	// ErrDeactivated: the account exists but an admin switched it off.
	ErrDeactivated = errors.New("signup: account deactivated")
)

// Group is the Google Group that makes the Nudge Chat App visible.
type Group interface {
	// Check proves the operator token and the group still work.
	Check(ctx context.Context) error
	AddMember(ctx context.Context, email string) error
}

type Directory interface {
	FindUserByEmail(ctx context.Context, email string) (*registration_dto.User, bool, error)
}

// JiraLookup asks Jira whether an account exists for email.
type JiraLookup interface {
	LookupUserByEmail(ctx context.Context, email string) (accountID string, found bool, err error)
}

type Registrar interface {
	Register(ctx context.Context, name, email string) error
}

// Store records, once the account exists, that the user is in the group.
type Store interface {
	MarkAdded(ctx context.Context, email string) error
}

type Deps struct {
	// Group is nil when auto-join is not configured: registration then skips the group.
	Group     Group
	Directory Directory
	Jira      JiraLookup
	Registrar Registrar
	Store     Store
}

type Service struct {
	d Deps
	// mu serializes sign-ups: a double click must not add the user twice, and
	// registrations are rare enough that waiting is fine.
	mu sync.Mutex
}

func NewService(d Deps) *Service { return &Service{d: d} }

// IsNew reports whether email has no account yet.
func (s *Service) IsNew(ctx context.Context, email string) (bool, error) {
	_, found, err := s.d.Directory.FindUserByEmail(ctx, email)
	return !found, err
}

// Register creates the account in this order, stopping at the first failure so
// that nothing is created until the group accepted the user:
//  1. the group token and the group are checked,
//  2. the Jira account is looked up (nobody without one is added to the group),
//  3. the user is added to the group,
//  4. the account is created.
func (s *Service) Register(ctx context.Context, name, email string) (*registration_dto.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if u, found, err := s.d.Directory.FindUserByEmail(ctx, email); err != nil {
		return nil, err
	} else if found {
		if !u.FlagActive {
			return nil, ErrDeactivated
		}
		return u, nil
	}

	if s.d.Group != nil {
		if err := s.d.Group.Check(ctx); err != nil {
			log.Printf("signup: ERROR the Google Group check failed, %s was not registered: %v", email, err)
			return nil, fmt.Errorf("%w: %v", ErrGroupUnavailable, err)
		}
	}

	_, found, err := s.d.Jira.LookupUserByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", registration_service.ErrJiraLookupUnavailable, err)
	}
	if !found {
		return nil, registration_service.ErrJiraAccountNotFound
	}

	if s.d.Group != nil {
		if err := s.d.Group.AddMember(ctx, email); err != nil {
			log.Printf("signup: ERROR adding %s to the Google Group failed, not registered: %v", email, err)
			return nil, fmt.Errorf("%w: %v", ErrGroupUnavailable, err)
		}
	}

	if err := s.d.Registrar.Register(ctx, name, email); err != nil {
		return nil, err
	}
	u, found, err := s.d.Directory.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("signup: user %s not found right after registration", email)
	}
	if s.d.Group != nil && s.d.Store != nil {
		if err := s.d.Store.MarkAdded(ctx, email); err != nil {
			log.Printf("signup: marking %s as added to the group failed: %v", email, err)
		}
	}
	return u, nil
}
