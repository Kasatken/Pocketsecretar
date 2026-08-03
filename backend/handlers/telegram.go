package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pocketsecret/config"
	"pocketsecret/models"
	"pocketsecret/repository"
)

// TelegramHandler — вся петля бота: назвал дело → выбрал время → позвал других → сессия прошла.
// Вход в продукт происходит здесь, поэтому регистрации и подключения календаря на пути нет.
type TelegramHandler struct {
	cfg   config.Config
	api   *telegramClient
	users *repository.UserRepository
	focus *repository.FocusRepository
}

func NewTelegramHandler(cfg config.Config, users *repository.UserRepository, focus *repository.FocusRepository) *TelegramHandler {
	return &TelegramHandler{
		cfg:   cfg,
		api:   newTelegramClient(cfg.TelegramBotToken),
		users: users,
		focus: focus,
	}
}

// Длительность сессии задана дефолтом колонки duration_min — второго источника правды не заводим.
const maxGoalLen = 200

// ServeHTTP принимает вебхук. Telegram повторяет апдейт при любом не-200, а повтор здесь
// означал бы вторую сессию или второе сообщение — поэтому отвечаем 200 всегда,
// а ошибки только логируем.
func (h *TelegramHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != h.cfg.TelegramWebhookSecret {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	var upd tgUpdate
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&upd); err != nil {
		log.Printf("telegram: decode update: %v", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	var err error
	switch {
	case upd.Message != nil:
		err = h.handleMessage(ctx, upd.Message)
	case upd.CallbackQuery != nil:
		err = h.handleCallback(ctx, upd.CallbackQuery)
	case upd.InlineQuery != nil:
		err = h.handleInlineQuery(ctx, upd.InlineQuery)
	}
	if err != nil {
		log.Printf("telegram: handle update: %v", err)
	}
	w.WriteHeader(http.StatusOK)
}

// ensureUser находит пользователя по telegram_id или заводит нового.
// inviterID и source пишутся только при создании — это исходные данные для k-фактора.
func (h *TelegramHandler) ensureUser(ctx context.Context, from *tgUser, inviterID *int64, source string) (*models.User, bool, error) {
	u, err := h.users.FindByTelegramID(ctx, from.ID)
	if err != nil {
		return nil, false, err
	}
	if u != nil {
		return u, false, nil
	}

	name := strings.TrimSpace(from.FirstName)
	if name == "" {
		name = "Друг"
	}
	u, err = h.users.CreateTelegram(ctx, from.ID, from.Username, name, inviterID, source)
	// Два /start подряд могут прийти параллельно — проигравший просто перечитывает строку.
	if errors.Is(err, repository.ErrUserExists) {
		u, err = h.users.FindByTelegramID(ctx, from.ID)
		return u, false, err
	}
	if err != nil {
		return nil, false, err
	}
	return u, true, nil
}

func (h *TelegramHandler) handleMessage(ctx context.Context, msg *tgMessage) error {
	if msg.From == nil || msg.From.IsBot || msg.Chat == nil || msg.Chat.Type != "private" {
		return nil
	}
	text := strings.TrimSpace(msg.Text)

	if strings.HasPrefix(text, "/start") {
		return h.handleStart(ctx, msg.From, strings.TrimSpace(strings.TrimPrefix(text, "/start")))
	}
	if strings.HasPrefix(text, "/") {
		if _, _, err := h.ensureUser(ctx, msg.From, nil, "direct"); err != nil {
			return err
		}
		return h.api.sendMessage(ctx, msg.From.ID, helpText, nil)
	}
	if text == "" {
		return nil
	}
	return h.startDraft(ctx, msg.From, text)
}

const helpText = `Напиши, за какое дело не можешь сесть — обычным сообщением.

Я предложу время, поставлю таймер на 50 минут и дам позвать тех, с кем не так тоскливо. Работать вместе проще, чем в одиночку.`

// handleStart обрабатывает и первый вход, и переход по карточке сессии из чужого чата.
func (h *TelegramHandler) handleStart(ctx context.Context, from *tgUser, payload string) error {
	if payload == "" {
		u, isNew, err := h.ensureUser(ctx, from, nil, "direct")
		if err != nil {
			return err
		}
		greeting := "С возвращением. " + helpText
		if isNew {
			greeting = fmt.Sprintf("Привет, %s.\n\n%s", u.DisplayName, helpText)
		}
		return h.api.sendMessage(ctx, from.ID, greeting, nil)
	}

	session, err := h.focus.GetByToken(ctx, payload)
	if err != nil {
		return err
	}
	if session == nil {
		if _, _, err := h.ensureUser(ctx, from, nil, "direct"); err != nil {
			return err
		}
		return h.api.sendMessage(ctx, from.ID, "Эта сессия не найдена — возможно, ссылка устарела.\n\n"+helpText, nil)
	}

	// Пригласившим считается автор сессии — по этой связи и меряется рост.
	u, isNew, err := h.ensureUser(ctx, from, &session.StarterUserID, "focus_card")
	if err != nil {
		return err
	}

	if session.StarterUserID == u.ID {
		return h.api.sendMessage(ctx, from.ID, "Это твоя сессия — ты уже в ней.", nil)
	}
	if session.Status == "finished" || session.Status == "cancelled" {
		return h.api.sendMessage(ctx, from.ID, "Эта сессия уже закончилась. Но можно начать свою — напиши, за какое дело не можешь сесть.", nil)
	}

	joined, err := h.focus.Join(ctx, session.ID, u.ID)
	if err != nil {
		return err
	}
	if !joined {
		return h.api.sendMessage(ctx, from.ID, "Ты уже в этой сессии. Напишу, когда начнём.", nil)
	}

	welcome := fmt.Sprintf("Ты в сессии: «%s».\nНачало в %s, работаем %d минут.\n\nНапишу за пять минут до старта.",
		session.Goal, h.formatTime(session.StartsAt), session.DurationMin)
	if isNew {
		welcome = fmt.Sprintf("Привет, %s.\n\n%s", u.DisplayName, welcome)
	}
	if err := h.api.sendMessage(ctx, from.ID, welcome, nil); err != nil {
		return err
	}

	// Автору важно увидеть, что он не один — это то, ради чего он звал.
	starterChat, err := h.users.TelegramIDByUserID(ctx, session.StarterUserID)
	if err != nil || starterChat == 0 {
		return err
	}
	return h.api.sendMessage(ctx, starterChat, fmt.Sprintf("%s присоединился к твоей сессии «%s».", u.DisplayName, session.Goal), nil)
}

// startDraft создаёт черновик сессии и спрашивает время. Черновик живёт в БД, а не в памяти
// процесса, поэтому перезапуск между двумя сообщениями ничего не теряет.
func (h *TelegramHandler) startDraft(ctx context.Context, from *tgUser, goal string) error {
	u, _, err := h.ensureUser(ctx, from, nil, "direct")
	if err != nil {
		return err
	}
	if len([]rune(goal)) > maxGoalLen {
		goal = string([]rune(goal)[:maxGoalLen])
	}

	token, err := newShareToken()
	if err != nil {
		return err
	}
	sessionID, err := h.focus.CreateDraft(ctx, u.ID, goal, token)
	if err != nil {
		return err
	}

	return h.api.sendMessage(ctx, from.ID,
		fmt.Sprintf("«%s».\n\nКогда садимся?", goal),
		keyboard(
			[]tgButton{
				{Text: "Сейчас", CallbackData: fmt.Sprintf("t:%d:0", sessionID)},
				{Text: "Через 15 мин", CallbackData: fmt.Sprintf("t:%d:15", sessionID)},
			},
			[]tgButton{
				{Text: "Через 30 мин", CallbackData: fmt.Sprintf("t:%d:30", sessionID)},
				{Text: "Через час", CallbackData: fmt.Sprintf("t:%d:60", sessionID)},
			},
		))
}

func (h *TelegramHandler) handleCallback(ctx context.Context, cb *tgCallback) error {
	if cb.From == nil {
		return nil
	}
	u, _, err := h.ensureUser(ctx, cb.From, nil, "direct")
	if err != nil {
		return err
	}

	parts := strings.Split(cb.Data, ":")
	if len(parts) != 3 {
		return h.api.answerCallback(ctx, cb.ID, "")
	}
	sessionID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return h.api.answerCallback(ctx, cb.ID, "")
	}

	switch parts[0] {
	case "t":
		return h.scheduleFromCallback(ctx, cb, u, sessionID, parts[2])
	case "d":
		if err := h.focus.SetDone(ctx, sessionID, u.ID, parts[2] == "1"); err != nil {
			return err
		}
		if err := h.api.answerCallback(ctx, cb.ID, "Записал"); err != nil {
			return err
		}
		reply := "Записал. Дело не сделано — бывает; следующая попытка считается так же."
		if parts[2] == "1" {
			reply = "Отлично. Дело сделано — это и был весь смысл."
		}
		return h.api.sendMessage(ctx, cb.From.ID, reply, nil)
	}
	return h.api.answerCallback(ctx, cb.ID, "")
}

func (h *TelegramHandler) scheduleFromCallback(ctx context.Context, cb *tgCallback, u *models.User, sessionID int64, offset string) error {
	mins, err := strconv.Atoi(offset)
	if err != nil {
		return h.api.answerCallback(ctx, cb.ID, "")
	}
	startsAt := time.Now().Add(time.Duration(mins) * time.Minute)

	ok, err := h.focus.Schedule(ctx, sessionID, u.ID, startsAt)
	if err != nil {
		return err
	}
	if !ok {
		// Черновик уже назначен (двойное нажатие) либо принадлежит другому пользователю.
		return h.api.answerCallback(ctx, cb.ID, "Время уже выбрано")
	}
	session, err := h.focus.GetByID(ctx, sessionID)
	if err != nil || session == nil {
		return err
	}
	if err := h.api.answerCallback(ctx, cb.ID, ""); err != nil {
		return err
	}

	share := session.ShareToken
	return h.api.sendMessage(ctx, cb.From.ID,
		fmt.Sprintf("Договорились: %s, %d минут.\n\nОдному сесть тяжелее. Кинь карточку в любой чат — кто-то сядет рядом.",
			h.formatTime(session.StartsAt), session.DurationMin),
		keyboard([]tgButton{{Text: "Позвать", SwitchInlineQuery: &share}}))
}

// handleInlineQuery отдаёт карточку сессии, которую пользователь отправляет в свой чат.
// Кнопка на карточке — диплинк в бота: так присоединившийся сразу оказывается в личке,
// и боту не нужны права ни в одном групповом чате.
func (h *TelegramHandler) handleInlineQuery(ctx context.Context, q *tgInlineQuery) error {
	if q.From == nil {
		return nil
	}
	token := strings.TrimSpace(q.Query)
	if token == "" {
		return h.api.answerInlineQuery(ctx, q.ID, []interface{}{})
	}
	session, err := h.focus.GetByToken(ctx, token)
	if err != nil {
		return err
	}
	if session == nil || session.Status != "scheduled" {
		return h.api.answerInlineQuery(ctx, q.ID, []interface{}{})
	}
	author, err := h.users.FindByID(ctx, session.StarterUserID)
	if err != nil || author == nil {
		return err
	}

	text := fmt.Sprintf("%s садится за дело в %s:\n«%s»\n\n%d минут в тишине. Кто со мной?",
		author.DisplayName, h.formatTime(session.StartsAt), session.Goal, session.DurationMin)

	result := map[string]interface{}{
		"type":                  "article",
		"id":                    session.ShareToken,
		"title":                 "Позвать в фокус-сессию",
		"description":           fmt.Sprintf("%s · %s", h.formatTime(session.StartsAt), session.Goal),
		"input_message_content": map[string]interface{}{"message_text": text},
		"reply_markup": keyboard([]tgButton{{
			Text: "Я тоже",
			URL:  fmt.Sprintf("https://t.me/%s?start=%s", h.cfg.TelegramBotUsername, session.ShareToken),
		}}),
	}
	return h.api.answerInlineQuery(ctx, q.ID, []interface{}{result})
}

// Tick прогоняет расписание: напоминание за пять минут, старт и итог.
// Вызывается из main по таймеру — состояние сессий живёт в БД, а не в горутинах,
// поэтому перезапуск процесса ничего не теряет.
func (h *TelegramHandler) Tick(ctx context.Context) {
	h.remindSoon(ctx)
	h.startDue(ctx)
	h.finishDue(ctx)
}

func (h *TelegramHandler) remindSoon(ctx context.Context) {
	sessions, err := h.focus.DueForReminder(ctx)
	if err != nil {
		log.Printf("focus: due for reminder: %v", err)
		return
	}
	for _, s := range sessions {
		h.broadcast(ctx, s.ID, fmt.Sprintf("Через пять минут — «%s». Закрой лишние вкладки.", s.Goal))
		if err := h.focus.MarkReminded(ctx, s.ID); err != nil {
			log.Printf("focus: mark reminded %d: %v", s.ID, err)
		}
	}
}

func (h *TelegramHandler) startDue(ctx context.Context) {
	sessions, err := h.focus.DueToStart(ctx)
	if err != nil {
		log.Printf("focus: due to start: %v", err)
		return
	}
	for _, s := range sessions {
		ok, err := h.focus.SetStatus(ctx, s.ID, "scheduled", "running")
		if err != nil {
			log.Printf("focus: start %d: %v", s.ID, err)
			continue
		}
		if !ok {
			continue
		}
		people, err := h.focus.Participants(ctx, s.ID)
		if err != nil {
			log.Printf("focus: participants %d: %v", s.ID, err)
			continue
		}
		text := fmt.Sprintf("Поехали. %d минут на «%s».", s.DurationMin, s.Goal)
		if len(people) > 1 {
			text += fmt.Sprintf("\n\nРядом ещё %d человек — вы начали одновременно.", len(people)-1)
		}
		for _, p := range people {
			h.send(ctx, p.TelegramID, text)
		}
	}
}

func (h *TelegramHandler) finishDue(ctx context.Context) {
	sessions, err := h.focus.DueToFinish(ctx)
	if err != nil {
		log.Printf("focus: due to finish: %v", err)
		return
	}
	for _, s := range sessions {
		ok, err := h.focus.SetStatus(ctx, s.ID, "running", "finished")
		if err != nil {
			log.Printf("focus: finish %d: %v", s.ID, err)
			continue
		}
		if !ok {
			continue
		}
		people, err := h.focus.Participants(ctx, s.ID)
		if err != nil {
			log.Printf("focus: participants %d: %v", s.ID, err)
			continue
		}
		for _, p := range people {
			if p.TelegramID == 0 {
				continue
			}
			if err := h.api.sendMessage(ctx, p.TelegramID,
				fmt.Sprintf("Время вышло. «%s» — получилось?", s.Goal),
				keyboard([]tgButton{
					{Text: "Да", CallbackData: fmt.Sprintf("d:%d:1", s.ID)},
					{Text: "Нет", CallbackData: fmt.Sprintf("d:%d:0", s.ID)},
				})); err != nil {
				log.Printf("focus: ask done %d: %v", s.ID, err)
			}
		}
	}
}

func (h *TelegramHandler) broadcast(ctx context.Context, sessionID int64, text string) {
	people, err := h.focus.Participants(ctx, sessionID)
	if err != nil {
		log.Printf("focus: participants %d: %v", sessionID, err)
		return
	}
	for _, p := range people {
		h.send(ctx, p.TelegramID, text)
	}
}

func (h *TelegramHandler) send(ctx context.Context, chatID int64, text string) {
	if chatID == 0 {
		return
	}
	if err := h.api.sendMessage(ctx, chatID, text, nil); err != nil {
		log.Printf("telegram: send to %d: %v", chatID, err)
	}
}

func (h *TelegramHandler) formatTime(t time.Time) string {
	return t.In(config.AppLocation()).Format("15:04")
}

// newShareToken — токен для диплинка t.me/<bot>?start=<token>; hex безопасен для payload.
func newShareToken() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
