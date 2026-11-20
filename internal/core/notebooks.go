package core

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/xiaotuantuan5/blog/internal/db"
)

type Notebook struct {
	ID        int64   `json:"id"`
	ParentID  *int64  `json:"parentId"`
	Name      string  `json:"name"`
	Sort      int     `json:"sort"`
	NoteCount int     `json:"noteCount"`
	CreatedAt db.Time `json:"createdAt"`
}

type NotebookInput struct {
	ParentID *int64 `json:"parentId"`
	Name     string `json:"name"`
	Sort     int    `json:"sort"`
}

// ListNotebooks returns all notebooks as a flat list; the client builds the tree.
func (a *App) ListNotebooks(ctx context.Context) ([]Notebook, error) {
	rows, err := a.DB.QueryContext(ctx, `
		SELECT nb.id, nb.parent_id, nb.name, nb.sort, nb.created_at,
		       (SELECT COUNT(*) FROM notes n WHERE n.notebook_id = nb.id AND n.deleted_at IS NULL)
		FROM notebooks nb ORDER BY nb.sort, nb.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Notebook{}
	for rows.Next() {
		var nb Notebook
		if err := rows.Scan(&nb.ID, &nb.ParentID, &nb.Name, &nb.Sort, &nb.CreatedAt, &nb.NoteCount); err != nil {
			return nil, err
		}
		list = append(list, nb)
	}
	return list, rows.Err()
}

func (a *App) getNotebook(ctx context.Context, q querier, id int64) (Notebook, error) {
	var nb Notebook
	err := q.QueryRowContext(ctx, `SELECT id, parent_id, name, sort, created_at FROM notebooks WHERE id = ?`, id).
		Scan(&nb.ID, &nb.ParentID, &nb.Name, &nb.Sort, &nb.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nb, ErrNotFound
	}
	return nb, err
}

func cleanNotebookInput(in *NotebookInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return invalid("笔记本名称不能为空")
	}
	if utf8.RuneCountInString(in.Name) > 100 {
		return invalid("笔记本名称太长")
	}
	return nil
}

func (a *App) CreateNotebook(ctx context.Context, in NotebookInput) (Notebook, error) {
	if err := cleanNotebookInput(&in); err != nil {
		return Notebook{}, err
	}
	var nb Notebook
	err := a.inTx(ctx, func(tx *sql.Tx) error {
		if in.ParentID != nil {
			if _, err := a.getNotebook(ctx, tx, *in.ParentID); err != nil {
				return invalid("上级笔记本不存在")
			}
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO notebooks (parent_id, name, sort, created_at) VALUES (?, ?, ?, ?)`,
			in.ParentID, in.Name, in.Sort, db.Now())
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		nb, err = a.getNotebook(ctx, tx, id)
		return err
	})
	return nb, err
}

func (a *App) UpdateNotebook(ctx context.Context, id int64, in NotebookInput) (Notebook, error) {
	if err := cleanNotebookInput(&in); err != nil {
		return Notebook{}, err
	}
	var nb Notebook
	err := a.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := a.getNotebook(ctx, tx, id); err != nil {
			return err
		}
		// Walk up from the new parent; reaching id would create a cycle.
		for p := in.ParentID; p != nil; {
			if *p == id {
				return invalid("不能把笔记本移动到它自己或它的子笔记本下")
			}
			parent, err := a.getNotebook(ctx, tx, *p)
			if err != nil {
				return invalid("上级笔记本不存在")
			}
			p = parent.ParentID
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE notebooks SET parent_id = ?, name = ?, sort = ? WHERE id = ?`,
			in.ParentID, in.Name, in.Sort, id); err != nil {
			return err
		}
		var err error
		nb, err = a.getNotebook(ctx, tx, id)
		return err
	})
	return nb, err
}

// DeleteNotebook deletes an empty notebook.
func (a *App) DeleteNotebook(ctx context.Context, id int64) error {
	return a.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := a.getNotebook(ctx, tx, id); err != nil {
			return err
		}
		var children, notes int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM notebooks WHERE parent_id = ?`, id).Scan(&children); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM notes WHERE notebook_id = ? AND deleted_at IS NULL`, id).Scan(&notes); err != nil {
			return err
		}
		if children > 0 || notes > 0 {
			return conflict("笔记本不为空，请先移走其中的笔记和子笔记本")
		}
		// Trashed notes detach to "no notebook"; restoring lands them there.
		if _, err := tx.ExecContext(ctx,
			`UPDATE notes SET notebook_id = NULL WHERE notebook_id = ? AND deleted_at IS NOT NULL`, id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM notebooks WHERE id = ?`, id)
		return err
	})
}
