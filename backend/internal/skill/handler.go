package skill

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/auth"
	"specpowers/backend/internal/httpapi"
)

// Handler serves the skill registry over HTTP so the agent runtime (the sp
// CLI) can list and load skills.
type Handler struct {
	reg    *Registry
	tokens *auth.TokenService
}

func NewHandler(reg *Registry, tokens *auth.TokenService) *Handler {
	return &Handler{reg: reg, tokens: tokens}
}

// Routes mounts the authenticated /skills endpoints.
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(auth.RequireAuth(h.tokens))
	r.Get("/", h.list)
	r.Get("/{key}", h.get)
	return r
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	httpapi.JSON(w, http.StatusOK, map[string]any{"skills": h.reg.List()})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	s, ok := h.reg.Get(chi.URLParam(r, "key"))
	if !ok {
		httpapi.Error(w, httpapi.ErrNotFound("skill not found"))
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"skill": s})
}
