package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"pocketsecret/config"
	"pocketsecret/middleware"
	"pocketsecret/repository"
)

// weekdayShortRu: индекс = time.Weekday (0=Вс, 1=Пн, … 6=Сб).
var weekdayShortRu = [...]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}

// monthNamesRu: индекс = time.Month (1=янв … 12=дек).
var monthNamesRu = [...]string{
	"", "января", "февраля", "марта", "апреля", "мая", "июня",
	"июля", "августа", "сентября", "октября", "ноября", "декабря",
}

const weeklyReportSystemPrompt = `Ты — краткий AI-помощник для еженедельного отчёта по продуктивности.
Пиши только текст на русском. Без Markdown, списков, эмодзи, выделений жирным.
Ровно 3–4 предложения. Тон — поддерживающий, честный, без пустых похвал.
Первое предложение: оценка недели с конкретными цифрами из данных.
Второе: что сработало хорошо (лучший день или динамика vs прошлой недели).
Третье/четвёртое: один конкретный совет на следующую неделю — с учётом предстоящих событий, если они есть.
Завершай законченным предложением с точкой. Не обрывай на полуслове.`

// WeeklyReportHandler — GET /api/reports/weekly
type WeeklyReportHandler struct {
	cfg         config.Config
	users       *repository.UserRepository
	google      *repository.GoogleCalendarRepository
	completions *repository.CalendarCompletionRepository
	smartTasks  *repository.SmartTaskRepository
	weeklyRepo  *repository.WeeklyReportRepository
}

func NewWeeklyReportHandler(
	cfg config.Config,
	users *repository.UserRepository,
	google *repository.GoogleCalendarRepository,
	completions *repository.CalendarCompletionRepository,
	smartTasks *repository.SmartTaskRepository,
	weeklyRepo *repository.WeeklyReportRepository,
) *WeeklyReportHandler {
	return &WeeklyReportHandler{
		cfg:         cfg,
		users:       users,
		google:      google,
		completions: completions,
		smartTasks:  smartTasks,
		weeklyRepo:  weeklyRepo,
	}
}

