package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Space struct {
	ID         int64
	Name       string
	SpaceType  string
	Policy     string
	CreatedBy  int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
	MyRole     string
	MyShare    string
}

type SpaceMemberRow struct {
	UserID      int64
	DisplayName string
	Email       string
	Role        string
	ShareAccess string
}

type SpaceInvite struct {
	ID           int64
	SpaceID      int64
	Token        string
	InvitedEmail string
	Role         string
	ShareAccess  string
	CreatedBy    int64
	ExpiresAt    time.Time
	CreatedAt    time.Time
}

type TaskProposal struct {
	ID             int64
	SpaceID        int64
	AuthorID       int64
	AssigneeID     int64
	Title          string
	Description    string
	ProposedStart  *time.Time
	ProposedEnd    *time.Time
	Payload        json.RawMessage
	Status         string
	AssigneeNote   string
	GoogleEventID string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// AcceptedProposalCalendarOverlay — принятое предложение для отображения в календаре у исполнителя.
type AcceptedProposalCalendarOverlay struct {
	ProposalID      int64
	SpaceID         int64
	SpaceName       string
	AuthorID        int64
	AuthorDisplay   string
	AuthorAvatarURL string
	Title           string
	ProposedStart   time.Time
	ProposedEnd     time.Time
	GoogleEventID   string
}

type CollabRepository struct {
	db *sql.DB
}

func NewCollabRepository(db *sql.DB) *CollabRepository {
	return &CollabRepository{db: db}
}

func (r *CollabRepository) CreateSpace(ctx context.Context, createdBy int64, name, spaceType, policy string) (*Space, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("empty name")
	}
	if policy == "" {
		policy = "proposals_only"
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var s Space
	err = tx.QueryRowContext(ctx, `
		INSERT INTO spaces (name, space_type, policy, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, space_type, policy, created_by, created_at, updated_at
	`, name, spaceType, policy, createdBy).Scan(
		&s.ID, &s.Name, &s.SpaceType, &s.Policy, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO space_members (space_id, user_id, role, share_access)
		VALUES ($1, $2, 'coordinator', 'full')
	`, s.ID, createdBy)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	s.MyRole = "coordinator"
	s.MyShare = "full"
	return &s, nil
}

func (r *CollabRepository) ListSpacesForUser(ctx context.Context, userID int64) ([]Space, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id, s.name, s.space_type, s.policy, s.created_by, s.created_at, s.updated_at,
			m.role, m.share_access
		FROM spaces s
		INNER JOIN space_members m ON m.space_id = s.id AND m.user_id = $1
		ORDER BY s.updated_at DESC, s.id DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Space
	for rows.Next() {
		var s Space
		if err := rows.Scan(&s.ID, &s.Name, &s.SpaceType, &s.Policy, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt, &s.MyRole, &s.MyShare); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *CollabRepository) GetSpace(ctx context.Context, spaceID int64) (*Space, error) {
	var s Space
	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, space_type, policy, created_by, created_at, updated_at
		FROM spaces WHERE id = $1
	`, spaceID).Scan(&s.ID, &s.Name, &s.SpaceType, &s.Policy, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

func (r *CollabRepository) GetMembership(ctx context.Context, spaceID, userID int64) (*SpaceMemberRow, error) {
	var m SpaceMemberRow
	m.UserID = userID
	err := r.db.QueryRowContext(ctx, `
		SELECT u.display_name, u.email, sm.role, sm.share_access
		FROM space_members sm
		INNER JOIN users u ON u.id = sm.user_id
		WHERE sm.space_id = $1 AND sm.user_id = $2
	`, spaceID, userID).Scan(&m.DisplayName, &m.Email, &m.Role, &m.ShareAccess)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (r *CollabRepository) ListMembers(ctx context.Context, spaceID int64) ([]SpaceMemberRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT sm.user_id, u.display_name, u.email, sm.role, sm.share_access
		FROM space_members sm
		INNER JOIN users u ON u.id = sm.user_id
		WHERE sm.space_id = $1
		ORDER BY sm.role = 'coordinator' DESC, u.display_name, u.email
	`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SpaceMemberRow
	for rows.Next() {
		var m SpaceMemberRow
		if err := rows.Scan(&m.UserID, &m.DisplayName, &m.Email, &m.Role, &m.ShareAccess); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *CollabRepository) AddMember(ctx context.Context, spaceID, userID int64, role, shareAccess string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO space_members (space_id, user_id, role, share_access)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (space_id, user_id) DO UPDATE SET
			role = EXCLUDED.role,
			share_access = EXCLUDED.share_access
	`, spaceID, userID, role, shareAccess)
	return err
}

func (r *CollabRepository) CreateInvite(ctx context.Context, spaceID, createdBy int64, token, invitedEmail, role, shareAccess string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO space_invites (space_id, token, invited_email, role, share_access, created_by, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, spaceID, token, invitedEmail, role, shareAccess, createdBy, expiresAt)
	return err
}

func (r *CollabRepository) GetInviteByToken(ctx context.Context, token string) (*SpaceInvite, error) {
	var inv SpaceInvite
	err := r.db.QueryRowContext(ctx, `
		SELECT id, space_id, token, COALESCE(invited_email, ''), role, share_access, created_by, expires_at, created_at
		FROM space_invites WHERE token = $1
	`, strings.TrimSpace(token)).Scan(
		&inv.ID, &inv.SpaceID, &inv.Token, &inv.InvitedEmail, &inv.Role, &inv.ShareAccess,
		&inv.CreatedBy, &inv.ExpiresAt, &inv.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &inv, nil
}

func (r *CollabRepository) DeleteInvite(ctx context.Context, inviteID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM space_invites WHERE id = $1`, inviteID)
	return err
}

func (r *CollabRepository) IsMember(ctx context.Context, spaceID, userID int64) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `
		SELECT 1 FROM space_members WHERE space_id = $1 AND user_id = $2
	`, spaceID, userID).Scan(&n)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ListAcceptedProposalCalendarOverlays возвращает принятые предложения с интервалом времени,
// пересекающим [fromUTC, toUTC], для assigneeID (только свои данные).
func (r *CollabRepository) ListAcceptedProposalCalendarOverlays(ctx context.Context, assigneeID int64, fromUTC, toUTC time.Time) ([]AcceptedProposalCalendarOverlay, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT tp.id, tp.space_id, s.name,
			tp.author_id,
			COALESCE(NULLIF(TRIM(au.display_name), ''), TRIM(au.email)),
			COALESCE(au.avatar_url, ''),
			tp.title, tp.proposed_start, tp.proposed_end, COALESCE(tp.google_event_id, '')
		FROM task_proposals tp
		INNER JOIN spaces s ON s.id = tp.space_id
		INNER JOIN users au ON au.id = tp.author_id
		WHERE tp.assignee_id = $1
			AND tp.status = 'accepted'
			AND tp.proposed_start IS NOT NULL
			AND tp.proposed_end IS NOT NULL
			AND tp.proposed_start < $3
			AND tp.proposed_end > $2
		ORDER BY tp.proposed_start ASC
	`, assigneeID, fromUTC, toUTC)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AcceptedProposalCalendarOverlay
	for rows.Next() {
		var o AcceptedProposalCalendarOverlay
		if err := rows.Scan(
			&o.ProposalID, &o.SpaceID, &o.SpaceName,
			&o.AuthorID, &o.AuthorDisplay, &o.AuthorAvatarURL,
			&o.Title, &o.ProposedStart, &o.ProposedEnd, &o.GoogleEventID,
		); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// PendingProposalAssigneeSummary — пространства, где у пользователя есть ожидающие ответа предложения (он исполнитель).
type PendingProposalAssigneeSummary struct {
	SpaceID       int64
	SpaceName     string
	PendingCount  int64
	LatestTitle   string
	LatestCreated time.Time
}

func (r *CollabRepository) ListPendingProposalsForAssignee(ctx context.Context, userID int64) ([]PendingProposalAssigneeSummary, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id, s.name, COUNT(tp.id) AS cnt,
			(array_agg(tp.title ORDER BY tp.created_at DESC))[1],
			MAX(tp.created_at)
		FROM task_proposals tp
		INNER JOIN spaces s ON s.id = tp.space_id
		INNER JOIN space_members sm ON sm.space_id = tp.space_id AND sm.user_id = $1
		WHERE tp.assignee_id = $1 AND tp.status = 'pending'
		GROUP BY s.id, s.name
		ORDER BY MAX(tp.created_at) DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingProposalAssigneeSummary
	for rows.Next() {
		var row PendingProposalAssigneeSummary
		if err := rows.Scan(&row.SpaceID, &row.SpaceName, &row.PendingCount, &row.LatestTitle, &row.LatestCreated); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *CollabRepository) ListProposals(ctx context.Context, spaceID int64) ([]TaskProposal, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, space_id, author_id, assignee_id, title, description,
			proposed_start, proposed_end, payload::text, status, assignee_note, google_event_id, created_at, updated_at
		FROM task_proposals
		WHERE space_id = $1
		ORDER BY created_at DESC
	`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskProposal
	for rows.Next() {
		var p TaskProposal
		var start, end sql.NullTime
		var payloadStr string
		if err := rows.Scan(
			&p.ID, &p.SpaceID, &p.AuthorID, &p.AssigneeID, &p.Title, &p.Description,
			&start, &end, &payloadStr, &p.Status, &p.AssigneeNote, &p.GoogleEventID, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if start.Valid {
			t := start.Time
			p.ProposedStart = &t
		}
		if end.Valid {
			t := end.Time
			p.ProposedEnd = &t
		}
		p.Payload = json.RawMessage(payloadStr)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *CollabRepository) GetProposal(ctx context.Context, proposalID int64) (*TaskProposal, error) {
	var p TaskProposal
	var start, end sql.NullTime
	var payloadStr string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, space_id, author_id, assignee_id, title, description,
			proposed_start, proposed_end, payload::text, status, assignee_note, google_event_id, created_at, updated_at
		FROM task_proposals WHERE id = $1
	`, proposalID).Scan(
		&p.ID, &p.SpaceID, &p.AuthorID, &p.AssigneeID, &p.Title, &p.Description,
		&start, &end, &payloadStr, &p.Status, &p.AssigneeNote, &p.GoogleEventID, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if start.Valid {
		t := start.Time
		p.ProposedStart = &t
	}
	if end.Valid {
		t := end.Time
		p.ProposedEnd = &t
	}
	p.Payload = json.RawMessage(payloadStr)
	return &p, nil
}

func (r *CollabRepository) CreateProposal(ctx context.Context, spaceID, authorID, assigneeID int64, title, description string, proposedStart, proposedEnd *time.Time, payload json.RawMessage) (*TaskProposal, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("empty title")
	}
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	var start, end sql.NullTime
	if proposedStart != nil {
		start = sql.NullTime{Time: *proposedStart, Valid: true}
	}
	if proposedEnd != nil {
		end = sql.NullTime{Time: *proposedEnd, Valid: true}
	}
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO task_proposals (space_id, author_id, assignee_id, title, description, proposed_start, proposed_end, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
		RETURNING id
	`, spaceID, authorID, assigneeID, title, description, start, end, string(payload)).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.GetProposal(ctx, id)
}

func (r *CollabRepository) UpdateProposalResponse(ctx context.Context, proposalID, assigneeID int64, status, assigneeNote string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE task_proposals
		SET status = $1, assignee_note = $2, updated_at = NOW()
		WHERE id = $3 AND assignee_id = $4 AND status = 'pending'
	`, status, strings.TrimSpace(assigneeNote), proposalID, assigneeID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *CollabRepository) SetProposalGoogleEventID(ctx context.Context, proposalID int64, googleEventID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE task_proposals SET google_event_id = $2, updated_at = NOW() WHERE id = $1
	`, proposalID, strings.TrimSpace(googleEventID))
	return err
}

func (r *CollabRepository) TouchSpace(ctx context.Context, spaceID int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE spaces SET updated_at = NOW() WHERE id = $1`, spaceID)
	return err
}

// DeleteSpace removes the space row; dependent rows cascade (members, invites, proposals).
func (r *CollabRepository) DeleteSpace(ctx context.Context, spaceID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM spaces WHERE id = $1`, spaceID)
	return err
}
