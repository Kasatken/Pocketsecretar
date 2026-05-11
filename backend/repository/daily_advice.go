package repository

import (
	"context"
	"database/sql"
	"errors"
)

type DailyAdviceRepository struct {
	db *sql.DB
}

func NewDailyAdviceRepository(db *sql.DB) *DailyAdviceRepository {
	return &DailyAdviceRepository{db: db}
}

// Get returns cached advice and events signature for the user and calendar day (YYYY-MM-DD).
func (r *DailyAdviceRepository) Get(ctx context.Context, userID int64, day string) (advice, sig string, ok bool, err error) {
	var a, s string
	e := r.db.QueryRowContext(ctx, `
		SELECT advice, events_sig FROM user_daily_advice
		WHERE user_id = $1 AND day = $2::date
	`, userID, day).Scan(&a, &s)
	if errors.Is(e, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if e != nil {
		return "", "", false, e
	}
	return a, s, true, nil
}

func (r *DailyAdviceRepository) Upsert(ctx context.Context, userID int64, day, advice, eventsSig string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO user_daily_advice (user_id, day, advice, events_sig)
		VALUES ($1, $2::date, $3, $4)
		ON CONFLICT (user_id, day) DO UPDATE SET
			advice = EXCLUDED.advice,
			events_sig = EXCLUDED.events_sig,
			updated_at = NOW()
	`, userID, day, advice, eventsSig)
	return err
}
