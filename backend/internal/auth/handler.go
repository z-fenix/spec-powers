package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userDTO struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/register", h.register)
	r.Post("/login", h.login)
	r.Group(func(r chi.Router) {
		r.Use(RequireAuth(h.svc.tokens))
		r.Get("/me", h.me)
		r.Post("/tokens", h.issueToken)
		r.Get("/tokens", h.listTokens)
		r.Delete("/tokens/{tokenID}", h.revokeToken)
	})
	return r
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	token, user, err := h.svc.Register(r.Context(), req.Email, req.Password, req.DisplayName)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"token": token, "user": toDTO(user)})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	token, user, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"token": token, "user": toDTO(user)})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	user, err := h.svc.User(r.Context(), UserIDFrom(r.Context()))
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"user": toDTO(user)})
}

type apiTokenDTO struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
	RevokedAt  *string `json:"revoked_at,omitempty"`
}

func toTokenDTO(t *domain.APIToken) apiTokenDTO {
	format := func(ts *time.Time) *string {
		if ts == nil {
			return nil
		}
		s := ts.UTC().Format(time.RFC3339)
		return &s
	}
	return apiTokenDTO{
		ID: t.ID, Name: t.Name, Prefix: t.Prefix,
		CreatedAt:  t.CreatedAt.UTC().Format(time.RFC3339),
		LastUsedAt: format(t.LastUsedAt),
		RevokedAt:  format(t.RevokedAt),
	}
}

// issueToken creates a personal API token; the plaintext appears in this
// response only and cannot be retrieved again.
func (h *Handler) issueToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	plaintext, tok, err := h.svc.IssueAPIToken(r.Context(), UserIDFrom(r.Context()), req.Name)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"token": plaintext, "token_record": toTokenDTO(tok)})
}

func (h *Handler) listTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.svc.ListAPITokens(r.Context(), UserIDFrom(r.Context()))
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]apiTokenDTO, 0, len(tokens))
	for i := range tokens {
		dtos = append(dtos, toTokenDTO(&tokens[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"tokens": dtos})
}

func (h *Handler) revokeToken(w http.ResponseWriter, r *http.Request) {
	_, err := h.svc.RevokeAPIToken(r.Context(), UserIDFrom(r.Context()), chi.URLParam(r, "tokenID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeAppError(w http.ResponseWriter, err error) {
	if appErr, ok := errors.AsType[*httpapi.AppError](err); ok {
		httpapi.Error(w, appErr)
		return
	}
	httpapi.Error(w, httpapi.ErrInternal("internal server error"))
}

func toDTO(u *domain.User) userDTO {
	return userDTO{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName}
}
