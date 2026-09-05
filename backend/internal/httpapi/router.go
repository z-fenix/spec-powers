package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type AppError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *AppError) Error() string { return e.Code + ": " + e.Message }

func ErrInvalid(msg string) *AppError {
	return &AppError{Status: 400, Code: "invalid_request", Message: msg}
}
func ErrUnauthorized(msg string) *AppError {
	return &AppError{Status: 401, Code: "unauthorized", Message: msg}
}
func ErrForbidden(msg string) *AppError {
	return &AppError{Status: 403, Code: "forbidden", Message: msg}
}
func ErrNotFound(msg string) *AppError {
	return &AppError{Status: 404, Code: "not_found", Message: msg}
}
func ErrConflict(msg string) *AppError { return &AppError{Status: 409, Code: "conflict", Message: msg} }
func ErrInternal(msg string) *AppError { return &AppError{Status: 500, Code: "internal", Message: msg} }

// Deps carries the handler bundles wired into the router. Nil bundles are
// skipped so the skeleton stays testable before auth/project handlers land.
type Deps struct {
	Auth      http.Handler
	Project   http.Handler
	Changes   http.Handler
	Skills    http.Handler
	Agents    http.Handler
	Runs      http.Handler
	Notifs    http.Handler
	Runtime   http.Handler
	Workspace http.Handler
	Squads  http.Handler
	// Hooks is the unauthenticated inbound webhook endpoint (signature is
	// the credential); Webhooks and Autopilots are the JWT management APIs.
	Hooks      http.Handler
	Webhooks   http.Handler
	Autopilots http.Handler
	// Static serves the SPA frontend at the root when set; API routes keep
	// precedence.
	Static http.Handler
}

func NewRouter(deps Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(Recover)
	r.Use(Logger)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
			JSON(w, http.StatusOK, map[string]string{"status": "ok"})
		})
		if deps.Auth != nil {
			r.Mount("/auth", deps.Auth)
		}
		if deps.Project != nil {
			r.Mount("/projects", http.StripPrefix("/api/v1/projects", deps.Project))
		}
		if deps.Changes != nil {
			r.Mount("/changes", http.StripPrefix("/api/v1/changes", deps.Changes))
		}
		if deps.Skills != nil {
			r.Mount("/skills", http.StripPrefix("/api/v1/skills", deps.Skills))
		}
		if deps.Agents != nil {
			r.Mount("/agents", http.StripPrefix("/api/v1/agents", deps.Agents))
		}
		if deps.Runs != nil {
			r.Mount("/runs", http.StripPrefix("/api/v1/runs", deps.Runs))
		}
		if deps.Notifs != nil {
			r.Mount("/notifications", http.StripPrefix("/api/v1/notifications", deps.Notifs))
		}
		if deps.Runtime != nil {
			r.Mount("/runtime", http.StripPrefix("/api/v1/runtime", deps.Runtime))
		}
		if deps.Workspace != nil {
			r.Mount("/workspace", http.StripPrefix("/api/v1/workspace", deps.Workspace))
		}
		if deps.Squads != nil {
			r.Mount("/squads", http.StripPrefix("/api/v1/squads", deps.Squads))
		}
		if deps.Webhooks != nil {
			r.Mount("/webhooks", http.StripPrefix("/api/v1/webhooks", deps.Webhooks))
		}
		if deps.Autopilots != nil {
			r.Mount("/autopilots", http.StripPrefix("/api/v1/autopilots", deps.Autopilots))
		}
		if deps.Hooks != nil {
			r.Mount("/hooks", http.StripPrefix("/api/v1/hooks", deps.Hooks))
		}
		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			Error(w, ErrNotFound("route not found"))
		})
	})

	if deps.Static != nil {
		r.Mount("/", deps.Static)
	}

	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		Error(w, ErrNotFound("route not found"))
	})
	return r
}

func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				Error(w, ErrInternal("internal server error"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func Error(w http.ResponseWriter, appErr *AppError) {
	JSON(w, appErr.Status, map[string]any{"error": appErr})
}
