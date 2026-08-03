package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

// SmartTaskJob — состояние асинхронного smart-task (таблица smart_task_jobs).
type SmartTaskJob struct {
	ID     string
	UserID int64
	Status string // pending, running, done, failed

	Result map[string]interface{} // при status=done — тот же JSON, что раньше отдавал синхронный POST

	ErrHTTP int                    // при status=failed
	ErrBody map[string]interface{} // тело для клиента (detail, error, smart_tasks_today, …)
}

// jobErrorPayload — формат колонки error (JSON в TEXT): HTTP-код и тело ответа клиенту.
type jobErrorPayload struct {
	HTTPStatus int                    `json:"http_status"`
	Body       map[string]interface{} `json:"body"`
}

// jobUUIDRe отсекает не-UUID из query-параметра до каста $1::uuid (иначе Postgres вернёт ошибку, а не «нет строк»).
var jobUUIDRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type SmartTaskJobRepository struct {
	db *sql.DB
}

func NewSmartTaskJobRepository(db *sql.DB) *SmartTaskJobRepository {
	return &SmartTaskJobRepository{db: db}
}

// Start создаёт job со статусом pending; UUID генерирует БД.
func (r *SmartTaskJobRepository) Start(ctx context.Context, userID int64) (string, error) {
	var id string
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO smart_task_jobs (user_id, status)
		VALUES ($1, 'pending')
		RETURNING id::text
	`, userID).Scan(&id)
	return id, err
}

func (r *SmartTaskJobRepository) MarkRunning(ctx context.Context, jobID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE smart_task_jobs SET status = 'running', updated_at = NOW() WHERE id = $1::uuid
	`, jobID)
	return err
}

// Finish помечает job выполненным и сохраняет result как JSONB.
func (r *SmartTaskJobRepository) Finish(ctx context.Context, jobID string, result map[string]interface{}) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		UPDATE smart_task_jobs SET status = 'done', result = $2::jsonb, updated_at = NOW() WHERE id = $1::uuid
	`, jobID, string(raw))
	return err
}

// Fail помечает job ошибочным; httpCode и body сохраняются вместе, чтобы GET мог воспроизвести прежний ответ клиенту.
func (r *SmartTaskJobRepository) Fail(ctx context.Context, jobID string, httpCode int, body map[string]interface{}) error {
	raw, err := json.Marshal(jobErrorPayload{HTTPStatus: httpCode, Body: body})
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		UPDATE smart_task_jobs SET status = 'failed', error = $2, updated_at = NOW() WHERE id = $1::uuid
	`, jobID, string(raw))
	return err
}

// GetForUser возвращает job по id, только если он принадлежит userID; nil без ошибки, если не найден
// (обработчик отдаёт на это тот же 404, что и раньше in-memory store).
func (r *SmartTaskJobRepository) GetForUser(ctx context.Context, jobID string, userID int64) (*SmartTaskJob, error) {
	if !jobUUIDRe.MatchString(strings.TrimSpace(jobID)) {
		return nil, nil
	}
	var (
		j       SmartTaskJob
		result  sql.NullString
		errText sql.NullString
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT id::text, user_id, status, result::text, error
		FROM smart_task_jobs
		WHERE id = $1::uuid AND user_id = $2
	`, strings.TrimSpace(jobID), userID).Scan(&j.ID, &j.UserID, &j.Status, &result, &errText)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if result.Valid && strings.TrimSpace(result.String) != "" {
		if e := json.Unmarshal([]byte(result.String), &j.Result); e != nil {
			return nil, e
		}
	}
	if errText.Valid && strings.TrimSpace(errText.String) != "" {
		var p jobErrorPayload
		if e := json.Unmarshal([]byte(errText.String), &p); e != nil {
			return nil, e
		}
		j.ErrHTTP = p.HTTPStatus
		j.ErrBody = p.Body
	}
	return &j, nil
}

// FailOrphaned помечает failed все pending/running job — вызывается один раз при старте процесса:
// их горутины погибли вместе с прошлым процессом, и поллинг должен получить ошибку сразу,
// а не ждать таймаута.
func (r *SmartTaskJobRepository) FailOrphaned(ctx context.Context) (int64, error) {
	raw, err := json.Marshal(jobErrorPayload{
		HTTPStatus: 503,
		Body:       map[string]interface{}{"detail": "Сервер был перезапущен во время выполнения. Повторите запрос."},
	})
	if err != nil {
		return 0, err
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE smart_task_jobs
		SET status = 'failed', error = $1, updated_at = NOW()
		WHERE status IN ('pending', 'running')
	`, string(raw))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteExpired удаляет завершённые job старше 15 минут и любые job старше часа
// (pending/running, осиротевшие после рестарта процесса).
func (r *SmartTaskJobRepository) DeleteExpired(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM smart_task_jobs
		WHERE (status IN ('done', 'failed') AND created_at < NOW() - INTERVAL '15 minutes')
		   OR created_at < NOW() - INTERVAL '1 hour'
	`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
