package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"pocketsecret/config"
	"pocketsecret/middleware"
	"pocketsecret/repository"
)

const calendarMaxResults = 2500

const xpPerCalendarEventComplete int64 = 25

type CalendarEventsHandler struct {
	cfg         config.Config
	repo        *repository.GoogleCalendarRepository
	users       *repository.UserRepository
	completions *repository.CalendarCompletionRepository
}

func NewCalendarEventsHandler(
	cfg config.Config,
	repo *repository.GoogleCalendarRepository,
	users *repository.UserRepository,
	completions *repository.CalendarCompletionRepository,
) *CalendarEventsHandler {
	return &CalendarEventsHandler{cfg: cfg, repo: repo, users: users, completions: completions}
}

func (h *CalendarEventsHandler) googleConfigured() bool {
	return strings.TrimSpace(h.cfg.GoogleClientID) != "" &&
		strings.TrimSpace(h.cfg.GoogleClientSecret) != "" &&
		strings.TrimSpace(h.cfg.GoogleRedirectURI) != ""
}

// ListEvents GET /api/calendar/events?timeMin=RFC3339&timeMax=RFC3339
func (h *CalendarEventsHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	timeMinStr := strings.TrimSpace(q.Get("timeMin"))
	timeMaxStr := strings.TrimSpace(q.Get("timeMax"))
	if timeMinStr == "" || timeMaxStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "timeMin and timeMax are required (RFC3339)"})
		return
	}
	timeMin, err := time.Parse(time.RFC3339, timeMinStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid timeMin"})
		return
	}
	timeMax, err := time.Parse(time.RFC3339, timeMaxStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid timeMax"})
		return
	}
	if !timeMax.After(timeMin) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "timeMax must be after timeMin"})
		return
	}

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if !h.googleConfigured() {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"linked": false,
			"events": []interface{}{},
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	refresh, err := h.repo.RefreshTokenByUserID(ctx, userID)
	if err != nil {
		log.Printf("calendar refresh token: %v", err)
		http.Error(w, "failed to load calendar link", http.StatusInternalServerError)
		return
	}
	if refresh == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"linked": false,
			"events": []interface{}{},
		})
		return
	}

	oauthCfg := newGoogleOAuthConfig(h.cfg)
	ts := oauthCfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refresh})
	httpClient := oauth2.NewClient(ctx, ts)

	raw, err := fetchGoogleCalendarEvents(ctx, httpClient, timeMin, timeMax)
	if err != nil {
		log.Printf("google calendar list: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to load events from Google Calendar"})
		return
	}

	events := make([]map[string]interface{}, 0, len(raw.Items))
	for i := range raw.Items {
		out, ok := mapGcalEvent(&raw.Items[i])
		if ok {
			events = append(events, out)
		}
	}

	ids := make([]string, 0, len(events))
	for _, ev := range events {
		id, _ := ev["id"].(string)
		if id != "" {
			ids = append(ids, id)
		}
	}
	completedSet, err := h.completions.CompletedEventIDs(ctx, userID, ids)
	if err != nil {
		log.Printf("calendar completions lookup: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load completion state"})
		return
	}
	for _, ev := range events {
		id, _ := ev["id"].(string)
		ev["completed"] = completedSet[id]
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"linked": true,
		"events": events,
	})
}

// DeleteEvent DELETE /api/calendar/events/{id}
func (h *CalendarEventsHandler) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	eventID := strings.TrimSpace(r.PathValue("id"))
	if eventID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event id required"})
		return
	}

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if !h.googleConfigured() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Google Calendar is not configured"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	refresh, err := h.repo.RefreshTokenByUserID(ctx, userID)
	if err != nil {
		log.Printf("calendar refresh token: %v", err)
		http.Error(w, "failed to load calendar link", http.StatusInternalServerError)
		return
	}
	if refresh == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Google Calendar is not linked"})
		return
	}

	oauthCfg := newGoogleOAuthConfig(h.cfg)
	ts := oauthCfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refresh})
	httpClient := oauth2.NewClient(ctx, ts)

	st, err := deleteGoogleCalendarEvent(ctx, httpClient, eventID)
	if err != nil {
		log.Printf("google calendar delete: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to delete event"})
		return
	}
	switch st {
	case http.StatusNoContent, http.StatusOK:
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.StatusNotFound, http.StatusGone:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "event not found"})
	default:
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to delete event"})
	}
}

