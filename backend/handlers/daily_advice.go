package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/oauth2"

	"pocketsecret/config"
	"pocketsecret/middleware"
	"pocketsecret/repository"
)

const dailyAdviceTZ = "Europe/Moscow"
const dailyAdviceHorizonDays = 6

const dailyAdviceSystem = `Ты — лаконичный и внимательный коуч по продуктивности для русскообразного пользователя приложения-календаря.

Задача: проанализировать расписание из Google Календаря (сегодня и ближайшие дни) и дать ОДИН полезный совет на текущий день — как лучше распределить энергию и время.

Правила ответа:
- Пиши только обычный текст на русском. Никакого Markdown, эмодзи, списков (через «-», «*» или цифры), JSON или выделения жирным.
- Строго 2–4 коротких предложения, общий объём до ~700 символов. Без воды: не пересказывай расписание целиком и не начинай с очевидностей; каждое предложение — новая мысль или действие.
- Завершай ответ последним цельным предложением с точкой в конце; не обрывай на середине слова, времени или скобки.
- Фокус — на сегодняшнем дне. Упоминай конкретику из календаря (например: «между созвоном X и встречей Y»), но не выдумывай события, которых нет во входных данных.
- Про завтра и дальше — не больше одного короткого предложения и только если это реально меняет план на сегодня (подготовка, сон, перенос нагрузки); иначе не отвлекай на будущие дни.
- Совет должен быть прикладным: конкретное действие (глубокая работа в окне, пауза после блока, лёгкая подготовка к важному пункту завтра сегодня вечером).
- Оценивай энергию: разгрузка при плотных блоках, «дыры» для фокуса или отдыха.
- Не давай медицинских, юридических или финансовых инструкций; избегай категоричных обещаний и слов «всегда», «никогда», «обязательно».
- Если календарь пуст — мягко предложи структурировать день (фокус, перерывы, глубокая работа) без давления и наставлений.`

// DailyAdviceHandler — GET /api/calendar/daily-advice
type DailyAdviceHandler struct {
	cfg    config.Config
	google *repository.GoogleCalendarRepository
	users  *repository.UserRepository
	cache  *repository.DailyAdviceRepository
}

func NewDailyAdviceHandler(cfg config.Config, google *repository.GoogleCalendarRepository, users *repository.UserRepository, cache *repository.DailyAdviceRepository) *DailyAdviceHandler {
	return &DailyAdviceHandler{cfg: cfg, google: google, users: users, cache: cache}
}

func (h *DailyAdviceHandler) googleConfigured() bool {
	return strings.TrimSpace(h.cfg.GoogleClientID) != "" &&
		strings.TrimSpace(h.cfg.GoogleClientSecret) != "" &&
		strings.TrimSpace(h.cfg.GoogleRedirectURI) != ""
}

