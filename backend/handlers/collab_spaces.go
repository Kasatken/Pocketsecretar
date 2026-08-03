package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/oauth2"

	"pocketsecret/config"
	"pocketsecret/middleware"
	"pocketsecret/repository"
)

type CollabHandler struct {
	cfg    config.Config
	google *repository.GoogleCalendarRepository
	users  *repository.UserRepository
	collab *repository.CollabRepository
}

func NewCollabHandler(cfg config.Config, google *repository.GoogleCalendarRepository, users *repository.UserRepository, collab *repository.CollabRepository) *CollabHandler {
	return &CollabHandler{cfg: cfg, google: google, users: users, collab: collab}
}

func (h *CollabHandler) googleConfigured() bool {
	return strings.TrimSpace(h.cfg.GoogleClientID) != "" &&
		strings.TrimSpace(h.cfg.GoogleClientSecret) != "" &&
		strings.TrimSpace(h.cfg.GoogleRedirectURI) != ""
}

func randomInviteToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func spaceToJSON(s *repository.Space) map[string]interface{} {
	if s == nil {
		return nil
	}
	return map[string]interface{}{
		"id":          s.ID,
		"name":        s.Name,
		"space_type":  s.SpaceType,
		"policy":      s.Policy,
		"created_by":  s.CreatedBy,
		"created_at":  s.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":  s.UpdatedAt.UTC().Format(time.RFC3339),
		"my_role":     s.MyRole,
		"my_share":    s.MyShare,
	}
}

func memberToJSON(m repository.SpaceMemberRow) map[string]interface{} {
	return map[string]interface{}{
		"user_id":      m.UserID,
		"display_name": strings.TrimSpace(m.DisplayName),
		"email":        m.Email,
		"role":         m.Role,
		"share_access": m.ShareAccess,
	}
}

func proposalToJSON(p *repository.TaskProposal) map[string]interface{} {
	if p == nil {
		return nil
	}
	out := map[string]interface{}{
		"id":               p.ID,
		"space_id":         p.SpaceID,
		"author_id":        p.AuthorID,
		"assignee_id":      p.AssigneeID,
		"title":            p.Title,
		"description":      p.Description,
		"status":           p.Status,
		"assignee_note":    p.AssigneeNote,
		"google_event_id": p.GoogleEventID,
		"created_at":       p.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":      p.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if len(p.Payload) > 0 {
		out["payload"] = json.RawMessage(p.Payload)
	} else {
		out["payload"] = map[string]interface{}{}
	}
	if p.ProposedStart != nil {
		out["proposed_start"] = p.ProposedStart.UTC().Format(time.RFC3339)
	}
	if p.ProposedEnd != nil {
		out["proposed_end"] = p.ProposedEnd.UTC().Format(time.RFC3339)
	}
	return out
}

func parseSpaceID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := strings.TrimSpace(r.PathValue("id"))
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing space id"})
		return 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid space id"})
		return 0, false
	}
	return id, true
}

func parseProposalID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := strings.TrimSpace(r.PathValue("proposalId"))
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing proposal id"})
		return 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid proposal id"})
		return 0, false
	}
	return id, true
}

func acceptedOverlayToJSON(o repository.AcceptedProposalCalendarOverlay) map[string]interface{} {
	return map[string]interface{}{
		"proposal_id":         o.ProposalID,
		"space_id":          o.SpaceID,
		"space_name":        o.SpaceName,
		"author_id":         o.AuthorID,
		"author_display_name": o.AuthorDisplay,
		"author_avatar_url":   o.AuthorAvatarURL,
		"title":               o.Title,
		"proposed_start":      o.ProposedStart.UTC().Format(time.RFC3339),
		"proposed_end":        o.ProposedEnd.UTC().Format(time.RFC3339),
		"google_event_id":     o.GoogleEventID,
	}
}

