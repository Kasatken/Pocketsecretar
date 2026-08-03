package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"pocketsecret/middleware"
	"pocketsecret/repository"
)

// Обёртки над repository.SmartTaskJobRepository для фоновой горутины:
// свой короткий контекст (не привязанный к workCtx, который к этому моменту мог истечь) и логирование ошибок записи.

func (h *SmartTaskHandler) jobMarkRunning(jobID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.jobs.MarkRunning(ctx, jobID); err != nil {
		log.Printf("smart-task job mark running: %v", err)
	}
}

func (h *SmartTaskHandler) jobFinishOK(jobID string, result map[string]interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.jobs.Finish(ctx, jobID, result); err != nil {
		log.Printf("smart-task job finish: %v", err)
	}
}

func (h *SmartTaskHandler) jobFinishErr(jobID string, httpCode int, body map[string]interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.jobs.Fail(ctx, jobID, httpCode, body); err != nil {
		log.Printf("smart-task job fail: %v", err)
	}
}

// executeSmartTaskSync выполняет OpenRouter + Google. ctx не должен быть привязан к HTTP-запросу клиента.
func (h *SmartTaskHandler) executeSmartTaskSync(
	ctx context.Context,
	userID int64,
	plan string,
	body smartTaskPayload,
	refresh string,
	userMsg string,
	initialCount int,
	limit int,
	quotaDay time.Time,
	apiKey string,
	model string,
	maxTok int,
	loc *time.Location,
	tzName string,
) (okBody map[string]interface{}, errHTTP int, errBody map[string]interface{}) {
	tLLM := time.Now()
	content, err := OpenRouterChat(ctx, h.cfg, apiKey, model, maxTok, 0.3, smartTaskSystem, userMsg)
	log.Printf("[smart-task timing] openrouter_llm=%dms", time.Since(tLLM).Milliseconds())
	if err != nil {
		log.Printf("openrouter: %v", err)
		return nil, http.StatusBadGateway, map[string]interface{}{"detail": err.Error()}
	}

	rawTasks, parseErr := parseAIJSONTasks(content)
	if parseErr != nil {
		preview := content
		if len(preview) > 480 {
			preview = preview[:480] + "…"
		}
		log.Printf("parse ai json (try 1): %v; preview: %q", parseErr, preview)
		repairMax := maxTok
		if repairMax > 4096 {
			repairMax = 4096
		}
		fixed, errRepair := OpenRouterChat(ctx, h.cfg, apiKey, model, repairMax, 0.05, smartTaskJSONRepairSystem, smartTaskJSONRepairUserContent(content))
		if errRepair != nil {
			log.Printf("openrouter json repair: %v", errRepair)
			return nil, http.StatusBadGateway, map[string]interface{}{"detail": "Некорректный ответ ИИ: " + parseErr.Error()}
		}
		rawTasks, parseErr = parseAIJSONTasks(fixed)
		if parseErr != nil {
			log.Printf("parse ai json (try 2): %v", parseErr)
			return nil, http.StatusBadGateway, map[string]interface{}{"detail": "Некорректный ответ ИИ: " + parseErr.Error()}
		}
	}
	if len(rawTasks) == 0 {
		return nil, http.StatusBadGateway, map[string]interface{}{"detail": "Нет задач в ответе ИИ"}
	}

	todayStr := strings.TrimSpace(body.Today)
	dateRe := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	if !dateRe.MatchString(todayStr) {
		todayStr = time.Now().Format("2006-01-02")
	}
	text := strings.TrimSpace(body.Text)

	tasks := make([]aiTaskRaw, 0, len(rawTasks))
	for i, one := range rawTasks {
		tasks = append(tasks, normalizeTask(one, text, todayStr, i, loc))
	}
	tasks = capBreakTasksPerDay(tasks, 2)

	dailyLimitTruncated := false
	remaining := limit - initialCount
	if remaining <= 0 {
		return nil, http.StatusForbidden, map[string]interface{}{
			"error":             "daily_limit",
			"detail":            "Лимит исчерпан",
			"smart_tasks_today": smartTasksQuotaPayload(initialCount, limit),
		}
	}
	if smartTaskNonBreakCount(tasks) > remaining {
		var trunc bool
		tasks, trunc = capSmartTasksRespectingQuota(tasks, remaining)
		dailyLimitTruncated = trunc
	}

	dbCtx := context.Background()
	if len(tasks) > 0 {
		delta := smartTaskNonBreakCount(tasks)
		_, ok, err := h.quota.TryIncrementCapped(dbCtx, userID, quotaDay, delta, limit)
		if err != nil {
			log.Printf("smart-task increment: %v", err)
			return nil, http.StatusInternalServerError, map[string]interface{}{"detail": "quota error"}
		}
		if !ok {
			c, _ := h.quota.CountForDay(dbCtx, userID, quotaDay)
			return nil, http.StatusForbidden, map[string]interface{}{
				"error":             "daily_limit",
				"detail":            "Лимит исчерпан",
				"smart_tasks_today": smartTasksQuotaPayload(c, limit),
			}
		}
	}

	googleCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	oauthCfg := newGoogleOAuthConfig(h.cfg)
	ts := oauthCfg.TokenSource(googleCtx, &oauth2.Token{RefreshToken: refresh})
	httpClient := oauth2.NewClient(googleCtx, ts)

	eventIDs := make([]*string, len(tasks))
	calendarFailed := 0
	for i := range tasks {
		id, err := createGoogleCalendarEvent(googleCtx, httpClient, &tasks[i], tzName)
		if err != nil {
			log.Printf("create calendar event: %v", err)
			eventIDs[i] = nil
			calendarFailed++
			continue
		}
		eventIDs[i] = &id
	}

	// Квота списана до создания событий; за задачи, не попавшие в календарь, возвращаем её —
	// иначе пользователь теряет дневной лимит и не получает ничего.
	nonBreakFailed := 0
	for i := range tasks {
		if eventIDs[i] == nil && !isBreakTask(&tasks[i]) {
			nonBreakFailed++
		}
	}
	if nonBreakFailed > 0 {
		if err := h.quota.Decrement(dbCtx, userID, quotaDay, nonBreakFailed); err != nil {
			log.Printf("smart-task quota refund: %v", err)
		}
	}
	if len(tasks) > 0 && calendarFailed == len(tasks) {
		used, _ := h.quota.CountForDay(dbCtx, userID, quotaDay)
		return nil, http.StatusBadGateway, map[string]interface{}{
			"detail":            "Не удалось добавить события в Google Календарь. Лимит не потрачен — попробуйте ещё раз.",
			"smart_tasks_today": smartTasksQuotaPayload(used, limit),
		}
	}

	out := map[string]interface{}{
		"ok":       true,
		"tasks":    tasksToAPI(tasks),
		"eventIds": eventIDsToIface(eventIDs),
	}
	if len(tasks) > 0 {
		out["task"] = tasksToAPI(tasks)[0]
		if eventIDs[0] != nil {
			out["eventId"] = *eventIDs[0]
		}
	}
	if dailyLimitTruncated {
		out["daily_limit_truncated"] = true
	}
	if calendarFailed > 0 {
		out["google_calendar_warning"] = fmt.Sprintf("Не удалось добавить %d из %d событий в Google Календарь", calendarFailed, len(tasks))
	}
	used, _ := h.quota.CountForDay(dbCtx, userID, quotaDay)
	out["smart_tasks_today"] = smartTasksQuotaPayload(used, limit)
	return out, 0, nil
}

