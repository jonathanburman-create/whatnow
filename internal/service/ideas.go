package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jameynakama/whatnow/internal/domain"
)

// IdeaFields are an idea's editable fields. RawText is read only on create: it is the user's
// original words. An empty Title on create defaults to the raw text's first line.
type IdeaFields struct {
	Title   string       `json:"title"`
	Notes   string       `json:"notes"`
	Area    *domain.Area `json:"area"`
	RawText string       `json:"raw_text"`
}

func (f *IdeaFields) normalize() error {
	f.Title = strings.TrimSpace(f.Title)
	if f.Title == "" {
		f.Title = titleFromRaw(f.RawText)
	}
	if f.Title == "" {
		return fmt.Errorf("%w: title or raw_text is required", ErrInvalid)
	}
	if f.Area != nil && !f.Area.Valid() {
		return fmt.Errorf("%w: area %q", ErrInvalid, *f.Area)
	}
	return nil
}

func (f IdeaFields) applyTo(i *domain.Idea) {
	i.Title = f.Title
	i.Notes = f.Notes
	i.Area = f.Area
}

func (s *Service) CreateIdea(ctx context.Context, batch int64, f IdeaFields) (domain.Idea, error) {
	if err := f.normalize(); err != nil {
		return domain.Idea{}, err
	}
	var idea domain.Idea
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		var err error
		idea, err = s.insertIdea(ctx, tx, batch, f)
		return err
	})
	return idea, err
}

func (s *Service) insertIdea(ctx context.Context, tx *sql.Tx, batch int64, f IdeaFields) (domain.Idea, error) {
	actor, err := batchActor(ctx, tx, batch)
	if err != nil {
		return domain.Idea{}, err
	}
	now := s.now().UTC()
	idea := domain.Idea{RawText: f.RawText, CreatedBy: actor, CreatedAt: now, UpdatedAt: now}
	f.applyTo(&idea)
	res, err := tx.ExecContext(ctx,
		`INSERT INTO ideas (title, notes, area, raw_text, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		idea.Title, idea.Notes, toNull(idea.Area), idea.RawText, string(idea.CreatedBy), formatTime(idea.CreatedAt),
		formatTime(idea.UpdatedAt))
	if err != nil {
		return idea, err
	}
	if idea.ID, err = res.LastInsertId(); err != nil {
		return idea, err
	}
	return idea, record(ctx, tx, batch, actionCreate, entityIdea, idea.ID, nil, idea)
}

func (s *Service) UpdateIdea(ctx context.Context, batch, id int64, f IdeaFields) (domain.Idea, error) {
	f.RawText = ""
	if err := f.normalize(); err != nil {
		return domain.Idea{}, err
	}
	var idea domain.Idea
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		var err error
		idea, err = s.changeIdea(ctx, tx, batch, id, actionUpdate, f.applyTo)
		return err
	})
	return idea, err
}

// DeleteIdea soft-deletes an idea, which hides it everywhere. Undo brings it back.
func (s *Service) DeleteIdea(ctx context.Context, batch, id int64) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		_, err := s.changeIdea(ctx, tx, batch, id, actionDelete, s.markIdeaDeleted)
		return err
	})
}

// IdeaToTask turns an idea into an open task with the same title, notes, area, and raw text, and
// takes the idea off the pile, all in one batch, so one undo puts the idea back.
func (s *Service) IdeaToTask(ctx context.Context, batch, id int64) (domain.Task, error) {
	var t domain.Task
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		idea, err := s.changeIdea(ctx, tx, batch, id, actionDelete, s.markIdeaDeleted)
		if err != nil {
			return err
		}
		actor, err := batchActor(ctx, tx, batch)
		if err != nil {
			return err
		}
		now := s.now().UTC()
		t = domain.Task{RawText: idea.RawText, Status: domain.StatusOpen, CreatedBy: actor, CreatedAt: now, UpdatedAt: now}
		f := TaskFields{Title: idea.Title, Notes: idea.Notes, Area: idea.Area}
		if err := f.normalize(); err != nil {
			return err
		}
		f.applyTo(&t)
		if err := insertTask(ctx, tx, &t); err != nil {
			return err
		}
		return record(ctx, tx, batch, actionCreate, entityTask, t.ID, nil, t)
	})
	return t, err
}

func (s *Service) markIdeaDeleted(i *domain.Idea) {
	deleted := s.now().UTC()
	i.DeletedAt = &deleted
}

func (s *Service) changeIdea(ctx context.Context, tx *sql.Tx, batch, id int64, action string, change func(*domain.Idea)) (domain.Idea, error) {
	if _, err := batchActor(ctx, tx, batch); err != nil {
		return domain.Idea{}, err
	}
	before, err := liveIdea(ctx, tx, id)
	if err != nil {
		return domain.Idea{}, err
	}
	idea := before
	change(&idea)
	idea.UpdatedAt = s.now().UTC()
	if err := writeIdea(ctx, tx, idea); err != nil {
		return idea, err
	}
	return idea, record(ctx, tx, batch, action, entityIdea, id, before, idea)
}

func (s *Service) GetIdea(ctx context.Context, id int64) (domain.Idea, error) {
	return liveIdea(ctx, s.db, id)
}

// ListIdeas returns every idea that isn't deleted, newest first.
func (s *Service) ListIdeas(ctx context.Context) ([]domain.Idea, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+ideaColumns+` FROM ideas WHERE deleted_at IS NULL ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ideas := []domain.Idea{}
	for rows.Next() {
		i, err := scanIdea(rows)
		if err != nil {
			return nil, err
		}
		ideas = append(ideas, i)
	}
	return ideas, rows.Err()
}

const ideaColumns = `id, title, notes, area, raw_text, created_by, created_at, updated_at, deleted_at`

func liveIdea(ctx context.Context, q querier, id int64) (domain.Idea, error) {
	i, err := scanIdea(q.QueryRowContext(ctx, `SELECT `+ideaColumns+` FROM ideas WHERE id = ? AND deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return i, fmt.Errorf("%w: idea %d", ErrNotFound, id)
	}
	return i, err
}

func scanIdea(row scanner) (domain.Idea, error) {
	var i domain.Idea
	var area, deletedAt sql.NullString
	var createdAt, updatedAt string
	err := row.Scan(&i.ID, &i.Title, &i.Notes, &area, &i.RawText, &i.CreatedBy, &createdAt, &updatedAt, &deletedAt)
	if err != nil {
		return i, err
	}
	i.Area = fromNull[domain.Area](area)
	if i.CreatedAt, err = parseTime(createdAt); err != nil {
		return i, err
	}
	if i.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return i, err
	}
	i.DeletedAt, err = parseNullTime(deletedAt)
	return i, err
}

// writeIdea overwrites every column of an existing idea. Undo uses it to restore a snapshot.
func writeIdea(ctx context.Context, tx *sql.Tx, i domain.Idea) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE ideas SET title = ?, notes = ?, area = ?, raw_text = ?, created_by = ?, created_at = ?, updated_at = ?,
		 deleted_at = ? WHERE id = ?`,
		i.Title, i.Notes, toNull(i.Area), i.RawText, string(i.CreatedBy), formatTime(i.CreatedAt), formatTime(i.UpdatedAt),
		formatNullTime(i.DeletedAt), i.ID)
	return err
}