// ListCalendarAcceptedOverlays GET /api/spaces/calendar-overlays?time_min=&time_max=
func (h *CollabHandler) ListCalendarAcceptedOverlays(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	q := r.URL.Query()
	tmin := strings.TrimSpace(q.Get("time_min"))
	tmax := strings.TrimSpace(q.Get("time_max"))
	if tmin == "" || tmax == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "time_min and time_max required (RFC3339)"})
		return
	}
	fromUTC, err1 := time.Parse(time.RFC3339, tmin)
	toUTC, err2 := time.Parse(time.RFC3339, tmax)
	if err1 != nil || err2 != nil || !toUTC.After(fromUTC) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid time range"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	rows, err := h.collab.ListAcceptedProposalCalendarOverlays(ctx, userID, fromUTC, toUTC)
	if err != nil {
		log.Printf("collab calendar overlays: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list overlays"})
		return
	}
	out := make([]map[string]interface{}, 0, len(rows))
	for i := range rows {
		out = append(out, acceptedOverlayToJSON(rows[i]))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"overlays": out})
}

// ListPendingProposalsForMe GET /api/spaces/pending-for-me — ожидающие ответа предложения, где текущий пользователь — исполнитель.
func (h *CollabHandler) ListPendingProposalsForMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	rows, err := h.collab.ListPendingProposalsForAssignee(ctx, userID)
	if err != nil {
		log.Printf("collab pending proposals for me: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list pending proposals"})
		return
	}
	var total int64
	spaces := make([]map[string]interface{}, 0, len(rows))
	for i := range rows {
		total += rows[i].PendingCount
		spaces = append(spaces, map[string]interface{}{
			"space_id":        rows[i].SpaceID,
			"space_name":      rows[i].SpaceName,
			"pending_count":   rows[i].PendingCount,
			"latest_title":    rows[i].LatestTitle,
			"latest_created":  rows[i].LatestCreated.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total":  total,
		"spaces": spaces,
	})
}

// ListSpaces GET /api/spaces
func (h *CollabHandler) ListSpaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	rows, err := h.collab.ListSpacesForUser(ctx, userID)
	if err != nil {
		log.Printf("collab list spaces: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list spaces"})
		return
	}
	out := make([]map[string]interface{}, 0, len(rows))
	for i := range rows {
		out = append(out, spaceToJSON(&rows[i]))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"spaces": out})
}

type createSpaceBody struct {
	Name      string `json:"name"`
	SpaceType string `json:"space_type"`
	Policy    string `json:"policy"`
}

// CreateSpace POST /api/spaces
func (h *CollabHandler) CreateSpace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body createSpaceBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	st := strings.TrimSpace(strings.ToLower(body.SpaceType))
	if st == "" {
		st = "custom"
	}
	if st != "work" && st != "personal" && st != "custom" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid space_type"})
		return
	}
	policy := strings.TrimSpace(body.Policy)
	if policy == "" {
		policy = "proposals_only"
	}
	if policy != "proposals_only" && policy != "direct_ok" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid policy"})
		return
	}
	if utf8.RuneCountInString(strings.TrimSpace(body.Name)) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name too long"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	s, err := h.collab.CreateSpace(ctx, userID, strings.TrimSpace(body.Name), st, policy)
	if err != nil {
		if err.Error() == "empty name" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
			return
		}
		log.Printf("collab create space: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create space"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"space": spaceToJSON(s)})
}

// GetSpace GET /api/spaces/{id}
func (h *CollabHandler) GetSpace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	spaceID, ok2 := parseSpaceID(w, r)
	if !ok2 {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	mem, err := h.collab.GetMembership(ctx, spaceID, userID)
	if err != nil {
		log.Printf("collab membership: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load membership"})
		return
	}
	if mem == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "space not found"})
		return
	}
	s, err := h.collab.GetSpace(ctx, spaceID)
	if err != nil {
		log.Printf("collab get space: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load space"})
		return
	}
	if s == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "space not found"})
		return
	}
	s.MyRole = mem.Role
	s.MyShare = mem.ShareAccess
	writeJSON(w, http.StatusOK, map[string]interface{}{"space": spaceToJSON(s)})
}

