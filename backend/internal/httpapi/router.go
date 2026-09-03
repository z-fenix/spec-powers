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

func ErrInvalid(msg string) *AppError    { return &AppError{Status: 400, Code: "invalid_request", Message: msg} }
func ErrUnauthorized(msg string) *AppError { return &AppError{Status: 401, Code: "unauthorized", Message: msg} }
func ErrForbidden(msg string) *AppError  { return &AppError{Status: 403, Code: "forbidden", Message: msg} }
func ErrNotFound(msg string) *AppError   { return &AppError{Status: 404, Code: "not_found", Message: msg} }
func ErrConflict(msg string) *AppError   { return &AppError{Status: 409, Code: "conflict", Message: msg} }
func ErrInternal(msg string) *AppError   { return &AppError{Status: 500, Code: "internal", Message: msg} }

// Deps carries the handler bundles wired into the router. Nil bundles are
// skipped so the skeleton stays testable before auth/project handlers land.
type Deps struct {
	Auth    http.Handler
	Project http.Handler
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
			r.Mount("/auth", http.StripPrefix("/api/v1/auth", deps.Auth))
			r.Get("/me", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				deps.Auth.ServeHTTP(w, r)
			}))
		}
		if deps.Project != nil {
			r.Mount("/projects", http.StripPrefix("/api/v1/projects", deps.Project))
		}
		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			Error(w, ErrNotFound("route not found"))
		})
	})

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
