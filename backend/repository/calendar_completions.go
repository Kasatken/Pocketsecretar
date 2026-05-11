package repository

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
)

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
