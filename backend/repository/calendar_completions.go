package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"
)

type CompletionStats struct {
	CompletedWeek  int
	TotalCompleted int
	Streak         int
}

type CalendarCompletionRepository struct {
	db *sql.DB
}

func NewCalendarCompletionRepository(db *sql.DB) *CalendarCompletionRepository {
	return &CalendarCompletionRepository{db: db}
}

func (r *CalendarCompletionRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, nil)
}

// TryInsertCompletion inserts a completion row; returns true if a new row was inserted.
func (r *CalendarCompletionRepository) TryInsertCompletion(ctx context.Context, tx *sql.Tx, userID int64, googleEventID string) (bool, error) {
	res, err := tx.ExecContext(ctx, `
		INSERT INTO calendar_event_completions (user_id, google_event_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, google_event_id) DO NOTHING
	`, userID, googleEventID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// CompletedEventIDs returns the subset of googleEventIDs that are marked completed for this user.
func (r *CalendarCompletionRepository) CompletedEventIDs(ctx context.Context, userID int64, googleEventIDs []string) (map[string]bool, error) {
	out := make(map[string]bool)
	if len(googleEventIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT google_event_id
		FROM calendar_event_completions
		WHERE user_id = $1 AND google_event_id = ANY($2::text[])
	`, userID, pq.Array(googleEventIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// GetStats returns completion stats for the user.
func (r *CalendarCompletionRepository) GetStats(ctx context.Context, userID int64) (CompletionStats, error) {
	var total, week int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM calendar_event_completions WHERE user_id = $1`,
		userID,
	).Scan(&total); err != nil {
		return CompletionStats{}, err
	}
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM calendar_event_completions WHERE user_id = $1 AND completed_at >= NOW() - INTERVAL '7 days'`,
		userID,
	).Scan(&week); err != nil {
		return CompletionStats{}, err
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT DATE(completed_at AT TIME ZONE 'Europe/Moscow')
		 FROM calendar_event_completions
		 WHERE user_id = $1 AND completed_at >= NOW() - INTERVAL '365 days'
		 ORDER BY 1 DESC`,
		userID,
	)
	if err != nil {
		return CompletionStats{}, err
	}
	defer rows.Close()

	var days []time.Time
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return CompletionStats{}, err
		}
		days = append(days, d)
	}
	if err := rows.Err(); err != nil {
		return CompletionStats{}, err
	}

	return CompletionStats{
		CompletedWeek:  week,
		TotalCompleted: total,
		Streak:         calcCompletionStreak(days),
	}, nil
}

func calcCompletionStreak(days []time.Time) int {
	if len(days) == 0 {
		return 0
	}
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	todayUTC := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	yesterdayUTC := todayUTC.AddDate(0, 0, -1)

	// most recent day with completions
	first := days[0].UTC().Truncate(24 * time.Hour)
	cursor := todayUTC
	if !first.Equal(todayUTC) {
		if !first.Equal(yesterdayUTC) {
			return 0
		}
		cursor = yesterdayUTC
	}

	streak := 0
	for _, d := range days {
		day := d.UTC().Truncate(24 * time.Hour)
		if day.Equal(cursor) {
			streak++
			cursor = cursor.AddDate(0, 0, -1)
		} else if day.Before(cursor) {
			break
		}
	}
	return streak
}
