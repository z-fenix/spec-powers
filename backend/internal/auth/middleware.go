package auth

import (
	"context"
	"net/http"
	"strings"

	"specpowers/backend/internal/httpapi"
)

type ctxKey int

const userIDKey ctxKey = 1

// RequireAuth validates the Bearer token and injects the user id into the
// request context. Requests without a valid token get a 401 envelope.
func RequireAuth(tokens *TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			token, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || token == "" {
				httpapi.Error(w, httpapi.ErrUnauthorized("missing bearer token"))
				return
			}
			userID, err := tokens.Verify(token)
			if err != nil {
				httpapi.Error(w, httpapi.ErrUnauthorized("invalid or expired token"))
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey, userID)))
		})
	}
}

func UserIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(userIDKey).(string)
	return id
}