// ServeHTTP handles GET /api/calendar/daily-advice?refresh=1
func (h *DailyAdviceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	refresh := r.URL.Query().Get("refresh") == "1" || strings.EqualFold(r.URL.Query().Get("refresh"), "true")

	if !h.googleConfigured() {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"linked": false,
			"advice": nil,
		})
		return
	}

	loc, err := time.LoadLocation(dailyAdviceTZ)
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	dayStr := now.Format("2006-01-02")

	// Должно быть ≥ таймаута HTTP-клиента OpenRouter (90 с), иначе контекст рвёт ответ раньше и получается 502.
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	refreshTok, err := h.google.RefreshTokenByUserID(ctx, userID)
	if err != nil {
		log.Printf("daily-advice refresh token: %v", err)
		http.Error(w, "failed to load calendar link", http.StatusInternalServerError)
		return
	}
	if refreshTok == "" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"linked": false,
			"advice": nil,
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

	oauthCfg := newGoogleOAuthConfig(h.cfg)
	ts := oauthCfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshTok})
	httpClient := oauth2.NewClient(ctx, ts)

	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	endDay := start.AddDate(0, 0, dailyAdviceHorizonDays)
	end := time.Date(endDay.Year(), endDay.Month(), endDay.Day(), 23, 59, 59, 999_999_999, loc)

	raw, err := fetchGoogleCalendarEventsWithRetry(ctx, httpClient, start, end)
	if err != nil {
		log.Printf("daily-advice google list: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"detail": "Не удалось загрузить события календаря"})
		return
	}

	events := make([]map[string]interface{}, 0, len(raw.Items))
	for i := range raw.Items {
		out, ok := mapGcalEvent(&raw.Items[i])
		if ok {
			events = append(events, out)
		}
	}
	// Сигнатура по всем событиям окна: новое или изменённое дело в любой из дней (в т.ч. только сегодня) инвалидирует кэш.
	sig := eventsSignature(events)
	scheduleText := formatScheduleForAdvice(events, loc)
	displayName := "пользователь"
	var coachCtx strings.Builder
	if h.users != nil {
		if u, err := h.users.FindByID(ctx, userID); err == nil && u != nil {
			personaHash := sha256.Sum256([]byte(
				strings.TrimSpace(u.Bio) + "\n" +
					strings.TrimSpace(u.PersonalizationNote) + "\n" +
					strings.TrimSpace(u.Chronotype),
			))
			sig += "|p:" + hex.EncodeToString(personaHash[:8])
			if n := strings.TrimSpace(u.DisplayName); n != "" {
				displayName = n
			}
			if b := strings.TrimSpace(u.Bio); b != "" {
				coachCtx.WriteString("\nПрофиль (раздел «о себе»): ")
				coachCtx.WriteString(b)
			}
			if p := strings.TrimSpace(u.PersonalizationNote); p != "" {
				coachCtx.WriteString("\nКраткая персонализация: ")
				coachCtx.WriteString(p)
			}
			if c := strings.TrimSpace(u.Chronotype); c != "" {
				if line := ChronotypePlannerRu(c); line != "" {
					coachCtx.WriteString("\nБиоритмы: ")
					coachCtx.WriteString(line)
				}
			}
		} else if err != nil {
			log.Printf("daily-advice user load: %v", err)
		}
	}

	if !refresh && h.cache != nil {
		if cached, cachedSig, hit, e := h.cache.Get(ctx, userID, dayStr); e == nil && hit && cachedSig == sig && strings.TrimSpace(cached) != "" {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"linked":       true,
				"advice":       strings.TrimSpace(cached),
				"cached":       true,
				"events_count": len(events),
			})
			return
		} else if e != nil {
			log.Printf("daily-advice cache read: %v", e)
		}
	}

	model := strings.TrimSpace(h.cfg.OpenRouterModelAdvice)
	if model == "" {
		model = strings.TrimSpace(h.cfg.OpenRouterModel)
	}
	maxTok := h.cfg.AdviceMaxTokens
	if maxTok <= 0 {
		maxTok = 700
	}

	userMsg := fmt.Sprintf(
		"Часовой пояс для интерпретации дат: %s\nТекущие дата и время: %s (%s)\nИмя (для обращения): %s%s\n\nСобытия календаря на сегодня и ближайшие %d дней:\n%s",
		dailyAdviceTZ,
		now.Format("2006-01-02 15:04"),
		weekdayRu(now.Weekday()),
		displayName,
		coachCtx.String(),
		dailyAdviceHorizonDays+1,
		scheduleText,
	)

	content, err := OpenRouterChatWithRetry(ctx, h.cfg, apiKey, model, maxTok, 0.45, dailyAdviceSystem, userMsg, 3)
	if err != nil {
		log.Printf("daily-advice openrouter: %v", err)
		// Показать последний сохранённый совет за сегодня, если генерация не удалась (лучше, чем пустой экран).
		fbCtx, fbCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if h.cache != nil {
			if cached, _, hit, e := h.cache.Get(fbCtx, userID, dayStr); e == nil && hit {
				if s := strings.TrimSpace(cached); s != "" {
					fbCancel()
					writeJSON(w, http.StatusOK, map[string]interface{}{
						"linked":        true,
						"advice":        s,
						"cached":        true,
						"stale":         true,
						"events_count":  len(events),
						"retry_suggest": "Можно обновить позже — показан сохранённый совет.",
					})
					return
				}
			}
		}
		fbCancel()
		writeJSON(w, http.StatusBadGateway, map[string]string{"detail": err.Error()})
		return
	}
	advice := clampAdviceText(strings.TrimSpace(content))

	if h.cache != nil && advice != "" {
		if err := h.cache.Upsert(ctx, userID, dayStr, advice, sig); err != nil {
			log.Printf("daily-advice cache write: %v", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"linked":       true,
		"advice":       advice,
		"cached":       false,
		"events_count": len(events),
	})
}

