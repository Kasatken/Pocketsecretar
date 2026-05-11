package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"pocketsecret/config"
	"pocketsecret/middleware"
	"pocketsecret/models"
	"pocketsecret/repository"
	"pocketsecret/security"
)

var emailRegex = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

type AuthHandler struct {
	users        *repository.UserRepository
	tokens       *security.TokenManager
	cfg          config.Config
	limiter      *middleware.RateLimiter
	promoLimiter *middleware.RateLimiter
	smartTasks   *repository.SmartTaskRepository
	completions  *repository.CalendarCompletionRepository
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type registerPayload struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

func NewAuthHandler(users *repository.UserRepository, tokens *security.TokenManager, cfg config.Config, smartTasks *repository.SmartTaskRepository, completions *repository.CalendarCompletionRepository) *AuthHandler {
	return &AuthHandler{
		users:        users,
		tokens:       tokens,
		cfg:          cfg,
		limiter:      middleware.NewRateLimiter(12, time.Minute),
		promoLimiter: middleware.NewRateLimiter(10, time.Minute),
		smartTasks:   smartTasks,
		completions:  completions,
	}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.limiter.Allow(clientIP(r)) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	var creds registerPayload
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	if err := validateCredentials(creds.Email, creds.Password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateDisplayName(creds.DisplayName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hash, err := security.HashPassword(creds.Password)
	if err != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}
	user, err := h.users.Create(r.Context(), creds.Email, hash, strings.TrimSpace(creds.DisplayName))
	if err != nil {
		if err == repository.ErrUserExists {
			http.Error(w, "email already exists", http.StatusConflict)
			return
		}
		http.Error(w, "failed to create user", http.StatusInternalServerError)
		return
	}

	h.respondWithSession(w, r, user)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.limiter.Allow(clientIP(r)) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	var creds credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	if err := validateCredentials(creds.Email, creds.Password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	user, err := h.users.FindByEmail(r.Context(), creds.Email)
	if err != nil {
		http.Error(w, "failed to fetch user", http.StatusInternalServerError)
		return
	}
	if user == nil || !security.CheckPassword(user.PasswordHash, creds.Password) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	_ = h.users.TouchLogin(r.Context(), user.ID)
	h.respondWithSession(w, r, user)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie("refresh_token")
	if err != nil || cookie.Value == "" {
		http.Error(w, "missing refresh token", http.StatusUnauthorized)
		return
	}
	claims, err := h.tokens.ParseRefreshToken(cookie.Value)
	if err != nil {
		http.Error(w, "invalid refresh token", http.StatusUnauthorized)
		return
	}

	user, err := h.users.FindByID(r.Context(), claims.UserID)
	if err != nil {
		http.Error(w, "failed to fetch user", http.StatusInternalServerError)
		return
	}
	if user == nil {
		http.Error(w, "user not found", http.StatusUnauthorized)
		return
	}

	h.respondWithSession(w, r, user)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *AuthHandler) respondWithSession(w http.ResponseWriter, r *http.Request, user *models.User) {
	accessToken, _, err := h.tokens.IssueAccessToken(user.ID)
	if err != nil {
		http.Error(w, "failed to issue access token", http.StatusInternalServerError)
		return
	}
	refreshToken, refreshExpiresAt, err := h.tokens.IssueRefreshToken(user.ID)
	if err != nil {
		http.Error(w, "failed to issue refresh token", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  refreshExpiresAt,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": accessToken,
		"user":         h.userJSONForResponse(r.Context(), user.ID, user),
	})
}

func (h *AuthHandler) userJSONForResponse(ctx context.Context, userID int64, user *models.User) map[string]interface{} {
	u := publicUserJSON(user)
	if h.smartTasks != nil && !isProPlan(user.Plan) {
		day, _ := repository.ParseQuotaDay(repository.QuotaDayUTC())
		used, err := h.smartTasks.CountForDay(ctx, userID, day)
		if err == nil {
			limit := h.cfg.FreeSmartTaskLimit
			if limit <= 0 {
				limit = 3
			}
			u["smart_tasks_today"] = map[string]interface{}{
				"used":      used,
				"limit":     limit,
				"resets_at": repository.QuotaResetsAtUTC(),
			}
		}
	}
	return u
}

func validateCredentials(email, password string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if !emailRegex.MatchString(email) {
		return &validationError{msg: "invalid email"}
	}
	if len(password) < 8 {
		return &validationError{msg: "password must contain at least 8 characters"}
	}
	return nil
}

func validateDisplayName(s string) error {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	if n < 2 {
		return &validationError{msg: "display name must be at least 2 characters"}
	}
	if n > 80 {
		return &validationError{msg: "display name must be at most 80 characters"}
	}
	return nil
}

type validationError struct {
	msg string
}

func (e *validationError) Error() string {
	return e.msg
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}
	return r.RemoteAddr
}
