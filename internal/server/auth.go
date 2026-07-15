// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"
	"errors"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

// loginRateLimit and loginRateWindow cap login attempts per client IP.
const (
	loginRateLimit  = 10
	loginRateWindow = time.Minute
)

type contextKey int

const userContextKey contextKey = 0

// withUser returns a context carrying the authenticated user's identity,
// deliberately excluding credentials such as the password hash.
func withUser(ctx context.Context, u userResponse) context.Context {
	return context.WithValue(ctx, userContextKey, u)
}

// userFromContext returns the identity stored by the requireSession
// middleware, or the zero identity outside of it.
func userFromContext(ctx context.Context) userResponse {
	u, _ := ctx.Value(userContextKey).(userResponse)
	return u
}

// sessionCookieName uses the __Host- prefix, which browsers honor only
// for cookies that are Secure, Path=/, and host-scoped (no Domain).
const sessionCookieName = "__Host-alphone_session"

// dummyPasswordHash is verified against when a login names an unknown
// user, so both outcomes cost one hash computation. It hashes a password
// too short to register, so no account can ever share it.
const dummyPasswordHash = "$argon2id$v=19$m=19456,t=2,p=1$c3Ra23u60gssamS7GUMIlA$" +
	"gw1m1IBIwi/ojF3wkAm3P07a5LSQwos4waXky7aLVWM"

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
}

// clientIPResolver returns middleware that records a request's client IP for rate limiting.
func clientIPResolver(trustedProxies []string) func(http.Handler) http.Handler {
	if len(trustedProxies) == 0 {
		return middleware.ClientIPFromRemoteAddr
	}
	return middleware.ClientIPFromXFF(trustedProxies...)
}

// keyByRemoteIP returns a request's canonical client IP for rate limiting.
func keyByRemoteIP(r *http.Request) string {
	ip := middleware.GetClientIP(r.Context())
	if ip == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip = host
	}
	return httprate.CanonicalizeIP(ip)
}

// loginRateLimiter returns middleware that limits failed login attempts per client IP.
func loginRateLimiter() func(http.Handler) http.Handler {
	limiter := httprate.NewRateLimiter(loginRateLimit, loginRateWindow,
		httprate.WithLimitCounter(httprate.NewLocalLimitCounter(loginRateWindow)),
	)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyByRemoteIP(r)
			_, rate, err := limiter.Status(key)
			if err != nil {
				respondError(w, http.StatusInternalServerError, "internal error")
				return
			}
			if int(math.Round(rate)) >= loginRateLimit {
				w.Header().Set("Retry-After", strconv.Itoa(int(loginRateWindow.Seconds())))
				respondError(w, http.StatusTooManyRequests, "too many login attempts, try again later")
				return
			}
			recorder := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(recorder, r)
			if recorder.Status() == http.StatusUnauthorized {
				window := time.Now().UTC().Truncate(loginRateWindow)
				_ = limiter.Counter().IncrementBy(key, window, 1)
			}
		})
	}
}

// handleLogin returns an HTTP handler that verifies credentials and
// issues a session cookie.
func (s *server) handleLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := decode[credentials](w, r)
		if err != nil {
			respondError(w, http.StatusBadRequest, "malformed login request")
			return
		}
		email := strings.ToLower(strings.TrimSpace(body.Email))
		u, err := s.users.UserByEmail(r.Context(), email)
		if errors.Is(err, gouncer.ErrUserNotFound) {
			gouncer.VerifyPassword(dummyPasswordHash, body.Password)
			respondError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		if err != nil {
			respondError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if !gouncer.VerifyPassword(u.PasswordHash, body.Password) || u.Disabled {
			respondError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		session, err := s.newSession(u.ID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := s.users.CreateSession(r.Context(), session); err != nil {
			respondError(w, http.StatusInternalServerError, "internal error")
			return
		}
		http.SetCookie(w, sessionCookie(session.Token, int(gouncer.DefaultSessionDuration.Seconds())))
		respond(w, http.StatusOK, userResponse{ID: u.ID, Email: u.Email, Name: u.Name})
	}
}

// handleLogout returns an HTTP handler that deletes the current session
// and clears its cookie.
func (s *server) handleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			if err := s.users.DeleteSession(r.Context(), gouncer.HashToken(cookie.Value)); err != nil {
				respondError(w, http.StatusInternalServerError, "internal error")
				return
			}
		}
		http.SetCookie(w, sessionCookie("", -1))
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleSession returns an HTTP handler reporting the logged-in user, whose
// identity the requireSession middleware already resolved.
func (s *server) handleSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, userFromContext(r.Context()))
	}
}

// sessionUser returns the user owning the request's session cookie.
func (s *server) sessionUser(r *http.Request) (gouncer.User, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return gouncer.User{}, gouncer.ErrSessionNotFound
	}
	return s.users.UserBySession(r.Context(), gouncer.HashToken(cookie.Value), time.Now().UTC())
}

// requireSession admits only requests carrying a usable session cookie,
// passing the authenticated user down through the request context.
func (s *server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := s.sessionUser(r)
		if errors.Is(err, gouncer.ErrSessionNotFound) {
			respondError(w, http.StatusUnauthorized, "no session")
			return
		}
		if err != nil {
			respondError(w, http.StatusInternalServerError, "internal error")
			return
		}
		identity := userResponse{ID: u.ID, Email: u.Email, Name: u.Name}
		next.ServeHTTP(w, r.WithContext(withUser(r.Context(), identity)))
	})
}

// protectPlugin wraps a plugin handler in the session middleware, letting
// the plugin's declared public paths through untouched.
func (s *server) protectPlugin(handler http.Handler, publicPaths []string) http.Handler {
	public := make(map[string]struct{}, len(publicPaths))
	for _, path := range publicPaths {
		public[path] = struct{}{}
	}
	protected := s.requireSession(handler)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := public[r.URL.Path]; ok {
			handler.ServeHTTP(w, r)
			return
		}
		protected.ServeHTTP(w, r)
	})
}

// sessionCookie builds the session cookie carrying token for maxAge
// seconds; a negative maxAge clears it.
func sessionCookie(token string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}