// DeleteSpace DELETE /api/spaces/{id} — only the user who created the space (created_by).
func (h *CollabHandler) DeleteSpace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	spaceID, ok2 := parseSpaceID(w, r)
	if !ok2 {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	s, err := h.collab.GetSpace(ctx, spaceID)
	if err != nil {
		log.Printf("collab get space: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load space"})
		return
	}
	if s == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "space not found"})
		return
	}
	if s.CreatedBy != userID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only the creator can delete this space"})
		return
	}
	if err := h.collab.DeleteSpace(ctx, spaceID); err != nil {
		log.Printf("collab delete space: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete space"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// ListMembers GET /api/spaces/{id}/members
func (h *CollabHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	spaceID, ok2 := parseSpaceID(w, r)
	if !ok2 {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	mem, err := h.collab.GetMembership(ctx, spaceID, userID)
	if err != nil {
		log.Printf("collab list members get membership: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if mem == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "space not found"})
		return
	}
	rows, err := h.collab.ListMembers(ctx, spaceID)
	if err != nil {
		log.Printf("collab list members: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list members"})
		return
	}
	out := make([]map[string]interface{}, 0, len(rows))
	for i := range rows {
		out = append(out, memberToJSON(rows[i]))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"members": out})
}

type createInviteBody struct {
	InvitedEmail string `json:"invited_email"`
	Role         string `json:"role"`
	ShareAccess  string `json:"share_access"`
	ExpiresDays  int    `json:"expires_days"`
}

// CreateInvite POST /api/spaces/{id}/invites
func (h *CollabHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	spaceID, ok2 := parseSpaceID(w, r)
	if !ok2 {
		return
	}
	var body createInviteBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	role := strings.TrimSpace(body.Role)
	if role == "" {
		role = "member"
	}
	if role != "coordinator" && role != "member" && role != "observer" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid role"})
		return
	}
	share := strings.TrimSpace(body.ShareAccess)
	if share == "" {
		share = "partial"
	}
	if share != "full" && share != "partial" && share != "view_only" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid share_access"})
		return
	}
	days := body.ExpiresDays
	if days <= 0 {
		days = 7
	}
	if days > 30 {
		days = 30
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	mem, err := h.collab.GetMembership(ctx, spaceID, userID)
	if err != nil {
		log.Printf("collab create invite get membership: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if mem == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "space not found"})
		return
	}
	if mem.Role != "coordinator" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only coordinators can invite"})
		return
	}
	token, err := randomInviteToken()
	if err != nil {
		log.Printf("collab token: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create invite"})
		return
	}
	invEmail := strings.TrimSpace(strings.ToLower(body.InvitedEmail))
	expires := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)
	if err := h.collab.CreateInvite(ctx, spaceID, userID, token, invEmail, role, share, expires); err != nil {
		log.Printf("collab create invite: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create invite"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":      token,
		"expires_at": expires.UTC().Format(time.RFC3339),
	})
}

type joinSpaceBody struct {
	Token string `json:"token"`
}

