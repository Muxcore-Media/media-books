package internal

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func (s *Store) CreateTag(ctx context.Context, label string) (string, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return "", fmt.Errorf("label required")
	}
	id := "tag_" + uuid.NewString()[:8]
	_, err := s.db.ExecContext(ctx, `INSERT INTO tags (id, label, created_at) VALUES (?, ?, ?)`, id, label, nowRFC3339())
	if err != nil {
		return "", fmt.Errorf("create tag: %w", err)
	}
	return id, nil
}

func (s *Store) DeleteTag(ctx context.Context, id string) error {
	_, _ = s.db.ExecContext(ctx, `DELETE FROM item_tags WHERE tag_id = ?`, id)
	res, err := s.db.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("tag %q not found", id)
	}
	return nil
}

func (s *Store) ListTags(ctx context.Context) ([]*Tag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, label, created_at FROM tags ORDER BY label`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]*Tag, 0)
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Label, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

func (s *Store) GetItemTags(ctx context.Context, itemID string) ([]*Tag, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return nil, fmt.Errorf("item_id required")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.label, t.created_at FROM tags t
		INNER JOIN item_tags it ON it.tag_id = t.id
		WHERE it.item_id = ? ORDER BY t.label
	`, itemID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]*Tag, 0)
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Label, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

func (s *Store) SetItemTags(ctx context.Context, itemID string, tagIDs []string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM item_tags WHERE item_id = ?`, itemID); err != nil {
		return err
	}
	for _, tid := range tagIDs {
		if tid == "" {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO item_tags (item_id, tag_id) VALUES (?, ?)`, itemID, tid); err != nil {
			return err
		}
	}
	return nil
}
