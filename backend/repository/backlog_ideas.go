package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const backlogIdeasMaxPerUser = 200

type BacklogIdea struct {
	ID        string
	UserID    int64
	Body      string
	ColorID   string
	Tag       string
	CreatedAt time.Time
}

type BacklogIdeaRepository struct {
	db *sql.DB
}

func NewBacklogIdeaRepository(db *sql.DB) *BacklogIdeaRepository {
	return &BacklogIdeaRepository{db: db}
}

func (r *BacklogIdeaRepository) CountByUser(ctx context.Context, userID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM user_backlog_idea WHERE user_id = $1
	`, userID).Scan(&n)
	return n, err
}

func (r *BacklogIdeaRepository) ListByUser(ctx context.Context, userID int64) ([]BacklogIdea, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id::text, user_id, body, color_id, tag, created_at
		FROM user_backlog_idea
		WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BacklogIdea
	for rows.Next() {
		var row BacklogIdea
		if err := rows.Scan(&row.ID, &row.UserID, &row.Body, &row.ColorID, &row.Tag, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *BacklogIdeaRepository) Create(ctx context.Context, userID int64, body, colorID, tag string) (*BacklogIdea, error) {
	n, err := r.CountByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if n >= backlogIdeasMaxPerUser {
		return nil, ErrBacklogLimitReached
	}

	var row BacklogIdea
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO user_backlog_idea (user_id, body, color_id, tag)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, user_id, body, color_id, tag, created_at
	`, userID, body, colorID, tag).Scan(&row.ID, &row.UserID, &row.Body, &row.ColorID, &row.Tag, &row.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// GetOwned returns the idea if it belongs to userID.
func (r *BacklogIdeaRepository) GetOwned(ctx context.Context, userID int64, id string) (*BacklogIdea, error) {
	var row BacklogIdea
	err := r.db.QueryRowContext(ctx, `
		SELECT id::text, user_id, body, color_id, tag, created_at
		FROM user_backlog_idea
		WHERE user_id = $1 AND id = $2::uuid
	`, userID, id).Scan(&row.ID, &row.UserID, &row.Body, &row.ColorID, &row.Tag, &row.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func (r *BacklogIdeaRepository) Delete(ctx context.Context, userID int64, id string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM user_backlog_idea WHERE user_id = $1 AND id = $2::uuid
	`, userID, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

var ErrBacklogLimitReached = errors.New("backlog idea limit reached")