func eventsSignature(events []map[string]interface{}) string {
	lines := make([]string, 0, len(events))
	for _, e := range events {
		id, _ := e["id"].(string)
		st, _ := e["start"].(string)
		en, _ := e["end"].(string)
		title, _ := e["title"].(string)
		lines = append(lines, id+"\t"+st+"\t"+en+"\t"+title)
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

type schedLine struct {
	start time.Time
	day   string
	slot  string
	title string
}

func formatScheduleForAdvice(events []map[string]interface{}, loc *time.Location) string {
	if len(events) == 0 {
		return "(В календаре нет событий на эти дни.)\n"
	}
	lines := make([]schedLine, 0, len(events))
	for _, e := range events {
		st, _ := e["start"].(string)
		en, _ := e["end"].(string)
		title, _ := e["title"].(string)
		ad, _ := e["all_day"].(bool)
		t0, err := time.Parse(time.RFC3339, st)
		if err != nil {
			continue
		}
		t0 = t0.In(loc)
		var slot string
		if ad {
			slot = "весь день"
		} else {
			t1, err1 := time.Parse(time.RFC3339, en)
			if err1 != nil {
				slot = t0.Format("15:04") + "–…"
			} else {
				t1 = t1.In(loc)
				slot = t0.Format("15:04") + "–" + t1.Format("15:04")
			}
		}
		lines = append(lines, schedLine{
			start: t0,
			day:   t0.Format("2006-01-02"),
			slot:  slot,
			title: strings.TrimSpace(title),
		})
	}
	if len(lines) == 0 {
		return "(Не удалось разобрать события.)\n"
	}
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].start.Equal(lines[j].start) {
			return lines[i].title < lines[j].title
		}
		return lines[i].start.Before(lines[j].start)
	})
	var b strings.Builder
	curDay := ""
	for _, ln := range lines {
		if ln.day != curDay {
			if curDay != "" {
				b.WriteString("\n")
			}
			curDay = ln.day
			b.WriteString(ln.day)
			b.WriteString(":\n")
		}
		b.WriteString("  • ")
		b.WriteString(ln.slot)
		b.WriteString(" — ")
		b.WriteString(ln.title)
		b.WriteString("\n")
	}
	return b.String()
}

func fetchGoogleCalendarEventsWithRetry(ctx context.Context, httpClient *http.Client, timeMin, timeMax time.Time) (*gcalEventsResponse, error) {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(350+150*attempt) * time.Millisecond
			t := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				t.Stop()
				return nil, ctx.Err()
			case <-t.C:
				t.Stop()
			}
		}
		raw, err := fetchGoogleCalendarEvents(ctx, httpClient, timeMin, timeMax)
		if err == nil {
			return raw, nil
		}
		last = err
		log.Printf("daily-advice google list retry %d/3: %v", attempt+1, err)
	}
	return nil, last
}

func weekdayRu(w time.Weekday) string {
	return [...]string{"воскресенье", "понедельник", "вторник", "среда", "четверг", "пятница", "суббота"}[w]
}

func clampAdviceText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	const maxRunes = 1400
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "…"
}
