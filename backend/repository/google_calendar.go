package repository

import (
	"context"
	"database/sql"
	"errors"
)

type GoogleCalendarLink struct {
	UserID       int64
	GoogleSub    string
	EmailHint    string
	HasRefresh   bool
}

type GoogleCalendarRepository struct {
	db *sql.DB
}

func NewGoogleCalendarRepository(db *sql.DB) *GoogleCalendarRepository {
	return &GoogleCalendarRepository{db: db}
}

func (r *GoogleCalendarRepository) Upsert(ctx context.Context, userID int64, googleSub, emailHint, refreshToken string) error {
	if googleSub == "" {
		return errors.New("empty google sub")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO user_google_calendar (user_id, google_sub, email_hint, refresh_token, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			google_sub = EXCLUDED.google_sub,
			email_hint = EXCLUDED.email_hint,
			refresh_token = CASE
				WHEN EXCLUDED.refresh_token <> '' THEN EXCLUDED.refresh_token
				ELSE user_google_calendar.refresh_token
			END,
			updated_at = NOW()
	`, userID, googleSub, emailHint, refreshToken)
	return err
}

func (r *GoogleCalendarRepository) GetByUserID(ctx context.Context, userID int64) (*GoogleCalendarLink, error) {
	var row GoogleCalendarLink
	var rt string
	err := r.db.QueryRowContext(ctx, `
		SELECT user_id, google_sub, COALESCE(email_hint, ''), COALESCE(refresh_token, '')
		FROM user_google_calendar
		WHERE user_id = $1
	`, userID).Scan(&row.UserID, &row.GoogleSub, &row.EmailHint, &rt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	row.HasRefresh = rt != ""
	return &row, nil
}

// RefreshTokenByUserID returns stored OAuth refresh token, or empty string if none.
func (r *GoogleCalendarRepository) RefreshTokenByUserID(ctx context.Context, userID int64) (string, error) {
	var rt string
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(refresh_token, '')
		FROM user_google_calendar
		WHERE user_id = $1
	`, userID).Scan(&rt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return rt, nil
}
