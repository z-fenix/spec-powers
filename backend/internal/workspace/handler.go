package workspace

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/domain"
	"specpowers/backend/internal/httpapi"
)

type Handler struct {
	svc    *Service
	tokens *auth.TokenService
}

func NewHandler(svc *Service, tokens *auth.TokenService) *Handler {
	return &Handler{svc: svc, tokens: tokens}
}

const timeFormat = time.RFC3339

type memberDTO struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	JoinedAt    string `json:"joined_at"`
}

func toMemberDTO(info MemberInfo) memberDTO {
	return memberDTO{
		UserID:      info.Member.UserID,
		Email:       info.User.Email,
		DisplayName: info.User.DisplayName,
		Role:        info.Role,
		JoinedAt:    info.Member.CreatedAt.UTC().Format(timeFormat),
	}
}

type inviteDTO struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	Email       string  `json:"email"`
	Role        string  `json:"role"`
	Code        string  `json:"code"`
	Status      string  `json:"status"`
	InvitedBy   string  `json:"invited_by"`
	CreatedAt   string  `json:"created_at"`
	AcceptedAt  *string `json:"accepted_at,omitempty"`
}

func toInviteDTO(i *domain.WorkspaceInvite) inviteDTO {
	dto := inviteDTO{
		ID:          i.ID,
		WorkspaceID: i.WorkspaceID,
		Email:       i.Email,
		Role:        RoleName(i.RoleID),
		Code:        i.Code,
		Status:      i.Status,
		InvitedBy:   i.InvitedBy,
		CreatedAt:   i.CreatedAt.UTC().Format(timeFormat),
	}
	if i.AcceptedAt != nil {
		accepted := i.AcceptedAt.UTC().Format(timeFormat)
		dto.AcceptedAt = &accepted
	}
	return dto
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(auth.RequireAuth(h.tokens))
	r.Get("/members", h.listMembers)
	r.Post("/members/invite", h.invite)
	r.Patch("/members/{userID}", h.setRole)
	r.Get("/invites", h.listInvites)
	r.Post("/invites/redeem", h.redeem)
	r.Delete("/invites/{inviteID}", h.revokeInvite)
	return r
}

func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) {
	ws, members, viewerRole, err := h.svc.Members(r.Context(), auth.UserIDFrom(r.Context()))
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]memberDTO, 0, len(members))
	for _, m := range members {
		dtos = append(dtos, toMemberDTO(m))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{
		"workspace":   map[string]string{"id": ws.ID, "name": ws.Name},
		"viewer_role": viewerRole,
		"members":     dtos,
	})
}

func (h *Handler) invite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	res, err := h.svc.Invite(r.Context(), auth.UserIDFrom(r.Context()), req.Email, req.Role)
	if err != nil {
		writeAppError(w, err)
		return
	}
	body := map[string]any{"joined": res.Joined}
	if res.Joined && res.Member != nil {
		body["member"] = toMemberDTO(*res.Member)
	}
	if !res.Joined && res.Invite != nil {
		dto := toInviteDTO(res.Invite)
		body["invite"] = dto
		body["code"] = dto.Code
	}
	httpapi.JSON(w, http.StatusCreated, body)
}

func (h *Handler) setRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	info, err := h.svc.SetRole(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "userID"), req.Role)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"member": toMemberDTO(*info)})
}

func (h *Handler) listInvites(w http.ResponseWriter, r *http.Request) {
	invites, err := h.svc.Invites(r.Context(), auth.UserIDFrom(r.Context()))
	if err != nil {
		writeAppError(w, err)
		return
	}
	dtos := make([]inviteDTO, 0, len(invites))
	for i := range invites {
		dtos = append(dtos, toInviteDTO(&invites[i]))
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"invites": dtos})
}

func (h *Handler) revokeInvite(w http.ResponseWriter, r *http.Request) {
	invite, err := h.svc.RevokeInvite(r.Context(), auth.UserIDFrom(r.Context()), chi.URLParam(r, "inviteID"))
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"invite": toInviteDTO(invite)})
}

func (h *Handler) redeem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("malformed JSON body"))
		return
	}
	ws, err := h.svc.Redeem(r.Context(), auth.UserIDFrom(r.Context()), req.Code)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{
		"workspace": map[string]string{"id": ws.ID, "name": ws.Name},
	})
}

func writeAppError(w http.ResponseWriter, err error) {
	if appErr, ok := errors.AsType[*httpapi.AppError](err); ok {
		httpapi.Error(w, appErr)
		return
	}
	httpapi.Error(w, httpapi.ErrInternal("internal server error"))
}