// JoinSpace POST /api/spaces/join
func (h *CollabHandler) JoinSpace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body joinSpaceBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	token := strings.TrimSpace(body.Token)
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token required"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	inv, err := h.collab.GetInviteByToken(ctx, token)
	if err != nil {
		log.Printf("collab get invite: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read invite"})
		return
	}
	if inv == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invalid invite"})
		return
	}
	if time.Now().UTC().After(inv.ExpiresAt.UTC()) {
		writeJSON(w, http.StatusGone, map[string]string{"error": "invite expired"})
		return
	}
	u, err := h.users.FindByID(ctx, userID)
	if err != nil || u == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user not found"})
		return
	}
	if inv.InvitedEmail != "" && !strings.EqualFold(strings.TrimSpace(u.Email), inv.InvitedEmail) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "this invite is for another email"})
		return
	}
	// Уже участник (повторный клик по той же ссылке) — отвечаем успехом и не сжигаем инвайт.
	if already, e := h.collab.IsMember(ctx, inv.SpaceID, userID); e == nil && already {
		s, sErr := h.collab.GetSpace(ctx, inv.SpaceID)
		if sErr != nil || s == nil {
			log.Printf("collab join get space (already member): %v", sErr)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load space"})
			return
		}
		if mem, _ := h.collab.GetMembership(ctx, inv.SpaceID, userID); mem != nil {
			s.MyRole = mem.Role
			s.MyShare = mem.ShareAccess
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"space": spaceToJSON(s), "already_member": true})
		return
	}
	if err := h.collab.AddMember(ctx, inv.SpaceID, userID, inv.Role, inv.ShareAccess); err != nil {
		log.Printf("collab add member: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to join"})
		return
	}
	_ = h.collab.TouchSpace(ctx, inv.SpaceID)
	s, err := h.collab.GetSpace(ctx, inv.SpaceID)
	if err != nil || s == nil {
		log.Printf("collab join get space: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "joined but failed to load space"})
		return
	}
	mem, _ := h.collab.GetMembership(ctx, inv.SpaceID, userID)
	if mem != nil {
		s.MyRole = mem.Role
		s.MyShare = mem.ShareAccess
	}
	if err := h.collab.DeleteInvite(ctx, inv.ID); err != nil {
		log.Printf("collab delete invite: %v", err)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"space": spaceToJSON(s)})
}

// ListProposals GET /api/spaces/{id}/proposals
func (h *CollabHandler) ListProposals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	spaceID, ok2 := parseSpaceID(w, r)
	if !ok2 {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	mem, err := h.collab.GetMembership(ctx, spaceID, userID)
	if err != nil {
		log.Printf("collab list proposals get membership: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if mem == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "space not found"})
		return
	}
	rows, err := h.collab.ListProposals(ctx, spaceID)
	if err != nil {
		log.Printf("collab list proposals: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list proposals"})
		return
	}
	out := make([]map[string]interface{}, 0, len(rows))
	for i := range rows {
		out = append(out, proposalToJSON(&rows[i]))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"proposals": out})
}

type createProposalBody struct {
	AssigneeID    int64           `json:"assignee_id"`
	Title         string          `json:"title"`
	Description   string          `json:"description"`
	ProposedStart string          `json:"proposed_start"`
	ProposedEnd   string          `json:"proposed_end"`
	Payload       json.RawMessage `json:"payload"`
}

// CreateProposal POST /api/spaces/{id}/proposals
func (h *CollabHandler) CreateProposal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	spaceID, ok2 := parseSpaceID(w, r)
	if !ok2 {
		return
	}
	var body createProposalBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if body.AssigneeID < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "assignee_id required"})
		return
	}
	if utf8.RuneCountInString(strings.TrimSpace(body.Title)) > 500 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title too long"})
		return
	}
	if utf8.RuneCountInString(body.Description) > 8000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "description too long"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	mem, err := h.collab.GetMembership(ctx, spaceID, userID)
	if err != nil {
		log.Printf("collab create proposal get membership: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if mem == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "space not found"})
		return
	}
	if mem.Role == "observer" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "observers cannot create proposals"})
		return
	}
	okAssignee, err := h.collab.IsMember(ctx, spaceID, body.AssigneeID)
	if err != nil {
		log.Printf("collab create proposal is member: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if !okAssignee {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "assignee must be a member of the space"})
		return
	}
	var tStart, tEnd *time.Time
	if strings.TrimSpace(body.ProposedStart) != "" {
		ts, e := time.Parse(time.RFC3339, strings.TrimSpace(body.ProposedStart))
		if e != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid proposed_start"})
			return
		}
		tStart = &ts
	}
	if strings.TrimSpace(body.ProposedEnd) != "" {
		te, e := time.Parse(time.RFC3339, strings.TrimSpace(body.ProposedEnd))
		if e != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid proposed_end"})
			return
		}
		tEnd = &te
	}
	if (tStart == nil) != (tEnd == nil) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "proposed_start and proposed_end must both be set or both empty"})
		return
	}
	p, err := h.collab.CreateProposal(ctx, spaceID, userID, body.AssigneeID, body.Title, body.Description, tStart, tEnd, body.Payload)
	if err != nil {
		if err.Error() == "empty title" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title required"})
			return
		}
		log.Printf("collab create proposal: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create proposal"})
		return
	}
	_ = h.collab.TouchSpace(ctx, spaceID)
	writeJSON(w, http.StatusOK, map[string]interface{}{"proposal": proposalToJSON(p)})
}

