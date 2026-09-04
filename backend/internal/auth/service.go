package auth

import (
	"context"
	"errors"
	"regexp"

	"golang.org/x/crypto/bcrypt"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type Service struct {
	users      store.UserStore
	workspaces store.WorkspaceStore
	members    store.MemberStore
	tokens     *TokenService
}

func NewService(users store.UserStore, workspaces store.WorkspaceStore, members store.MemberStore, tokens *TokenService) *Service {
	return &Service{users: users, workspaces: workspaces, members: members, tokens: tokens}
}

// Register creates the user and their default workspace and doubles as
// first login: it returns an issued token alongside the user (PasswordHash
// cleared), mirroring /auth/login so clients are authenticated right away.
func (s *Service) Register(ctx context.Context, email, password, displayName string) (string, *domain.User, error) {
	if !emailRe.MatchString(email) {
		return "", nil, ErrInvalid("invalid email address")
	}
	if len(password) < 8 {
		return "", nil, ErrInvalid("password must be at least 8 characters")
	}
	if displayName == "" {
		return "", nil, ErrInvalid("display name is required")
	}

	_, err := s.users.GetUserByEmail(ctx, email)
	if err == nil {
		return "", nil, httpapi.ErrConflict("email already registered")
	}
	if !errors.Is(err, store.ErrNotFound) {
		return "", nil, httpapi.ErrInternal("lookup user failed")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", nil, httpapi.ErrInternal("hash password failed")
	}
	user, err := s.users.CreateUser(ctx, email, string(hash), displayName)
	if err != nil {
		return "", nil, httpapi.ErrInternal("create user failed")
	}

	ws, err := s.workspaces.CreateWorkspace(ctx, displayName, user.ID)
	if err != nil {
		return "", nil, httpapi.ErrInternal("create default workspace failed")
	}
	if err := s.members.AddMember(ctx, ws.ID, user.ID, store.RoleOwner); err != nil {
		return "", nil, httpapi.ErrInternal("add workspace member failed")
	}

	token, err := s.tokens.Issue(user.ID)
	if err != nil {
		return "", nil, httpapi.ErrInternal("issue token failed")
	}
	user.PasswordHash = ""
	return token, user, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (string, *domain.User, error) {
	user, err := s.users.GetUserByEmail(ctx, email)
	if errors.Is(err, store.ErrNotFound) {
		return "", nil, ErrUnauthorized("invalid email or password")
	}
	if err != nil {
		return "", nil, httpapi.ErrInternal("lookup user failed")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", nil, ErrUnauthorized("invalid email or password")
	}
	token, err := s.tokens.Issue(user.ID)
	if err != nil {
		return "", nil, httpapi.ErrInternal("issue token failed")
	}
	user.PasswordHash = ""
	return token, user, nil
}

func (s *Service) TokenService() *TokenService { return s.tokens }

func (s *Service) User(ctx context.Context, id string) (*domain.User, error) {
	user, err := s.users.GetUser(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpapi.ErrNotFound("user not found")
	}
	if err != nil {
		return nil, httpapi.ErrInternal("lookup user failed")
	}
	user.PasswordHash = ""
	return user, nil
}

func ErrInvalid(msg string) *httpapi.AppError { return &httpapi.AppError{Status: 400, Code: "invalid_request", Message: msg} }
func ErrUnauthorized(msg string) *httpapi.AppError {
	return &httpapi.AppError{Status: 401, Code: "unauthorized", Message: msg}
}
