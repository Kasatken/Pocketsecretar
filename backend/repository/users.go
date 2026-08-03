package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"

	"pocketsecret/models"
)

var ErrUserExists = errors.New("user exists")

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func scanUser(row interface{ Scan(dest ...interface{}) error }) (*models.User, error) {
	var u models.User
	var last sql.NullTime
	err := row.Scan(
		&u.ID,
		&u.Email,
		&u.PasswordHash,
		&u.DisplayName,
		&u.AvatarURL,
		&u.Bio,
		&u.PersonalizationNote,
		&u.Chronotype,
		&u.Timezone,
		&u.ExperiencePoints,
		&u.Level,
		&u.Plan,
		&u.CreatedAt,
		&u.UpdatedAt,
		&last,
	)
	if err != nil {
		return nil, err
	}
	if last.Valid {
		t := last.Time
		u.LastLoginAt = &t
	}
	return &u, nil
}

func (r *UserRepository) Create(ctx context.Context, email, passwordHash, displayName string) (*models.User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO users (email, password_hash, display_name)
		VALUES ($1, $2, $3)
		RETURNING id, email, password_hash, display_name, avatar_url, bio,
			personalization_note, chronotype, timezone,
			experience_points, level, plan, created_at, updated_at, last_login_at
	`, email, passwordHash, displayName)
	u, err := scanUser(row)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return nil, ErrUserExists
		}
		return nil, err
	}
	return u, nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	row := r.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, display_name, avatar_url, bio,
			personalization_note, chronotype, timezone,
			experience_points, level, plan, created_at, updated_at, last_login_at
		FROM users WHERE email = $1
	`, email)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

func (r *UserRepository) FindByTelegramID(ctx context.Context, telegramID int64) (*models.User, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, display_name, avatar_url, bio,
			personalization_note, chronotype, timezone,
			experience_points, level, plan, created_at, updated_at, last_login_at
		FROM users WHERE telegram_id = $1
	`, telegramID)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

// CreateTelegram заводит пользователя, пришедшего из бота. email и password_hash в схеме
// NOT NULL, поэтому заполняются синтетически: войти паролем в такой аккаунт нельзя —
// пустой хеш не совпадёт ни с одним паролем. invitedBy и joinSource нужны для k-фактора.
// При гонке двух /start возвращает ErrUserExists — вызывающему следует перечитать по telegram_id.
func (r *UserRepository) CreateTelegram(ctx context.Context, telegramID int64, username, displayName string, invitedBy *int64, joinSource string) (*models.User, error) {
	email := fmt.Sprintf("tg%d@telegram.local", telegramID)
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO users (email, password_hash, display_name, telegram_id, telegram_username, invited_by, join_source)
		VALUES ($1, '', $2, $3, $4, $5, $6)
		RETURNING id, email, password_hash, display_name, avatar_url, bio,
			personalization_note, chronotype, timezone,
			experience_points, level, plan, created_at, updated_at, last_login_at
	`, email, displayName, telegramID, username, invitedBy, joinSource)
	u, err := scanUser(row)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return nil, ErrUserExists
		}
		return nil, err
	}
	return u, nil
}

func (r *UserRepository) FindByID(ctx context.Context, id int64) (*models.User, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash, display_name, avatar_url, bio,
			personalization_note, chronotype, timezone,
			experience_points, level, plan, created_at, updated_at, last_login_at
		FROM users WHERE id = $1
	`, id)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

// TelegramIDByUserID возвращает telegram_id пользователя; 0 — если аккаунт заведён не из бота
// (тогда написать ему в личку нельзя).
func (r *UserRepository) TelegramIDByUserID(ctx context.Context, userID int64) (int64, error) {
	var id sql.NullInt64
	err := r.db.QueryRowContext(ctx, `SELECT telegram_id FROM users WHERE id = $1`, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return id.Int64, nil
}

func (r *UserRepository) TouchLogin(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE users SET last_login_at = NOW(), updated_at = NOW() WHERE id = $1
	`, userID)
	return err
}

func (r *UserRepository) SetPlan(ctx context.Context, userID int64, plan string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE users SET plan = $2, updated_at = NOW() WHERE id = $1
	`, userID, plan)
	return err
}

func (r *UserRepository) UpdateProfile(ctx context.Context, user *models.User) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE users SET display_name = $2, bio = $3, personalization_note = $4, chronotype = $5, timezone = $6, updated_at = NOW() WHERE id = $1
	`, user.ID, user.DisplayName, user.Bio, user.PersonalizationNote, user.Chronotype, user.Timezone)
	return err
}

func (r *UserRepository) SetAvatarURL(ctx context.Context, userID int64, url string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE users SET avatar_url = $2, updated_at = NOW() WHERE id = $1
	`, userID, url)
	return err
}

// AddExperienceInTx adds delta to experience_points and sets level = 1 + floor(new_total / 100).
func (r *UserRepository) AddExperienceInTx(ctx context.Context, tx *sql.Tx, userID int64, delta int64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE users SET
			experience_points = experience_points + $2,
			level = ((experience_points + $2) / 100) + 1,
			updated_at = NOW()
		WHERE id = $1
	`, userID, delta)
	return err
}

// ExperienceLeaderboardPosition returns 1-based rank by total XP (ties share the same rank).
func (r *UserRepository) ExperienceLeaderboardPosition(ctx context.Context, userID int64) (rank int64, totalUsers int64, err error) {
	var xp int64
	err = r.db.QueryRowContext(ctx, `SELECT experience_points FROM users WHERE id = $1`, userID).Scan(&xp)
	if err != nil {
		return 0, 0, err
	}
	err = r.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*)::bigint FROM users WHERE experience_points > $1) + 1,
			(SELECT COUNT(*)::bigint FROM users)
	`, xp).Scan(&rank, &totalUsers)
	if err != nil {
		return 0, 0, err
	}
	return rank, totalUsers, nil
}
