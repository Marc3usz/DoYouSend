package iam

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"path"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/platform/httpx"
)

// SessionCookie is the name of the cookie holding the session token.
const SessionCookie = "dys_session"

type ctxKey struct{}

// WithUser returns ctx carrying the logged-in user.
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// UserFrom returns the logged-in user of a request that passed the middleware.
func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}

// Middleware enforces RouteAccess on every request before it reaches next and
// puts the logged-in user in the request context. It is the only place access
// is decided; handlers read the user with UserFrom.
func Middleware(svc *Service, logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Decide on the cleaned path, so "/api/x/../users" cannot slip past
		// the rule for "/api/users". The mux cleans it the same way.
		access := RouteAccess(r.Method, path.Clean("/"+r.URL.Path))
		if access == AccessPublic {
			next.ServeHTTP(w, r)
			return
		}

		user, err := svc.Authenticate(r.Context(), sessionToken(r))
		switch {
		case errors.Is(err, ErrUnauthenticated):
			httpx.Error(w, http.StatusUnauthorized, httpx.ErrorBody{Code: "unauthenticated", Message: ErrUnauthenticated.Error()})
			return
		case err != nil:
			logger.Error("authenticate request", "path", r.URL.Path, "err", err)
			httpx.Error(w, http.StatusInternalServerError, httpx.ErrorBody{Code: "internal", Message: "internal error"})
			return
		}
		if access == AccessAdmin && !user.IsAdmin() {
			httpx.Error(w, http.StatusForbidden, httpx.ErrorBody{Code: "forbidden", Message: ErrForbidden.Error()})
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), user)))
	})
}

func sessionToken(r *http.Request) string {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// CookieOptions are the attributes of the session cookie. Secure must be on
// everywhere but plain-HTTP local development (SESSION_COOKIE_SECURE).
type CookieOptions struct {
	Secure bool
}

func (o CookieOptions) session(token string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   o.Secure,
		// Lax keeps the cookie off cross-site POSTs, which is the CSRF
		// protection of this API (ADR-0010).
		SameSite: http.SameSiteLaxMode,
	}
}

func (o CookieOptions) cleared() *http.Cookie {
	return &http.Cookie{
		Name: SessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: o.Secure, SameSite: http.SameSiteLaxMode,
	}
}
