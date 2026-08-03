package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"pocketsecret/config"
)

// DayCount хранит количество выполненных задач за один день.
type DayCount struct {
	Day   string // "YYYY-MM-DD" в часовом поясе Europe/Moscow
	Count int
}

// WeeklyReportRepository читает агрегированные данные для еженедельного отчёта.
// Только SELECT — не меняет никакие данные.
type WeeklyReportRepository struct {
	db *sql.DB
}

func NewWeeklyReportRepository(db *sql.DB) *WeeklyReportRepository {
	return &WeeklyReportRepository{db: db}
}

// CompletionsByDay возвращает количество выполненных задач по дням
// в диапазоне [start, end); tz — IANA-таймзона границ дня (пустая — таймзона приложения).
func (r *WeeklyReportRepository) CompletionsByDay(ctx context.Context, userID int64, start, end time.Time, tz string) ([]DayCount, error) {
	if strings.TrimSpace(tz) == "" {
		tz = config.AppTimeZone
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			DATE(completed_at AT TIME ZONE $4::text)::text AS day,
			COUNT(*)::int AS cnt
		FROM calendar_event_completions
		WHERE user_id = $1
		  AND completed_at >= $2
		  AND completed_at < $3
		GROUP BY 1
		ORDER BY 1
	`, userID, start.UTC(), end.UTC(), tz)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DayCount
	for rows.Next() {
		var dc DayCount
		if err := rows.Scan(&dc.Day, &dc.Count); err != nil {
			return nil, err
		}
		result = append(result, dc)
	}
	return result, rows.Err()
}

// CompletionsTotalForRange возвращает суммарное число выполненных задач в диапазоне [start, end).
func (r *WeeklyReportRepository) CompletionsTotalForRange(ctx context.Context, userID int64, start, end time.Time) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)::int
		FROM calendar_event_completions
		WHERE user_id = $1 AND completed_at >= $2 AND completed_at < $3
	`, userID, start.UTC(), end.UTC()).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return count, err
}

// SmartTasksSumForRange суммирует AI-задачи из user_daily_smart_tasks за UTC-дни в диапазоне [start, end).
func (r *WeeklyReportRepository) SmartTasksSumForRange(ctx context.Context, userID int64, start, end time.Time) (int, error) {
	var total int
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(task_count), 0)::int
		FROM user_daily_smart_tasks
		WHERE user_id = $1
		  AND day >= $2::date
		  AND day <  $3::date
	`, userID, start.UTC().Format("2006-01-02"), end.UTC().Format("2006-01-02")).Scan(&total)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return total, err
}
