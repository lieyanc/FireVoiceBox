package store

import (
	"database/sql"
	"errors"
	"time"
)

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("not found")

const projectCols = "id, slug, title, description, max_duration_sec, max_per_ip, status, manage_token, created_at"

func scanProject(s rowScanner) (*Project, error) {
	var p Project
	var created int64
	if err := s.Scan(&p.ID, &p.Slug, &p.Title, &p.Description, &p.MaxDurationSec,
		&p.MaxPerIP, &p.Status, &p.ManageToken, &created); err != nil {
		return nil, err
	}
	p.CreatedAt = time.Unix(created, 0)
	return &p, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

// CreateProject inserts a new project. ID, ManageToken and CreatedAt are
// expected to be populated by the caller; Slug defaults to ID when empty.
func (s *Store) CreateProject(p *Project) error {
	if p.Slug == "" {
		p.Slug = p.ID
	}
	if p.Status == "" {
		p.Status = StatusOpen
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	_, err := s.db.Exec(
		`INSERT INTO projects (`+projectCols+`) VALUES (?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Slug, p.Title, p.Description, p.MaxDurationSec, p.MaxPerIP,
		p.Status, p.ManageToken, p.CreatedAt.Unix(),
	)
	return err
}

// GetProject looks up a project by its ID.
func (s *Store) GetProject(id string) (*Project, error) {
	row := s.db.QueryRow(`SELECT `+projectCols+` FROM projects WHERE id = ?`, id)
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// GetProjectByIDOrSlug resolves a project by ID first, then by slug.
func (s *Store) GetProjectByIDOrSlug(key string) (*Project, error) {
	row := s.db.QueryRow(`SELECT `+projectCols+` FROM projects WHERE id = ? OR slug = ? LIMIT 1`, key, key)
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// ListProjects returns all projects, newest first.
func (s *Store) ListProjects() ([]*Project, error) {
	rows, err := s.db.Query(`SELECT ` + projectCols + ` FROM projects ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdateProject persists mutable fields of an existing project.
func (s *Store) UpdateProject(p *Project) error {
	res, err := s.db.Exec(
		`UPDATE projects SET slug=?, title=?, description=?, max_duration_sec=?,
		 max_per_ip=?, status=? WHERE id=?`,
		p.Slug, p.Title, p.Description, p.MaxDurationSec, p.MaxPerIP, p.Status, p.ID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteProject removes a project; submissions cascade via FK.
func (s *Store) DeleteProject(id string) error {
	res, err := s.db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountSubmissions returns the number of submissions for a project.
func (s *Store) CountSubmissions(projectID string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM submissions WHERE project_id = ?`, projectID).Scan(&n)
	return n, err
}