// CompleteEvent POST /api/calendar/events/{id}/complete
func (h *CalendarEventsHandler) CompleteEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	eventID := strings.TrimSpace(r.PathValue("id"))
	if eventID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event id required"})
		return
	}

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if !h.googleConfigured() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Google Calendar is not configured"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	refresh, err := h.repo.RefreshTokenByUserID(ctx, userID)
	if err != nil {
		log.Printf("calendar refresh token: %v", err)
		http.Error(w, "failed to load calendar link", http.StatusInternalServerError)
		return
	}
	if refresh == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Google Calendar is not linked"})
		return
	}

	oauthCfg := newGoogleOAuthConfig(h.cfg)
	ts := oauthCfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refresh})
	httpClient := oauth2.NewClient(ctx, ts)

	st, err := getGoogleCalendarEvent(ctx, httpClient, eventID)
	if err != nil {
		log.Printf("google calendar get event: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to verify event"})
		return
	}
	switch st {
	case http.StatusOK:
		break
	case http.StatusNotFound, http.StatusGone:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "event not found"})
		return
	default:
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to verify event"})
		return
	}

	tx, err := h.completions.BeginTx(ctx)
	if err != nil {
		log.Printf("calendar complete tx: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save completion"})
		return
	}
	defer func() { _ = tx.Rollback() }()

	newDone, err := h.completions.TryInsertCompletion(ctx, tx, userID, eventID)
	if err != nil {
		log.Printf("calendar complete insert: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save completion"})
		return
	}

	if newDone {
		if err := h.users.AddExperienceInTx(ctx, tx, userID, xpPerCalendarEventComplete); err != nil {
			log.Printf("calendar complete xp: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update experience"})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("calendar complete commit: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save completion"})
		return
	}

	u, err := h.users.FindByID(ctx, userID)
	if err != nil || u == nil {
		log.Printf("calendar complete load user: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load user"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":                true,
		"already_completed": !newDone,
		"user":              publicUserJSON(u),
	})
}

type updateEventPayload struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// UpdateEvent PATCH /api/calendar/events/{id} — переносит событие (меняет только время начала/конца).
func (h *CalendarEventsHandler) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	eventID := strings.TrimSpace(r.PathValue("id"))
	if eventID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event id required"})
		return
	}

	var body updateEventPayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	start, err := time.Parse(time.RFC3339, strings.TrimSpace(body.Start))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid start"})
		return
	}
	end, err := time.Parse(time.RFC3339, strings.TrimSpace(body.End))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid end"})
		return
	}
	if !end.After(start) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "end must be after start"})
		return
	}

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if !h.googleConfigured() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Google Calendar is not configured"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	refresh, err := h.repo.RefreshTokenByUserID(ctx, userID)
	if err != nil {
		log.Printf("calendar refresh token: %v", err)
		http.Error(w, "failed to load calendar link", http.StatusInternalServerError)
		return
	}
	if refresh == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Google Calendar is not linked"})
		return
	}

	oauthCfg := newGoogleOAuthConfig(h.cfg)
	ts := oauthCfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refresh})
	httpClient := oauth2.NewClient(ctx, ts)

	st, err := patchGoogleCalendarEventTime(ctx, httpClient, eventID, start, end, defaultBacklogEventTZ)
	if err != nil {
		log.Printf("google calendar patch: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to reschedule event"})
		return
	}
	switch st {
	case http.StatusOK:
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.StatusNotFound, http.StatusGone:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "event not found"})
	default:
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to reschedule event"})
	}
}

