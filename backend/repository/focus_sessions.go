package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// FocusSession — совместная фокус-сессия (таблица focus_sessions).
type FocusSession struct {
	ID            int64
	StarterUserID int64
	Goal          string
	StartsAt      time.Time
	DurationMin   int
	Status        string
	ShareToken    string
}

// FocusParticipant — участник сессии вместе с telegram_id, чтобы бот мог написать в личку
// без второго запроса к users.
type FocusParticipant struct {
	UserID      int64
	TelegramID  int64
	DisplayName string
	Done        sql.NullBool
}

type FocusRepository struct {
	db *sql.DB
}

func NewFocusRepository(db *sql.DB) *FocusRepository {
	return &FocusRepository{db: db}
}

const focusSessionColumns = `id, starter_user_id, goal, starts_at, duration_min, status, share_token`

// CreateDraft заводит черновик: цель уже названа, время ещё не выбрано.
// Автор сразу становится участником — сессия «сам с собой» валидна, если никто не присоединится.
func (r *FocusRepository) CreateDraft(ctx context.Context, userID int64, goal, shareToken string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var id int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO focus_sessions (starter_user_id, goal, share_token)
		VALUES ($1, $2, $3)
		RETURNING id
	`, userID, goal, shareToken).Scan(&id); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO focus_participants (session_id, user_id) VALUES ($1, $2)
	`, id, userID); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// Schedule переводит черновик в назначенную сессию. Обновляет только собственный черновик
// автора, поэтому подставить чужой id в callback-данных нельзя.
func (r *FocusRepository) Schedule(ctx context.Context, sessionID, userID int64, startsAt time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE focus_sessions
		SET starts_at = $3, status = 'scheduled', updated_at = NOW()
		WHERE id = $1 AND starter_user_id = $2 AND status = 'draft'
	`, sessionID, userID, startsAt)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func scanFocusSession(row interface{ Scan(dest ...interface{}) error }) (*FocusSession, error) {
	var (
		s     FocusSession
		start sql.NullTime
	)
	err := row.Scan(&s.ID, &s.StarterUserID, &s.Goal, &start, &s.DurationMin, &s.Status, &s.ShareToken)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if start.Valid {
		s.StartsAt = start.Time
	}
	return &s, nil
}

// GetByToken ищет сессию по токену из диплинка. nil без ошибки, если токена нет.
func (r *FocusRepository) GetByToken(ctx context.Context, token string) (*FocusSession, error) {
	return scanFocusSession(r.db.QueryRowContext(ctx, `
		SELECT `+focusSessionColumns+` FROM focus_sessions WHERE share_token = $1
	`, token))
}

func (r *FocusRepository) GetByID(ctx context.Context, id int64) (*FocusSession, error) {
	return scanFocusSession(r.db.QueryRowContext(ctx, `
		SELECT `+focusSessionColumns+` FROM focus_sessions WHERE id = $1
	`, id))
}

// Join добавляет участника. Возвращает false, если он уже в сессии — тогда бот не шлёт
// автору повторное «к тебе присоединились».
func (r *FocusRepository) Join(ctx context.Context, sessionID, userID int64) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO focus_participants (session_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT (session_id, user_id) DO NOTHING
	`, sessionID, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *FocusRepository) Participants(ctx context.Context, sessionID int64) ([]FocusParticipant, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT p.user_id, COALESCE(u.telegram_id, 0), u.display_name, p.done
		FROM focus_participants p
		JOIN users u ON u.id = p.user_id
		WHERE p.session_id = $1
		ORDER BY p.joined_at
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FocusParticipant
	for rows.Next() {
		var p FocusParticipant
		if err := rows.Scan(&p.UserID, &p.TelegramID, &p.DisplayName, &p.Done); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetDone записывает ответ на «получилось?».
func (r *FocusRepository) SetDone(ctx context.Context, sessionID, userID int64, done bool) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE focus_participants SET done = $3 WHERE session_id = $1 AND user_id = $2
	`, sessionID, userID, done)
	return err
}

func (r *FocusRepository) listSessions(ctx context.Context, query string) ([]FocusSession, error) {
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FocusSession
	for rows.Next() {
		s, err := scanFocusSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// DueForReminder — назначенные сессии, до старта которых меньше пяти минут
// и напоминание ещё не уходило.
func (r *FocusRepository) DueForReminder(ctx context.Context) ([]FocusSession, error) {
	return r.listSessions(ctx, `
		SELECT `+focusSessionColumns+` FROM focus_sessions
		WHERE status = 'scheduled' AND notified_soon = FALSE
		  AND starts_at <= NOW() + INTERVAL '5 minutes' AND starts_at > NOW()
	`)
}

func (r *FocusRepository) MarkReminded(ctx context.Context, sessionID int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE focus_sessions SET notified_soon = TRUE, updated_at = NOW() WHERE id = $1
	`, sessionID)
	return err
}

// DueToStart — сессии, время которых наступило.
func (r *FocusRepository) DueToStart(ctx context.Context) ([]FocusSession, error) {
	return r.listSessions(ctx, `
		SELECT `+focusSessionColumns+` FROM focus_sessions
		WHERE status = 'scheduled' AND starts_at <= NOW()
	`)
}

// DueToFinish — сессии, у которых вышло время.
func (r *FocusRepository) DueToFinish(ctx context.Context) ([]FocusSession, error) {
	return r.listSessions(ctx, `
		SELECT `+focusSessionColumns+` FROM focus_sessions
		WHERE status = 'running'
		  AND starts_at + (duration_min || ' minutes')::interval <= NOW()
	`)
}

// SetStatus меняет статус, только если сессия ещё в ожидаемом состоянии: тикер может пройти
// по одной строке дважды (перезапуск, долгий запрос), а сообщения дублировать нельзя.
func (r *FocusRepository) SetStatus(ctx context.Context, sessionID int64, from, to string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE focus_sessions SET status = $3, updated_at = NOW()
		WHERE id = $1 AND status = $2
	`, sessionID, from, to)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// DeleteStaleDrafts убирает черновики, для которых так и не выбрали время.
func (r *FocusRepository) DeleteStaleDrafts(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM focus_sessions
		WHERE status = 'draft' AND created_at < NOW() - INTERVAL '1 day'
	`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
