package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Минимальный клиент Bot API: только те методы, которые нужны петле фокус-сессий.
// Библиотеку не тянем — здесь три вызова и четыре структуры.

type telegramClient struct {
	token string
	http  *http.Client
}

func newTelegramClient(token string) *telegramClient {
	return &telegramClient{
		token: token,
		// Bot API отвечает быстро; длинный таймаут только задержал бы тикер рассылки.
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *telegramClient) call(ctx context.Context, method string, payload interface{}) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("telegram %s: %s: %s", method, resp.Status, string(body))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// sendMessage шлёт текст в личку пользователю; markup может быть nil.
func (c *telegramClient) sendMessage(ctx context.Context, chatID int64, text string, markup *tgInlineKeyboard) error {
	payload := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}
	if markup != nil {
		payload["reply_markup"] = markup
	}
	return c.call(ctx, "sendMessage", payload)
}

// answerCallback убирает «часики» с нажатой кнопки; text показывается всплывающим уведомлением.
func (c *telegramClient) answerCallback(ctx context.Context, callbackID, text string) error {
	return c.call(ctx, "answerCallbackQuery", map[string]interface{}{
		"callback_query_id": callbackID,
		"text":              text,
	})
}

// answerInlineQuery отдаёт единственную карточку сессии для отправки в любой чат.
func (c *telegramClient) answerInlineQuery(ctx context.Context, queryID string, results []interface{}) error {
	return c.call(ctx, "answerInlineQuery", map[string]interface{}{
		"inline_query_id": queryID,
		"results":         results,
		// Карточка привязана к конкретной сессии, кэшировать её между запросами нельзя.
		"cache_time":  0,
		"is_personal": true,
	})
}

// --- входящие апдейты (разбираем только используемые поля) ---

type tgUpdate struct {
	Message       *tgMessage     `json:"message"`
	CallbackQuery *tgCallback    `json:"callback_query"`
	InlineQuery   *tgInlineQuery `json:"inline_query"`
}

type tgUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

type tgChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type tgMessage struct {
	From *tgUser `json:"from"`
	Chat *tgChat `json:"chat"`
	Text string  `json:"text"`
}

type tgCallback struct {
	ID      string     `json:"id"`
	From    *tgUser    `json:"from"`
	Data    string     `json:"data"`
	Message *tgMessage `json:"message"`
}

type tgInlineQuery struct {
	ID    string  `json:"id"`
	From  *tgUser `json:"from"`
	Query string  `json:"query"`
}

// --- клавиатуры ---

type tgButton struct {
	Text string `json:"text"`
	// CallbackData приходит обратно в callback_query.data (лимит Bot API — 64 байта).
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
	// SwitchInlineQuery открывает выбор чата с подставленным запросом — так карточка
	// уходит в чужой чат от имени пользователя, и бота не нужно никуда добавлять.
	SwitchInlineQuery *string `json:"switch_inline_query,omitempty"`
}

type tgInlineKeyboard struct {
	InlineKeyboard [][]tgButton `json:"inline_keyboard"`
}

func keyboard(rows ...[]tgButton) *tgInlineKeyboard {
	return &tgInlineKeyboard{InlineKeyboard: rows}
}
