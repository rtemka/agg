package memdb

import (
	"context"

	"github.com/rtemka/agg/comments/domain"
)

type MemDB struct{}

func (m *MemDB) Create(_ context.Context, _ *domain.Comment) (int64, error) {
	return 1, nil
}

func (m *MemDB) Read(ctx context.Context, newsID int64) ([]domain.Comment, error) {
	return []domain.Comment{Testcom}, nil
}

func (m *MemDB) Close() error { return nil }

var Testcom = domain.Comment{
	ID:       1,
	NewsID:   1,
	ReplyID:  10,
	PostedAt: 1659947255,
	Text:     "this is simple test comment",
	Author:   domain.Author{ID: 1, Name: "alice"},
}
