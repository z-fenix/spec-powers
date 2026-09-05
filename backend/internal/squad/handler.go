package squad

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
)

// Handler serves the squad REST endpoints mounted at /squads.
type Handler struct {
	svc    *Service
	tokens *auth.TokenService
}

func NewHandler(svc *Service, tokens *auth.TokenService) *Handler {
	return &Handler{svc: svc, tokens: tokens}
}

// Routes mounts the squad endpoints behind authentication.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(auth.RequireAuth(h.tokens))
	r.Post("/", h.createSquad)
	r.Get("/", h.listSquads)
	r.Route("/{squadID}", func(r chi.Router) {
		r.Get("/", h.getSquad)
		r.Patch("/", h.updateSquad)
		r.Delete("/", h.removeSquad)
		r.Post("/leader", h.setLeader)
		r.Post("/members", h.addMember)
		r.Delete("/members/{userID}", h.removeMember)
	})
	return r
}

func writeAppError(w http.ResponseWriter, err error) {
	var appErr *httpapi.AppError
	if errors.As(err, &appErr) {
		httpapi.Error(w, appErr)
		return
	}
	httpapi.Error(w, httpapi.ErrInternal("internal server error"))
}

type memberDTO struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	IsAgent     bool   `json:"is_agent"`
}

func toMemberDTOs(list []domain.SquadMemberDetail) []memberDTO {
	out := make([]memberDTO, 0, len(list))
	for _, m := range list {
		out = append(out, memberDTO{UserID: m.UserID, DisplayName: m.DisplayName, IsAgent: m.IsAgent})
	}
	return out
}

type squadDTO struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	LeaderID    string       `json:"leader_id"`
	CreatedBy   string       `json:"created_by"`
	Members     []memberDTO  `json:"members,omitempty"`
}

func toSquadDTO(s *domain.Squad) squadDTO {
	return squadDTO{ID: s.ID, Name: s.Name, Description: s.Description, LeaderID: s.LeaderID, CreatedBy: s.CreatedBy}
}

func (h *Handler) createSquad(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		LeaderID    string `json:"leader_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	sq, err := h.svc.CreateSquad(r.Context(), auth.UserIDFrom(r.Context()), CreateInput{
		Name:        req.Name,
		Description: req.Description,
		LeaderID:    req.LeaderID,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusCreated, map[string]any{"squad": toSquadDTO(sq)})
}

func (h *Handler) listSquads(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListSquads(r.Context())
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]squadDTO, 0, len(list))
	for i := range list {
		dtos = append(dtos, toSquadDTO(&list[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"squads": dtos})
}

func (h *Handler) getSquad(w http.ResponseWriter, r *http.Request) {
	sq, members, err := h.svc.GetSquad(r.Context(), chi.URLParam(r, "squadID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	dto := toSquadDTO(sq)
	dto.Members = toMemberDTOs(members)
	httpapi.JSON(w, http.StatusOK, map[string]any{"squad": dto})
}

func (h *Handler) updateSquad(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	sq, err := h.svc.UpdateSquad(r.Context(), chi.URLParam(r, "squadID"), UpdateInput{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"squad": toSquadDTO(sq)})
}

func (h *Handler) removeSquad(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteSquad(r.Context(), chi.URLParam(r, "squadID")); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) setLeader(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	sq, err := h.svc.SetLeader(r.Context(), chi.URLParam(r, "squadID"), req.UserID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"squad": toSquadDTO(sq)})
}

func (h *Handler) addMember(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	if err := h.svc.AddMember(r.Context(), chi.URLParam(r, "squadID"), req.UserID); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.RemoveMember(r.Context(), chi.URLParam(r, "squadID"), chi.URLParam(r, "userID")); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