// patchGoogleCalendarEventTime меняет только start/end события (wall clock в timeZone), остальные поля Google не трогает.
func patchGoogleCalendarEventTime(ctx context.Context, httpClient *http.Client, eventID string, start, end time.Time, timeZone string) (int, error) {
	loc, err := time.LoadLocation(timeZone)
	if err != nil {
		loc = time.UTC
	}
	s := start.In(loc)
	e := end.In(loc)
	patch := map[string]interface{}{
		"start": map[string]string{"dateTime": s.Format("2006-01-02T15:04:05"), "timeZone": timeZone},
		"end":   map[string]string{"dateTime": e.Format("2006-01-02T15:04:05"), "timeZone": timeZone},
	}
	raw, err := json.Marshal(patch)
	if err != nil {
		return 0, err
	}
	u := "https://www.googleapis.com/calendar/v3/calendars/primary/events/" + url.PathEscape(eventID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, u, bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func getGoogleCalendarEvent(ctx context.Context, httpClient *http.Client, eventID string) (int, error) {
	u := "https://www.googleapis.com/calendar/v3/calendars/primary/events/" + url.PathEscape(eventID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func deleteGoogleCalendarEvent(ctx context.Context, httpClient *http.Client, eventID string) (int, error) {
	u := "https://www.googleapis.com/calendar/v3/calendars/primary/events/" + url.PathEscape(eventID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return 0, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

type gcalEventsResponse struct {
	Items []gcalEventRaw `json:"items"`
}

type gcalExtPropsRaw struct {
	Private map[string]string `json:"private"`
}

type gcalEventRaw struct {
	ID                 string             `json:"id"`
	Summary            string             `json:"summary"`
	Start              gcalWhenRaw        `json:"start"`
	End                gcalWhenRaw        `json:"end"`
	ColorID            string             `json:"colorId"`
	ExtendedProperties *gcalExtPropsRaw `json:"extendedProperties,omitempty"`
}

type gcalWhenRaw struct {
	Date     string `json:"date"`
	DateTime string `json:"dateTime"`
}

func fetchGoogleCalendarEvents(ctx context.Context, httpClient *http.Client, timeMin, timeMax time.Time) (*gcalEventsResponse, error) {
	v := url.Values{}
	v.Set("singleEvents", "true")
	v.Set("orderBy", "startTime")
	v.Set("maxResults", strconv.Itoa(calendarMaxResults))
	v.Set("timeMin", timeMin.UTC().Format(time.RFC3339))
	v.Set("timeMax", timeMax.UTC().Format(time.RFC3339))

	u := "https://www.googleapis.com/calendar/v3/calendars/primary/events?" + v.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google calendar %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out gcalEventsResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func mapGcalEvent(e *gcalEventRaw) (map[string]interface{}, bool) {
	if e == nil || e.ID == "" {
		return nil, false
	}
	title := strings.TrimSpace(e.Summary)
	if title == "" {
		title = "Без названия"
	}
	out := map[string]interface{}{
		"id":    e.ID,
		"title": title,
	}

	switch {
	case e.Start.Date != "":
		startDay, err1 := time.Parse("2006-01-02", e.Start.Date)
		if err1 != nil {
			return nil, false
		}
		endDay := startDay.Add(24 * time.Hour)
		if e.End.Date != "" {
			endParsed, err2 := time.Parse("2006-01-02", e.End.Date)
			if err2 == nil {
				endDay = endParsed
			}
		}
		out["all_day"] = true
		out["start"] = startDay.UTC().Format(time.RFC3339)
		out["end"] = endDay.UTC().Format(time.RFC3339)

	default:
		if e.Start.DateTime == "" {
			return nil, false
		}
		st, err := time.Parse(time.RFC3339, e.Start.DateTime)
		if err != nil {
			return nil, false
		}
		var en time.Time
		if e.End.DateTime != "" {
			en, err = time.Parse(time.RFC3339, e.End.DateTime)
			if err != nil {
				return nil, false
			}
		} else {
			en = st.Add(time.Hour)
		}
		out["all_day"] = false
		out["start"] = st.UTC().Format(time.RFC3339)
		out["end"] = en.UTC().Format(time.RFC3339)
	}

	if e.ColorID != "" {
		out["color_id"] = e.ColorID
	}
	isBreak := false
	if e.ExtendedProperties != nil && e.ExtendedProperties.Private != nil {
		priv := e.ExtendedProperties.Private
		if pid := strings.TrimSpace(priv["pocketsecret_proposal_id"]); pid != "" {
			out["space_proposal_id"] = pid
		}
		if strings.TrimSpace(priv["pocketsecret_kind"]) == "break" {
			isBreak = true
		}
	}
	if !isBreak && (strings.HasPrefix(title, "☕ ") || strings.HasPrefix(title, "☕️")) {
		isBreak = true
	}
	if isBreak {
		out["is_break"] = true
	}
	return out, true
}

// createGoogleCalendarAllDayEvent creates an all-day event on primary calendar. dateISO is YYYY-MM-DD (local calendar date).
func createGoogleCalendarAllDayEvent(ctx context.Context, httpClient *http.Client, summary, dateISO, colorID, description string) (string, error) {
	startDay, err := time.Parse("2006-01-02", dateISO)
	if err != nil {
		return "", fmt.Errorf("invalid date")
	}
	endExclusive := startDay.AddDate(0, 0, 1)
	ev := map[string]interface{}{
		"summary": strings.TrimSpace(summary),
		"start":   map[string]string{"date": startDay.Format("2006-01-02")},
		"end":     map[string]string{"date": endExclusive.Format("2006-01-02")},
	}
	desc := strings.TrimSpace(description)
	if desc != "" {
		ev["description"] = desc
	}
	c := strings.TrimSpace(colorID)
	if c != "" {
		ev["colorId"] = c
	}
	body, err := json.Marshal(ev)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://www.googleapis.com/calendar/v3/calendars/primary/events",
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := string(rb)
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return "", fmt.Errorf("google calendar %d: %s", resp.StatusCode, msg)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rb, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

const defaultBacklogEventTZ = "Europe/Moscow"

// createGoogleCalendarTimedEvent creates a timed event (wall clock in timeZone). startHM/endHM are "HH:MM". If endHM is empty, end is start + 1 hour.
func createGoogleCalendarTimedEvent(ctx context.Context, httpClient *http.Client, summary, dateISO, startHM, endHM, colorID, description, timeZone string) (string, error) {
	if strings.TrimSpace(timeZone) == "" {
		timeZone = defaultBacklogEventTZ
	}
	loc, err := time.LoadLocation(timeZone)
	if err != nil {
		return "", fmt.Errorf("invalid timezone")
	}
	startDay, err := time.ParseInLocation("2006-01-02", dateISO, loc)
	if err != nil {
		return "", fmt.Errorf("invalid date")
	}
	var sh, sm, eh, em int
	if _, err := fmt.Sscanf(strings.TrimSpace(startHM), "%d:%d", &sh, &sm); err != nil {
		return "", fmt.Errorf("invalid start_time")
	}
	if sh < 0 || sh > 23 || sm < 0 || sm > 59 {
		return "", fmt.Errorf("invalid start_time")
	}
	start := time.Date(startDay.Year(), startDay.Month(), startDay.Day(), sh, sm, 0, 0, loc)
	var end time.Time
	if strings.TrimSpace(endHM) != "" {
		if _, err := fmt.Sscanf(strings.TrimSpace(endHM), "%d:%d", &eh, &em); err != nil {
			return "", fmt.Errorf("invalid end_time")
		}
		if eh < 0 || eh > 23 || em < 0 || em > 59 {
			return "", fmt.Errorf("invalid end_time")
		}
		end = time.Date(startDay.Year(), startDay.Month(), startDay.Day(), eh, em, 0, 0, loc)
		if !end.After(start) {
			return "", fmt.Errorf("end_time must be after start_time")
		}
	} else {
		end = start.Add(time.Hour)
	}

	ev := map[string]interface{}{
		"summary": strings.TrimSpace(summary),
		"start": map[string]string{
			"dateTime": start.Format("2006-01-02T15:04:05"),
			"timeZone": timeZone,
		},
		"end": map[string]string{
			"dateTime": end.Format("2006-01-02T15:04:05"),
			"timeZone": timeZone,
		},
	}
	desc := strings.TrimSpace(description)
	if desc != "" {
		ev["description"] = desc
	}
	c := strings.TrimSpace(colorID)
	if c != "" {
		ev["colorId"] = c
	}
	body, err := json.Marshal(ev)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://www.googleapis.com/calendar/v3/calendars/primary/events",
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := string(rb)
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return "", fmt.Errorf("google calendar %d: %s", resp.StatusCode, msg)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rb, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

// createGoogleCalendarInstantRangeEvent creates a timed event from absolute instants using the given IANA timezone for Google's dateTime fields.
// proposalID > 0 — записывается в private extendedProperties для связи с предложением в приложении.
func createGoogleCalendarInstantRangeEvent(ctx context.Context, httpClient *http.Client, summary, description, timeZone string, start, end time.Time, proposalID int64) (string, error) {
	if strings.TrimSpace(timeZone) == "" {
		timeZone = defaultBacklogEventTZ
	}
	loc, err := time.LoadLocation(timeZone)
	if err != nil {
		return "", fmt.Errorf("invalid timezone")
	}
	if !end.After(start) {
		return "", fmt.Errorf("end must be after start")
	}
	s := start.In(loc)
	e := end.In(loc)
	ev := map[string]interface{}{
		"summary": strings.TrimSpace(summary),
		"start": map[string]string{
			"dateTime": s.Format("2006-01-02T15:04:05"),
			"timeZone": timeZone,
		},
		"end": map[string]string{
			"dateTime": e.Format("2006-01-02T15:04:05"),
			"timeZone": timeZone,
		},
	}
	desc := strings.TrimSpace(description)
	if desc != "" {
		ev["description"] = desc
	}
	if proposalID > 0 {
		ev["extendedProperties"] = map[string]interface{}{
			"private": map[string]string{
				"pocketsecret_proposal_id": strconv.FormatInt(proposalID, 10),
			},
		}
	}
	body, err := json.Marshal(ev)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://www.googleapis.com/calendar/v3/calendars/primary/events",
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := string(rb)
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return "", fmt.Errorf("google calendar %d: %s", resp.StatusCode, msg)
	}
	var createdInstant struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rb, &createdInstant); err != nil {
		return "", err
	}
	return createdInstant.ID, nil
}
