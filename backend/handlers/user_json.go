package handlers

import (
	"time"

	"pocketsecret/models"
)

func publicUserJSON(u *models.User) map[string]interface{} {
	out := map[string]interface{}{
		"id":                    u.ID,
		"email":                 u.Email,
		"display_name":          u.DisplayName,
		"avatar_url":            u.AvatarURL,
		"bio":                   u.Bio,
		"personalization_note":  u.PersonalizationNote,
		"chronotype":            u.Chronotype,
		"experience_points":     u.ExperiencePoints,
		"level":                 u.Level,
		"plan":                  u.Plan,
		"created_at":            u.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":            u.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if u.LastLoginAt != nil {
		out["last_login_at"] = u.LastLoginAt.UTC().Format(time.RFC3339)
	} else {
		out["last_login_at"] = nil
	}
	return out
}
