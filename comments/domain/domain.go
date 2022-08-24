package domain

import "context"

// Comment - модель данных комментария к rss-новости.
type Comment struct {
	ID       int64  `json:"id"`
	NewsID   int64  `json:"news_id"`
	ReplyID  int64  `json:"reply_id,omitempty"`
	PostedAt int64  `json:"posted_at"`
	Text     string `json:"text"`
	Author
}

// Author - автор комментария к новости.
type Author struct {
	ID   int64  `json:"author_id,omitempty"`
	Name string `json:"author"`
}

type Repository interface {
	Create(context.Context, *Comment) (int64, error)           // создать комментарий к новости
	Read(ctx context.Context, newsID int64) ([]Comment, error) // получить комментарии к новости
	Close() error                                              // закрыть соединение с БД.
}