func (h *WeeklyReportHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	// ── Профиль пользователя (таймзона недели и имя для промпта) ─────────────
	user, err := h.users.FindByID(ctx, userID)
	if err != nil || user == nil {
		http.Error(w, "user not found", http.StatusInternalServerError)
		return
	}
	displayName := strings.TrimSpace(user.DisplayName)
	if displayName == "" {
		displayName = "пользователь"
	}

	// ── Границы текущей недели (пн 00:00 … вс 23:59 в таймзоне пользователя) ─
	loc, tzName := userLocation(user)
	now := time.Now().In(loc)
	wd := int(now.Weekday()) // 0 = Воскресенье
	if wd == 0 {
		wd = 7
	}
	monday := time.Date(now.Year(), now.Month(), now.Day()-(wd-1), 0, 0, 0, 0, loc)
	nextMonday := monday.AddDate(0, 0, 7)
	lastMonday := monday.AddDate(0, 0, -7)

	sunday := monday.AddDate(0, 0, 6)
	weekLabel := fmt.Sprintf("%d–%d %s", monday.Day(), sunday.Day(), monthNamesRu[sunday.Month()])
	if monday.Month() != sunday.Month() {
		weekLabel = fmt.Sprintf(
			"%d %s – %d %s",
			monday.Day(), monthNamesRu[monday.Month()],
			sunday.Day(), monthNamesRu[sunday.Month()],
		)
	}

	// ── Выполненные задачи по дням текущей недели ────────────────────────────
	dayCompletions, err := h.weeklyRepo.CompletionsByDay(ctx, userID, monday, nextMonday, tzName)
	if err != nil {
		log.Printf("weekly-report completions by day: %v", err)
		dayCompletions = nil
	}

	// Строим карту day→count и заполняем все 7 дней (включая дни с нулём)
	dayCountMap := make(map[string]int)
	for _, dc := range dayCompletions {
		dayCountMap[dc.Day] = dc.Count
	}

	totalThisWeek := 0
	bestDayCount := 0
	bestDayName := ""
	completionsByDay := make([]map[string]interface{}, 7)
	for i := 0; i < 7; i++ {
		d := monday.AddDate(0, 0, i)
		dayStr := d.Format("2006-01-02")
		wdShort := weekdayShortRu[d.Weekday()]
		cnt := dayCountMap[dayStr]
		totalThisWeek += cnt
		if cnt > bestDayCount {
			bestDayCount = cnt
			bestDayName = wdShort
		}
		completionsByDay[i] = map[string]interface{}{
			"weekday": wdShort,
			"count":   cnt,
		}
	}

	// ── Прошлая неделя (для сравнения) ───────────────────────────────────────
	totalLastWeek, err := h.weeklyRepo.CompletionsTotalForRange(ctx, userID, lastMonday, monday)
	if err != nil {
		log.Printf("weekly-report last week total: %v", err)
	}

	// ── AI-задачи за текущую неделю ───────────────────────────────────────────
	smartTasksWeek, err := h.weeklyRepo.SmartTasksSumForRange(ctx, userID, monday, nextMonday)
	if err != nil {
		log.Printf("weekly-report smart tasks: %v", err)
	}

	// ── Стрик ─────────────────────────────────────────────────────────────────
	compStats, err := h.completions.GetStats(ctx, userID, user.Timezone)
	if err != nil {
		log.Printf("weekly-report comp stats: %v", err)
	}

	// ── Google Calendar: события следующей недели (опционально) ──────────────
	var upcomingEvents []map[string]interface{}
	googleLinked := false

	refreshTok, gErr := h.google.RefreshTokenByUserID(ctx, userID)
	if gErr == nil && refreshTok != "" &&
		strings.TrimSpace(h.cfg.GoogleClientID) != "" &&
		strings.TrimSpace(h.cfg.GoogleClientSecret) != "" {

		googleLinked = true
		oauthCfg := newGoogleOAuthConfig(h.cfg)
		ts := oauthCfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshTok})
		httpClient := oauth2.NewClient(ctx, ts)

		nextWeekEnd := nextMonday.AddDate(0, 0, 7)
		raw, gCalErr := fetchGoogleCalendarEventsWithRetry(ctx, httpClient, nextMonday, nextWeekEnd)
		if gCalErr != nil {
			log.Printf("weekly-report google upcoming: %v", gCalErr)
		} else {
			for i := range raw.Items {
				ev, ok := mapGcalEvent(&raw.Items[i])
				if !ok {
					continue
				}
				// Пропускаем события на весь день — менее информативны в отчёте.
				if allDay, _ := ev["all_day"].(bool); allDay {
					continue
				}
				upcomingEvents = append(upcomingEvents, ev)
				if len(upcomingEvents) >= 5 {
					break
				}
			}
		}
	}

	// ── AI-нарратив (деградирует без ошибки, если ключ не задан) ─────────────
	aiNarrative := ""
	apiKey := strings.TrimSpace(h.cfg.OpenRouterAPIKey)
	if apiKey != "" {
		model := strings.TrimSpace(h.cfg.OpenRouterModelAdvice)
		if model == "" {
			model = strings.TrimSpace(h.cfg.OpenRouterModel)
		}
		userPrompt := buildWeeklyPrompt(
			displayName, weekLabel,
			totalThisWeek, totalLastWeek,
			smartTasksWeek, compStats.Streak,
			bestDayName, bestDayCount,
			completionsByDay, upcomingEvents,
		)
		content, aiErr := OpenRouterChatWithRetry(
			ctx, h.cfg, apiKey, model, 500, 0.45,
			weeklyReportSystemPrompt, userPrompt, 2,
		)
		if aiErr != nil {
			log.Printf("weekly-report openrouter: %v", aiErr)
		} else {
			aiNarrative = strings.TrimSpace(content)
		}
	}

	// ── Дельта в процентах (nil если нет данных за прошлую неделю) ───────────
	var deltaPct interface{}
	if totalLastWeek > 0 {
		deltaPct = int(float64(totalThisWeek-totalLastWeek) / float64(totalLastWeek) * 100)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"week_label":            weekLabel,
		"completed_this_week":   totalThisWeek,
		"completed_last_week":   totalLastWeek,
		"delta_pct":             deltaPct,
		"smart_tasks_this_week": smartTasksWeek,
		"streak":                compStats.Streak,
		"best_day": map[string]interface{}{
			"weekday": bestDayName,
			"count":   bestDayCount,
		},
		"completions_by_day": completionsByDay,
		"upcoming_events":    upcomingEvents,
		"ai_narrative":       aiNarrative,
		"google_linked":      googleLinked,
	})
}

// buildWeeklyPrompt собирает текстовый промпт для AI из собранных данных.
func buildWeeklyPrompt(
	name, weekLabel string,
	thisWeek, lastWeek, smartTasks, streak int,
	bestDay string, bestDayCount int,
	byDay []map[string]interface{},
	upcoming []map[string]interface{},
) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Пользователь: %s\n", name)
	fmt.Fprintf(&b, "Неделя: %s\n\n", weekLabel)
	fmt.Fprintf(&b, "Выполнено задач: %d", thisWeek)
	if lastWeek > 0 {
		diff := thisWeek - lastWeek
		sign := "+"
		if diff < 0 {
			sign = ""
		}
		fmt.Fprintf(&b, " (прошлая неделя: %d, %s%d)", lastWeek, sign, diff)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "Создано AI-задач: %d\n", smartTasks)
	fmt.Fprintf(&b, "Стрик: %d дней\n", streak)
	if bestDay != "" && bestDayCount > 0 {
		fmt.Fprintf(&b, "Лучший день: %s (%d задач)\n", bestDay, bestDayCount)
	}

	b.WriteString("По дням: ")
	parts := make([]string, len(byDay))
	for i, d := range byDay {
		wd, _ := d["weekday"].(string)
		cnt, _ := d["count"].(int)
		parts[i] = fmt.Sprintf("%s %d", wd, cnt)
	}
	b.WriteString(strings.Join(parts, " / "))
	b.WriteString("\n")

	if len(upcoming) > 0 {
		b.WriteString("\nСобытия следующей недели:\n")
		for _, ev := range upcoming {
			title, _ := ev["title"].(string)
			start, _ := ev["start"].(string)
			fmt.Fprintf(&b, "- %s — %s\n", start, title)
		}
	}

	return b.String()
}
