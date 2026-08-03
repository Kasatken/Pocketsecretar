package models

import "time"

type User struct {
	ID                  int64
	Email               string
	PasswordHash        string
	DisplayName         string
	AvatarURL           string
	Bio                 string
	PersonalizationNote string
	Chronotype          string
	Timezone            string
	ExperiencePoints    int64
	Level               int
	Plan                string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	LastLoginAt         *time.Time
}
