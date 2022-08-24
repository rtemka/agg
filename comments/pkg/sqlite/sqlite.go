package sqlite

import (
	"context"
	"database/sql"
	"os"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rtemka/agg/comments/domain"
)

// ErrNoRows когда по запросу не найдены строки.
var ErrNoRows = sql.ErrNoRows

// SQLite выполняет операции CRUD в БД.
type SQLite struct {
	// это поле экпортируемое, чтобы пользователь
	// мог установить такие важные параметры подлючения как
	// SetConnMaxIdleTime, SetMaxOpenConns, SetMaxIdleConns...
	DB *sql.DB
}

// New производит подключение к [*SQLite] БД.
func New(connstr string) (*SQLite, error) {

	db, err := sql.Open("sqlite3", connstr)
	if err != nil {
		return nil, err
	}

	return &SQLite{DB: db}, db.Ping()
}

// Close closes db connection.
func (l *SQLite) Close() error {
	return l.DB.Close()
}

// Create создает комментарий к новости.
func (l *SQLite) Create(ctx context.Context, c *domain.Comment) (int64, error) {
	tx, err := l.DB.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err := createAuthor(ctx, tx, c); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	id, err := createComment(ctx, tx, c)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}

	return id, tx.Commit()
}

func createAuthor(ctx context.Context, tx *sql.Tx, c *domain.Comment) error {
	if c.Author.ID == 0 {
		stmt := `INSERT INTO authors(name) VALUES($1);`
		res, err := tx.ExecContext(ctx, stmt, c.Author.Name)
		if err != nil {
			return err
		}
		c.Author.ID, err = res.LastInsertId()
		return err
	}
	stmt := `INSERT INTO authors(id, name) VALUES($1, $2) ON CONFLICT(id) DO NOTHING;`
	_, err := tx.ExecContext(ctx, stmt, c.Author.ID, c.Author.Name)
	return err
}

func createComment(ctx context.Context, tx *sql.Tx, c *domain.Comment) (int64, error) {
	stmt := `INSERT INTO comments(author_id, news_id, reply_id, text, timestamp)
		VALUES($1, $2, $3, $4, $5)`
	args := []any{c.Author.ID, c.NewsID, c.ReplyID, c.Text, c.PostedAt}
	if c.NewsID == 0 && c.ReplyID != 0 {
		stmt = `INSERT INTO comments(author_id, news_id, reply_id, text, timestamp)
		VALUES($1, (SELECT news_id FROM comments WHERE id = $2), $2, $3, $4)`
		args = []any{c.Author.ID, c.ReplyID, c.Text, c.PostedAt}
	}
	res, err := tx.ExecContext(ctx, stmt, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Read получает все комментарии к новости.
func (l *SQLite) Read(ctx context.Context, newsID int64) ([]domain.Comment, error) {
	stmt := `
		SELECT 
			c.id, a.id, a.name, 
			c.news_id, c.reply_id, c.text,
			c.timestamp
		FROM comments as c JOIN authors as a ON c.author_id = a.id
		WHERE c.news_id = $1;`

	rows, err := l.DB.QueryContext(ctx, stmt, newsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var coms []domain.Comment

	for rows.Next() {
		var c domain.Comment

		err := rows.Scan(&c.ID, &c.Author.ID, &c.Author.Name,
			&c.NewsID, &c.ReplyID, &c.Text, &c.PostedAt)
		if err != nil {
			return nil, err
		}
		coms = append(coms, c)
	}
	if len(coms) == 0 {
		return nil, ErrNoRows
	}

	return coms, rows.Err()

}

// RunFile читает и исполняет sql-файл.
func (l *SQLite) RunFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return l.exec(context.Background(), string(b))
}

// exec вспомогательная функция, выполняет
// *tx.Exec() в транзакции.
func (l *SQLite) exec(ctx context.Context, stmt string, args ...any) error {
	tx, err := l.DB.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	_, err = l.DB.ExecContext(ctx, stmt, args...)
	if err != nil {
		return err
	}

	return tx.Commit()
}
