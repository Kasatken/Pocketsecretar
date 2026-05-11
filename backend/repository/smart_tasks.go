package repository

import (
	"context"
	"database/sql"
	"time"
)

type SmartTaskRepository struct {
	db *sql.DB
}

func NewSmartTaskRepository(db *sql.DB) *SmartTaskRepository {
	return &SmartTaskRepository{db: db}
}

// CountForDay returns how many smart tasks were created on the given UTC calendar day.
func (r *SmartTaskRepository) CountForDay(ctx context.Context, userID int64, dayUTC time.Time) (int, error) {
	day := time.Date(dayUTC.Year(), dayUTC.Month(), dayUTC.Day(), 0, 0, 0, 0, time.UTC)
	var n int
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(task_count, 0) FROM user_daily_smart_tasks
		WHERE user_id = $1 AND day = $2::date
	`, userID, day.Format("2006-01-02")).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return n, nil
}

// TryIncrementCapped atomically adds delta to the day counter without exceeding limit. Returns new total or current if would exceed.
func (r *SmartTaskRepository) TryIncrementCapped(ctx context.Context, userID int64, dayUTC time.Time, delta, limit int) (newCount int, ok bool, err error) {
	if delta <= 0 {
		n, e := r.CountForDay(ctx, userID, dayUTC)
		return n, true, e
	}
	if delta > limit {
		n, e := r.CountForDay(ctx, userID, dayUTC)
		return n, false, e
	}
	day := time.Date(dayUTC.Year(), dayUTC.Month(), dayUTC.Day(), 0, 0, 0, 0, time.UTC)
	dayStr := day.Format("2006-01-02")

	var out sql.NullInt64
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO user_daily_smart_tasks (user_id, day, task_count)
		VALUES ($1, $2::date, $3)
		ON CONFLICT (user_id, day) DO UPDATE SET
			task_count = user_daily_smart_tasks.task_count + EXCLUDED.task_count
		WHERE user_daily_smart_tasks.task_count + EXCLUDED.task_count <= $4
		RETURNING task_count
	`, userID, dayStr, delta, limit).Scan(&out)
	if err == sql.ErrNoRows {
		n, e := r.CountForDay(ctx, userID, dayUTC)
		return n, false, e
	}
	if err != nil {
		return 0, false, err
	}
	if !out.Valid {
		n, e := r.CountForDay(ctx, userID, dayUTC)
		return n, false, e
	}
	return int(out.Int64), true, nil
}

// SumForLastDays returns total smart tasks created by the user in the last days days.
func (r *SmartTaskRepository) SumForLastDays(ctx context.Context, userID int64, days int) (int, error) {
	since := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02")
	var total int
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(task_count), 0) FROM user_daily_smart_tasks WHERE user_id = $1 AND day >= $2::date`,
		userID, since,
	).Scan(&total)
	return total, err
}

// QuotaDayUTC returns today's date string in UTC (YYYY-MM-DD) for rate limiting.
func QuotaDayUTC() string {
	return time.Now().UTC().Format("2006-01-02")
}

// QuotaResetsAtUTC returns ISO time of next UTC midnight.
func QuotaResetsAtUTC() string {
	now := time.Now().UTC()
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	return next.Format(time.RFC3339)
}

// ParseQuotaDay parses YYYY-MM-DD as UTC date start.
func ParseQuotaDay(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
}
