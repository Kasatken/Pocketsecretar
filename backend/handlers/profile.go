package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"pocketsecret/middleware"
)

const avatarPublicPrefix = "/api/static/avatars/"

type patchProfileBody struct {
	DisplayName          *string `json:"display_name"`
	Bio                  *string `json:"bio"`
	PersonalizationNote  *string `json:"personalization_note"`
	Chronotype           *string `json:"chronotype"`
}

type redeemPromoBody struct {
	Code string `json:"code"`
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.meGet(w, r)
	case http.MethodPatch:
		h.patchProfile(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// RedeemPromo POST /api/auth/me/promo — при верном коде из PRO_PROMO_CODE выставляет plan=pro (без лимита умных задач в календаре).
func (h *AuthHandler) RedeemPromo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.promoLimiter.Allow(clientIP(r)) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	expected := strings.TrimSpace(h.cfg.ProPromoCode)
	if expected == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"detail": "активация промокода не настроена на сервере",
		})
		return
	}

	user, err := h.users.FindByID(r.Context(), userID)
	if err != nil {
		http.Error(w, "failed to fetch user", http.StatusInternalServerError)
		return
	}
	if user == nil {
		http.Error(w, "user not found", http.StatusUnauthorized)
		return
	}
	if isProPlan(user.Plan) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":          true,
			"already_pro": true,
			"user":        h.userJSONForResponse(r.Context(), userID, user),
		})
		return
	}

	var body redeemPromoBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "некорректный JSON"})
		return
	}
	code := strings.TrimSpace(body.Code)
	if code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "укажите поле code"})
		return
	}
	if code != expected {
		writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "неверный промокод"})
		return
	}

	if err := h.users.SetPlan(r.Context(), userID, "pro"); err != nil {
		http.Error(w, "failed to update plan", http.StatusInternalServerError)
		return
	}
	updated, err := h.users.FindByID(r.Context(), userID)
	if err != nil || updated == nil {
		http.Error(w, "failed to fetch user", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":   true,
		"user": h.userJSONForResponse(r.Context(), userID, updated),
	})
}

func (h *AuthHandler) meGet(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	user, err := h.users.FindByID(r.Context(), userID)
	if err != nil {
		http.Error(w, "failed to fetch user", http.StatusInternalServerError)
		return
	}
	if user == nil {
		http.Error(w, "user not found", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": h.userJSONForResponse(r.Context(), userID, user),
	})
}

// MeXpRank GET /api/auth/me/xp-rank — место пользователя в общем рейтинге по experience_points.
func (h *AuthHandler) MeXpRank(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	rank, total, err := h.users.ExperienceLeaderboardPosition(r.Context(), userID)
	if err != nil {
		http.Error(w, "failed to compute rank", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"rank":        rank,
		"total_users": total,
	})
}

func (h *AuthHandler) patchProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	user, err := h.users.FindByID(r.Context(), userID)
	if err != nil {
		http.Error(w, "failed to fetch user", http.StatusInternalServerError)
		return
	}
	if user == nil {
		http.Error(w, "user not found", http.StatusUnauthorized)
		return
	}

	var body patchProfileBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if body.DisplayName == nil && body.Bio == nil && body.PersonalizationNote == nil && body.Chronotype == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"user": h.userJSONForResponse(r.Context(), userID, user),
		})
		return
	}

	if body.DisplayName != nil {
		if err := validateDisplayName(*body.DisplayName); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		user.DisplayName = strings.TrimSpace(*body.DisplayName)
	}
	if body.Bio != nil {
		if err := validateOptionalText("bio", *body.Bio, 2000); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		user.Bio = strings.TrimSpace(*body.Bio)
	}
	if body.PersonalizationNote != nil {
		if err := validateOptionalText("personalization_note", *body.PersonalizationNote, 280); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		user.PersonalizationNote = strings.TrimSpace(*body.PersonalizationNote)
	}
	if body.Chronotype != nil {
		c := strings.TrimSpace(strings.ToLower(*body.Chronotype))
		if !ValidChronotype(c) {
			http.Error(w, "invalid chronotype", http.StatusBadRequest)
			return
		}
		user.Chronotype = c
	}

	if err := h.users.UpdateProfile(r.Context(), user); err != nil {
		http.Error(w, "failed to update profile", http.StatusInternalServerError)
		return
	}
	updated, err := h.users.FindByID(r.Context(), userID)
	if err != nil || updated == nil {
		http.Error(w, "failed to fetch user", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": h.userJSONForResponse(r.Context(), userID, updated),
	})
}

func validateOptionalText(field, s string, max int) error {
	n := utf8.RuneCountInString(s)
	if n > max {
		return &validationError{msg: fmt.Sprintf("%s must be at most %d characters", field, max)}
	}
	return nil
}

func extForImage(ct string) string {
	switch ct {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}

func removeOldAvatarFile(uploadDir, oldURL string, userID int64) {
	if oldURL == "" || !strings.HasPrefix(oldURL, avatarPublicPrefix) {
		return
	}
	base := filepath.Base(oldURL)
	parts := strings.SplitN(strings.TrimSuffix(base, filepath.Ext(base)), "_", 2)
	if len(parts) != 2 {
		return
	}
	var uid int64
	if _, err := fmt.Sscanf(parts[0], "%d", &uid); err != nil || uid != userID {
		return
	}
	full := filepath.Join(uploadDir, "avatars", base)
	_ = os.Remove(full)
}

// UploadAvatar принимает multipart поле "avatar" (jpeg/png/webp).
func (h *AuthHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.AvatarMaxBytes+(512<<10))
	if err := r.ParseMultipartForm(h.cfg.AvatarMaxBytes + (256 << 10)); err != nil {
		http.Error(w, "invalid multipart", http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("avatar")
	if err != nil {
		http.Error(w, "missing avatar file", http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, h.cfg.AvatarMaxBytes+1))
	if err != nil {
		http.Error(w, "failed to read file", http.StatusBadRequest)
		return
	}
	if int64(len(data)) > h.cfg.AvatarMaxBytes {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}

	ct := http.DetectContentType(data)
	ext := extForImage(ct)
	if ext == "" {
		http.Error(w, "unsupported image type (use JPEG, PNG, or WebP)", http.StatusBadRequest)
		return
	}

	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		http.Error(w, "failed to generate name", http.StatusInternalServerError)
		return
	}
	name := fmt.Sprintf("%d_%s%s", userID, hex.EncodeToString(buf), ext)

	dir := filepath.Join(h.cfg.UploadDir, "avatars")
	if err := os.MkdirAll(dir, 0755); err != nil {
		http.Error(w, "failed to prepare upload dir", http.StatusInternalServerError)
		return
	}
	fullPath := filepath.Join(dir, name)
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		http.Error(w, "failed to save file", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	user, err := h.users.FindByID(ctx, userID)
	if err != nil || user == nil {
		_ = os.Remove(fullPath)
		http.Error(w, "failed to fetch user", http.StatusInternalServerError)
		return
	}
	oldURL := user.AvatarURL
	publicURL := avatarPublicPrefix + name

	if err := h.users.SetAvatarURL(ctx, userID, publicURL); err != nil {
		_ = os.Remove(fullPath)
		http.Error(w, "failed to update profile", http.StatusInternalServerError)
		return
	}

	removeOldAvatarFile(h.cfg.UploadDir, oldURL, userID)

	updated, err := h.users.FindByID(ctx, userID)
	if err != nil || updated == nil {
		http.Error(w, "failed to fetch user", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": h.userJSONForResponse(r.Context(), userID, updated),
	})
}
