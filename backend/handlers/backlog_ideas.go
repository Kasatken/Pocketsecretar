package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/oauth2"

	"pocketsecret/config"
	"pocketsecret/middleware"
	"pocketsecret/repository"
)

var backlogIdeaUUIDRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

var backlogTimeHMRe = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)$`)

var allowedBacklogColorIDs = map[string]struct{}{
	"1": {}, "2": {}, "3": {}, "4": {}, "5": {}, "6": {},
	"7": {}, "8": {}, "9": {}, "10": {}, "11": {},
}

type BacklogIdeasHandler struct {
	cfg     config.Config
	google  *repository.GoogleCalendarRepository
	backlog *repository.BacklogIdeaRepository
}

func NewBacklogIdeasHandler(cfg config.Config, google *repository.GoogleCalendarRepository, backlog *repository.BacklogIdeaRepository) *BacklogIdeasHandler {
	return &BacklogIdeasHandler{cfg: cfg, google: google, backlog: backlog}
}

func (h *BacklogIdeasHandler) googleConfigured() bool {
	return strings.TrimSpace(h.cfg.GoogleClientID) != "" &&
		strings.TrimSpace(h.cfg.GoogleClientSecret) != "" &&
		strings.TrimSpace(h.cfg.GoogleRedirectURI) != ""
}

func backlogIdeaToJSON(row *repository.BacklogIdea) map[string]interface{} {
	return map[string]interface{}{
		"id":         row.ID,
		"body":       row.Body,
		"color_id":   row.ColorID,
		"tag":        row.Tag,
		"created_at": row.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func validBacklogTag(s string) bool {
	if utf8.RuneCountInString(s) > 64 {
		return false
	}
	for _, r := range s {
		switch {
		case r == ' ' || r == '-' || r == '_' || r == '.':
			continue
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			continue
		default:
			return false
		}
	}
	return true
}

func normalizeBacklogColorID(s string) string {
	c := strings.TrimSpace(s)
	if c == "" {
		return "1"
	}
	if _, ok := allowedBacklogColorIDs[c]; ok {
		return c
	}
	return ""
}

type createBacklogPayload struct {
	Body    string `json:"body"`
	ColorID string `json:"color_id"`
	Tag     string `json:"tag"`
}

type scheduleBacklogPayload struct {
	Date      string `json:"date"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

// ListIdeas GET /api/backlog/ideas
func (h *BacklogIdeasHandler) ListIdeas(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	rows, err := h.backlog.ListByUser(ctx, userID)
	if err != nil {
		log.Printf("backlog list: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load ideas"})
		return
	}
	out := make([]map[string]interface{}, 0, len(rows))
	for i := range rows {
		out = append(out, backlogIdeaToJSON(&rows[i]))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ideas": out})
}

// CreateIdea POST /api/backlog/ideas
func (h *BacklogIdeasHandler) CreateIdea(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var payload createBacklogPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	body := strings.TrimSpace(payload.Body)
	if body == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body is required"})
		return
	}
	if utf8.RuneCountInString(body) > 2000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body too long"})
		return
	}
	colorID := normalizeBacklogColorID(payload.ColorID)
	if colorID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid color_id"})
		return
	}
	tag := strings.TrimSpace(payload.Tag)
	if !validBacklogTag(tag) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tag"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	row, err := h.backlog.Create(ctx, userID, body, colorID, tag)
	if err != nil {
		if errors.Is(err, repository.ErrBacklogLimitReached) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "too many ideas — удалите старые"})
			return
		}
		log.Printf("backlog create: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"idea": backlogIdeaToJSON(row)})
}

// DeleteIdea DELETE /api/backlog/ideas/{id}
func (h *BacklogIdeasHandler) DeleteIdea(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" || !backlogIdeaUUIDRe.MatchString(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	okDel, err := h.backlog.Delete(ctx, userID, id)
	if err != nil {
		log.Printf("backlog delete: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete"})
		return
	}
	if !okDel {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ScheduleIdea POST /api/backlog/ideas/{id}/schedule
func (h *BacklogIdeasHandler) ScheduleIdea(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" || !backlogIdeaUUIDRe.MatchString(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var payload scheduleBacklogPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	dateISO := strings.TrimSpace(payload.Date)
	if _, err := time.Parse("2006-01-02", dateISO); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid date"})
		return
	}

	startHM := strings.TrimSpace(payload.StartTime)
	endHM := strings.TrimSpace(payload.EndTime)
	if startHM != "" && !backlogTimeHMRe.MatchString(startHM) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid start_time"})
		return
	}
	if endHM != "" && startHM == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "end_time requires start_time"})
		return
	}

	if !h.googleConfigured() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Google Календарь не настроен на сервере"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	idea, err := h.backlog.GetOwned(ctx, userID, id)
	if err != nil {
		log.Printf("backlog get: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load idea"})
		return
	}
	if idea == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	refresh, err := h.google.RefreshTokenByUserID(ctx, userID)
	if err != nil {
		log.Printf("calendar refresh token: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load calendar link"})
		return
	}
	if refresh == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Сначала подключите Google Календарь в профиле"})
		return
	}

	oauthCfg := newGoogleOAuthConfig(h.cfg)
	ts := oauthCfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refresh})
	httpClient := oauth2.NewClient(ctx, ts)

	desc := ""
	if strings.TrimSpace(idea.Tag) != "" {
		desc = "Тег: " + strings.TrimSpace(idea.Tag)
	}

	var eventID string
	var calErr error
	if startHM != "" {
		eventID, calErr = createGoogleCalendarTimedEvent(ctx, httpClient, idea.Body, dateISO, startHM, endHM, idea.ColorID, desc, defaultBacklogEventTZ)
	} else {
		eventID, calErr = createGoogleCalendarAllDayEvent(ctx, httpClient, idea.Body, dateISO, idea.ColorID, desc)
	}
	if calErr != nil {
		log.Printf("backlog schedule gcal: %v", calErr)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Не удалось создать событие в календаре"})
		return
	}

	okDel, err := h.backlog.Delete(ctx, userID, id)
	if err != nil {
		log.Printf("backlog schedule delete after gcal: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Событие создано, но не удалось убрать стикер — обновите страницу"})
		return
	}
	if !okDel {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":       true,
			"event_id": eventID,
			"warning":  "event created but sticker row missing",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":       true,
		"event_id": eventID,
	})
}