// ServeHTTP: GET ?job= — статус; POST — поставить задачу в очередь и сразу ответить 202 (тяжёлая работа в фоне).
func (h *SmartTaskHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		job := strings.TrimSpace(r.URL.Query().Get("job"))
		if job == "" {
			http.Error(w, "job query required", http.StatusBadRequest)
			return
		}
		userID, ok := middleware.UserIDFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		st, err := h.jobs.GetForUser(r.Context(), job, userID)
		if err != nil {
			log.Printf("smart-task job get: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"detail": "Не удалось получить статус задачи"})
			return
		}
		if st == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"detail": "Задача не найдена или устарела"})
			return
		}
		switch st.Status {
		case "pending":
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "status": "pending"})
		case "running":
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "status": "running"})
		case "done":
			out := map[string]interface{}{"ok": true, "status": "done"}
			for k, v := range st.Result {
				out[k] = v
			}
			writeJSON(w, http.StatusOK, out)
		case "failed":
			// В БД статус называется failed; клиенту отдаём прежний "error" с тем же телом.
			resp := map[string]interface{}{"ok": false, "status": "error", "http_status": st.ErrHTTP}
			for k, v := range st.ErrBody {
				resp[k] = v
			}
			writeJSON(w, http.StatusOK, resp)
		default:
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "status": st.Status})
		}
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.googleOAuthReady() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"detail": "Google OAuth не настроен на сервере",
		})
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var body smartTaskPayload
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "Передайте поле text"})
		return
	}

	user, err := h.users.FindByID(r.Context(), userID)
	if err != nil || user == nil {
		http.Error(w, "user not found", http.StatusUnauthorized)
		return
	}

	todayStr := strings.TrimSpace(body.Today)
	dateRe := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	if !dateRe.MatchString(todayStr) {
		todayStr = time.Now().Format("2006-01-02")
	}

	limit := planSmartTaskLimit(h.cfg, user.Plan)

	quotaDay, _ := repository.ParseQuotaDay(repository.QuotaDayUTC())
	initialCount, err := h.quota.CountForDay(r.Context(), userID, quotaDay)
	if err != nil {
		log.Printf("smart-task quota read: %v", err)
		http.Error(w, "quota error", http.StatusInternalServerError)
		return
	}
	if initialCount >= limit {
		detail := "Достигнут дневной лимит умных задач на тарифе Free."
		if isProPlan(user.Plan) {
			detail = fmt.Sprintf("Достигнут дневной лимит умных задач на тарифе Pro (%d/день).", limit)
		}
		writeJSON(w, http.StatusForbidden, map[string]interface{}{
			"error":             "daily_limit",
			"detail":            detail,
			"smart_tasks_today": smartTasksQuotaPayload(initialCount, limit),
		})
		return
	}

	refresh, err := h.google.RefreshTokenByUserID(r.Context(), userID)
	if err != nil {
		log.Printf("smart-task refresh: %v", err)
		http.Error(w, "google link error", http.StatusInternalServerError)
		return
	}
	if refresh == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"detail": "Подключите Google Календарь в профиле. Нужен доступ на создание событий — отключите и снова подключите Google, если подключали только чтение.",
		})
		return
	}

	apiKey := strings.TrimSpace(h.cfg.OpenRouterAPIKey)
	if apiKey == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"detail": "Не задан OPENROUTER_API_KEY на сервере",
		})
		return
	}

	model := strings.TrimSpace(h.cfg.OpenRouterModel)
	if model == "" {
		model = "openai/gpt-4o-mini"
	}
	maxTok := h.cfg.SmartTaskMaxTokens
	if maxTok <= 0 {
		maxTok = 2048
	}

	jobID, err := h.jobs.Start(r.Context(), userID)
	if err != nil {
		log.Printf("smart-task job start: %v", err)
		http.Error(w, "job store error", http.StatusInternalServerError)
		return
	}

	plan := user.Plan
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("smart-task job panic: %v", rec)
				h.jobFinishErr(jobID, http.StatusInternalServerError, map[string]interface{}{
					"detail": "Внутренняя ошибка сервера",
				})
			}
		}()
		h.llmSlots <- struct{}{}
		defer func() { <-h.llmSlots }()
		h.jobMarkRunning(jobID)
		workCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		t0 := time.Now()
		userLoc, tzName := userLocation(user)
		// Строим контекст для AI здесь, чтобы HTTP 202 вернулся немедленно
		busyText := h.buildBusySlotsText(workCtx, todayStr, refresh, userLoc)
		log.Printf("[smart-task timing] google_calendar_fetch=%dms", time.Since(t0).Milliseconds())
		profileCtx := ""
		if strings.TrimSpace(user.DisplayName) != "" || strings.TrimSpace(user.Bio) != "" {
			profileCtx = "\nПрофиль: " + strings.TrimSpace(user.DisplayName) + ". " + strings.TrimSpace(user.Bio)
		}
		if note := strings.TrimSpace(user.PersonalizationNote); note != "" || strings.TrimSpace(user.Chronotype) != "" {
			profileCtx += "\nПерсонализация для планировщика:"
			if note != "" {
				profileCtx += " " + note + "."
			}
			if c := strings.TrimSpace(user.Chronotype); c != "" {
				if line := ChronotypePlannerRu(c); line != "" {
					profileCtx += " " + line + "."
				}
			}
			profileCtx += " Соблюдай персонализацию как занятость: не ставь новые задачи на пересечение с указанными днями/часами (буфер 10 мин), даже если этого слота нет в списке из Google Calendar."
		}
		nowLocal := time.Now().In(userLoc)
		datetimeStr := nowLocal.Format("2006-01-02 15:04")
		userMsg := fmt.Sprintf(
			"Текущие дата и время (%s): %s\nОриентир «сегодня» из приложения: %s.%s\n\nЗапрос пользователя:\n%s",
			tzName,
			datetimeStr,
			todayStr,
			profileCtx,
			text,
		)
		if busyText != "" {
			userMsg = busyText + userMsg
		}

		tAI := time.Now()
		okBody, errHTTP, errBody := h.executeSmartTaskSync(
			workCtx,
			userID,
			plan,
			body,
			refresh,
			userMsg,
			initialCount,
			limit,
			quotaDay,
			apiKey,
			model,
			maxTok,
			userLoc,
			tzName,
		)
		log.Printf("[smart-task timing] ai_and_gcal_create=%dms total=%dms model=%s",
			time.Since(tAI).Milliseconds(), time.Since(t0).Milliseconds(), model)
		if errBody != nil {
			h.jobFinishErr(jobID, errHTTP, errBody)
			return
		}
		h.jobFinishOK(jobID, okBody)
	}()

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"ok":     true,
		"async":  true,
		"job_id": jobID,
	})
}
