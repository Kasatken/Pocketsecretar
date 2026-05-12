package repository

import (
	"context"
	"database/sql"
	"errors"
	"log"

	"pocketsecret/security"
)

type GoogleCalendarLink struct {
	UserID       int64
	GoogleSub    string
	EmailHint    string
	HasRefresh   bool
}

type GoogleCalendarRepository struct {
	db     *sql.DB
	encKey []byte // AES-256-GCM ключ; nil = шифрование отключено
}

func NewGoogleCalendarRepository(db *sql.DB, encKey []byte) *GoogleCalendarRepository {
	return &GoogleCalendarRepository{db: db, encKey: encKey}
}

func (r *GoogleCalendarRepository) Upsert(ctx context.Context, userID int64, googleSub, emailHint, refreshToken string) error {
	if googleSub == "" {
		return errors.New("empty google sub")
	}
	storedToken := refreshToken
	if refreshToken != "" {
		encrypted, err := security.EncryptToken(r.encKey, refreshToken)
		if err != nil {
			log.Printf("token encrypt: %v — saving plaintext", err)
		} else {
			storedToken = encrypted
		}
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
	`, userID, googleSub, emailHint, storedToken)
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
// Прозрачно расшифровывает токен если он зашифрован; plaintext-записи (старые) работают без изменений.
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
	if rt == "" {
		return "", nil
	}
	decrypted, err := security.DecryptToken(r.encKey, rt)
	if err != nil {
		log.Printf("token decrypt user=%d: %v — returning empty", userID, err)
		return "", nil
	}
	return decrypted, nil
}