type respondProposalBody struct {
	Status        string `json:"status"`
	AssigneeNote  string `json:"assignee_note"`
}

// RespondProposal POST /api/spaces/{id}/proposals/{proposalId}/respond
func (h *CollabHandler) RespondProposal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	spaceID, ok2 := parseSpaceID(w, r)
	if !ok2 {
		return
	}
	propID, ok3 := parseProposalID(w, r)
	if !ok3 {
		return
	}
	var body respondProposalBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	st := strings.TrimSpace(strings.ToLower(body.Status))
	if st != "accepted" && st != "declined" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status must be accepted or declined"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	inSpace, err := h.collab.IsMember(ctx, spaceID, userID)
	if err != nil {
		log.Printf("collab respond proposal is member: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if !inSpace {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "space not found"})
		return
	}
	p, err := h.collab.GetProposal(ctx, propID)
	if err != nil || p == nil || p.SpaceID != spaceID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "proposal not found"})
		return
	}
	if p.AssigneeID != userID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only the assignee can respond"})
		return
	}
	if p.Status != "pending" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "proposal is no longer pending"})
		return
	}
	okUpd, err := h.collab.UpdateProposalResponse(ctx, propID, userID, st, body.AssigneeNote)
	if err != nil {
		log.Printf("collab respond: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update proposal"})
		return
	}
	if !okUpd {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "could not update proposal"})
		return
	}
	resp := map[string]interface{}{"ok": true, "status": st}
	if st == "accepted" && p.ProposedStart != nil && p.ProposedEnd != nil && h.googleConfigured() {
		refresh, err := h.google.RefreshTokenByUserID(ctx, userID)
		if err != nil || refresh == "" {
			resp["calendar_warning"] = "Подключите Google Календарь — событие не создано автоматически"
		} else {
			oauthCfg := newGoogleOAuthConfig(h.cfg)
			ts := oauthCfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refresh})
			httpClient := oauth2.NewClient(ctx, ts)
			desc := strings.TrimSpace(p.Description)
			if desc == "" {
				desc = "Предложение из пространства"
			}
			evID, calErr := createGoogleCalendarInstantRangeEvent(ctx, httpClient, p.Title, desc, defaultBacklogEventTZ, *p.ProposedStart, *p.ProposedEnd, propID)
			if calErr != nil {
				log.Printf("collab gcal on accept: %v", calErr)
				resp["calendar_warning"] = "Не удалось создать событие в календаре"
			} else if err := h.collab.SetProposalGoogleEventID(ctx, propID, evID); err != nil {
				log.Printf("collab save event id: %v", err)
			} else {
				resp["google_event_id"] = evID
			}
		}
	}
	_ = h.collab.TouchSpace(ctx, spaceID)
	p2, _ := h.collab.GetProposal(ctx, propID)
	if p2 != nil {
		resp["proposal"] = proposalToJSON(p2)
	}
	writeJSON(w, http.StatusOK, resp)
}
