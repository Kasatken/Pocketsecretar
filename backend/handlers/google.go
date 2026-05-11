package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"pocketsecret/config"
	"pocketsecret/middleware"
	"pocketsecret/repository"
	"pocketsecret/security"
)

type GoogleOAuthHandler struct {
	cfg    config.Config
	tokens *security.TokenManager
	repo   *repository.GoogleCalendarRepository
	oauth  *oauth2.Config
}

func NewGoogleOAuthHandler(cfg config.Config, repo *repository.GoogleCalendarRepository, tm *security.TokenManager) *GoogleOAuthHandler {
	return &GoogleOAuthHandler{cfg: cfg, tokens: tm, repo: repo, oauth: newGoogleOAuthConfig(cfg)}
}

func newGoogleOAuthConfig(cfg config.Config) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		RedirectURL:  cfg.GoogleRedirectURI,
		Scopes: []string{
			"openid",
			"email",
			"https://www.googleapis.com/auth/calendar.readonly",
			"https://www.googleapis.com/auth/calendar.events",
		},
		Endpoint: google.Endpoint,
	}
}

func (h *GoogleOAuthHandler) configured() bool {
	return h.cfg.GoogleClientID != "" && h.cfg.GoogleClientSecret != "" && strings.TrimSpace(h.cfg.GoogleRedirectURI) != ""
}

// Start — пользователь уже вошёл в приложение (HttpOnly cookie refresh_token).
func (h *GoogleOAuthHandler) Start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.configured() {
		http.Error(w, "Google OAuth is not configured", http.StatusServiceUnavailable)
		return
	}
	cookie, err := r.Cookie("refresh_token")
	if err != nil || cookie.Value == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	claims, err := h.tokens.ParseRefreshToken(cookie.Value)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	state, err := h.tokens.IssueGoogleOAuthState(claims.UserID)
	if err != nil {
		http.Error(w, "failed to issue state", http.StatusInternalServerError)
		return
	}
	opts := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
	}
	authURL := h.oauth.AuthCodeURL(state, opts...)
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (h *GoogleOAuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.configured() {
		h.redirectError(w, r, "not_configured")
		return
	}
	q := r.URL.Query()
	if errMsg := q.Get("error"); errMsg != "" {
		h.redirectError(w, r, errMsg)
		return
	}
	code := q.Get("code")
	state := q.Get("state")
	if code == "" || state == "" {
		h.redirectError(w, r, "missing_code")
		return
	}
	stClaims, err := h.tokens.ParseGoogleOAuthState(state)
	if err != nil {
		h.redirectError(w, r, "bad_state")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	tok, err := h.oauth.Exchange(ctx, code)
	if err != nil {
		log.Printf("google oauth exchange: %v", err)
		h.redirectError(w, r, "exchange_failed")
		return
	}
	userInfo, err := fetchGoogleUserInfo(ctx, tok.AccessToken)
	if err != nil {
		log.Printf("google userinfo: %v", err)
		h.redirectError(w, r, "userinfo_failed")
		return
	}
	refresh := ""
	if tok.RefreshToken != "" {
		refresh = tok.RefreshToken
	}
	if err := h.repo.Upsert(ctx, stClaims.UserID, userInfo.Sub, userInfo.Email, refresh); err != nil {
		log.Printf("google upsert: %v", err)
		h.redirectError(w, r, "save_failed")
		return
	}
	h.redirectSuccess(w, r)
}

func (h *GoogleOAuthHandler) Status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.configured() {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"configured": false,
			"linked":     false,
		})
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	link, err := h.repo.GetByUserID(r.Context(), userID)
	if err != nil {
		http.Error(w, "failed to load link", http.StatusInternalServerError)
		return
	}
	if link == nil || !link.HasRefresh {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"configured": true,
			"linked":     false,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"configured":        true,
		"linked":            true,
		"google_email":      link.EmailHint,
		"has_refresh_token": link.HasRefresh,
	})
}

func (h *GoogleOAuthHandler) redirectSuccess(w http.ResponseWriter, r *http.Request) {
	target := profileRedirectURL(h.cfg.AllowedOrigin, "connected", "")
	http.Redirect(w, r, target, http.StatusFound)
}

func (h *GoogleOAuthHandler) redirectError(w http.ResponseWriter, r *http.Request, reason string) {
	target := profileRedirectURL(h.cfg.AllowedOrigin, "error", reason)
	http.Redirect(w, r, target, http.StatusFound)
}

func profileRedirectURL(origin, status, reason string) string {
	base := strings.TrimSuffix(strings.TrimSpace(origin), "/")
	if base == "" {
		base = "http://localhost:5173"
	}
	u, err := url.Parse(base)
	if err != nil {
		return "/profile?google=" + url.QueryEscape(status) + "&reason=" + url.QueryEscape(reason)
	}
	u.Path = "/profile"
	q := url.Values{}
	q.Set("google", status)
	if reason != "" {
		q.Set("reason", reason)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

type googleUserInfo struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
}

func fetchGoogleUserInfo(ctx context.Context, accessToken string) (*googleUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v3/userinfo", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("userinfo bad status")
	}
	var out googleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}
