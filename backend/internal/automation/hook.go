package automation

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"specpowers/backend/internal/httpapi"
	"specpowers/backend/internal/store"
)

// maxEventBody bounds an inbound event body.
const maxEventBody = 1 << 20

// signatureHeader carries the hex-encoded HMAC-SHA256 of the request body,
// prefixed with "sha256=".
const signatureHeader = "X-SP-Signature"

// HookRoutes returns the unauthenticated inbound endpoint: POST
// /{webhookID} with a valid signature fires every enabled autopilot bound
// to the webhook. Authentication is the signature itself, so this bundle
// must be mounted without auth.RequireAuth.
func (h *Handler) HookRoutes() http.Handler {
	r := chi.NewRouter()
	r.Post("/{webhookID}", h.receiveEvent)
	return r
}

// verifySignature reports whether sig matches the HMAC-SHA256 of body keyed
// with secret. Comparison is constant-time.
func verifySignature(secret string, body []byte, sig string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(sig, prefix) {
		return false
	}
	given, err := hex.DecodeString(strings.TrimPrefix(sig, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), given)
}

// sign returns the signature header value for body keyed with secret.
func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func (h *Handler) receiveEvent(w http.ResponseWriter, r *http.Request) {
	webhookID := chi.URLParam(r, "webhookID")
	hook, err := h.svc.GetWebhook(r.Context(), webhookID)
	if err != nil {
		if isNotFound(err) {
			httpapi.Error(w, httpapi.ErrNotFound("webhook not found"))
		} else {
			httpapi.Error(w, httpapi.ErrInternal("get webhook failed"))
		}
		return
	}
	if !hook.Enabled {
		httpapi.Error(w, httpapi.ErrForbidden("webhook is disabled"))
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxEventBody))
	if err != nil {
		httpapi.Error(w, httpapi.ErrInvalid("event body too large or unreadable"))
		return
	}
	if !verifySignature(hook.Secret, body, r.Header.Get(signatureHeader)) {
		httpapi.Error(w, httpapi.ErrUnauthorized("invalid signature"))
		return
	}
	var payload EventPayload
	if len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			httpapi.Error(w, httpapi.ErrInvalid("event body must be JSON"))
			return
		}
	}
	fired, err := h.svc.fireWebhook(r.Context(), hook.ID, payload)
	if err != nil {
		httpapi.Error(w, httpapi.ErrInternal("fire autopilots failed"))
		return
	}
	httpapi.JSON(w, http.StatusAccepted, map[string]any{"fired": fired})
}

// isNotFound reports whether err is the store's not-found sentinel or an
// httpapi 404 (wrapped chains included).
func isNotFound(err error) bool {
	var appErr *httpapi.AppError
	if errors.As(err, &appErr) && appErr.Status == http.StatusNotFound {
		return true
	}
	return errors.Is(err, store.ErrNotFound)
}
