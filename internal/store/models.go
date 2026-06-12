package store

import "time"

// Project is a voice-collection campaign with its own settings and share token.
type Project struct {
	ID             string    `json:"id"`
	Slug           string    `json:"slug"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	MaxDurationSec int       `json:"max_duration_sec"`
	MaxPerIP       int       `json:"max_per_ip"` // 0 = unlimited
	Status         string    `json:"status"`     // "open" | "closed"
	ManageToken    string    `json:"manage_token"`
	CreatedAt      time.Time `json:"created_at"`
}

// Submission is a single uploaded voice message belonging to a project.
type Submission struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	StudentID   string    `json:"student_id"`
	Nickname    string    `json:"nickname"`
	IP          string    `json:"ip"`
	UserAgent   string    `json:"user_agent"`
	DurationSec int       `json:"duration_sec"`
	FilePath    string    `json:"-"` // relative to data dir, not exposed via JSON
	MimeType    string    `json:"mime_type"`
	SizeBytes   int64     `json:"size_bytes"`
	CreatedAt   time.Time `json:"created_at"`
}

const (
	StatusOpen   = "open"
	StatusClosed = "closed"
)
