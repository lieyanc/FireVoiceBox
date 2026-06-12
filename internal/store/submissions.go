package store

import (
	"database/sql"
	"errors"
	"time"
)

const submissionCols = "id, project_id, student_id, nickname, ip, user_agent, duration_sec, file_path, mime_type, size_bytes, created_at"

func scanSubmission(s rowScanner) (*Submission, error) {
	var sub Submission
	var created int64
	if err := s.Scan(&sub.ID, &sub.ProjectID, &sub.StudentID, &sub.Nickname, &sub.IP,
		&sub.UserAgent, &sub.DurationSec, &sub.FilePath, &sub.MimeType, &sub.SizeBytes, &created); err != nil {
		return nil, err
	}
	sub.CreatedAt = time.Unix(created, 0)
	return &sub, nil
}

// CreateSubmission inserts a new submission. CreatedAt defaults to now.
func (s *Store) CreateSubmission(sub *Submission) error {
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = time.Now()
	}
	_, err := s.db.Exec(
		`INSERT INTO submissions (`+submissionCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		sub.ID, sub.ProjectID, sub.StudentID, sub.Nickname, sub.IP, sub.UserAgent,
		sub.DurationSec, sub.FilePath, sub.MimeType, sub.SizeBytes, sub.CreatedAt.Unix(),
	)
	return err
}

// GetSubmission looks up a submission by ID.
func (s *Store) GetSubmission(id string) (*Submission, error) {
	row := s.db.QueryRow(`SELECT `+submissionCols+` FROM submissions WHERE id = ?`, id)
	sub, err := scanSubmission(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return sub, err
}

// ListSubmissions returns submissions for a project, newest first.
func (s *Store) ListSubmissions(projectID string) ([]*Submission, error) {
	rows, err := s.db.Query(`SELECT `+submissionCols+` FROM submissions WHERE project_id = ? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Submission
	for rows.Next() {
		sub, err := scanSubmission(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

// CountSubmissionsByIP returns how many submissions an IP has made to a project.
func (s *Store) CountSubmissionsByIP(projectID, ip string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM submissions WHERE project_id = ? AND ip = ?`, projectID, ip).Scan(&n)
	return n, err
}

// DeleteSubmission removes a submission row and returns it so the caller can
// delete the associated audio file.
func (s *Store) DeleteSubmission(id string) (*Submission, error) {
	sub, err := s.GetSubmission(id)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`DELETE FROM submissions WHERE id = ?`, id); err != nil {
		return nil, err
	}
	return sub, nil
}
