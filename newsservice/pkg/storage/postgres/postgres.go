package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rtemka/agg/news/pkg/storage"
)

var ErrNoRows = pgx.ErrNoRows

type statement struct {
	sql  string
	args []any
}

// Postgres выполняет CRUD операции с БД
type Postgres struct {
	db *pgxpool.Pool
}

// New выполняет подключение
// и возвращает объект для взаимодействия с БД
func New(connString string) (*Postgres, error) {

	pool, err := pgxpool.Connect(context.Background(), connString)
	if err != nil {
		return nil, err
	}

	return &Postgres{db: pool}, pool.Ping(context.Background())
}

// Close выполняет закрытие подключения к БД
func (p *Postgres) Close() error {
	p.db.Close()
	return nil
}

// ItemByLink находит по ссылке и возвращает rss-новость
func (p *Postgres) ItemByLink(ctx context.Context, link string) (storage.Item, error) {
	stmt := `
		SELECT
			id,
			title,
			description,
			pub_date,
			link
		FROM news
		WHERE link = $1;`

	var item storage.Item

	err := p.db.QueryRow(ctx, stmt, link).Scan(
		&item.Id, &item.Title, &item.Description,
		&item.PubDate, &item.Link)
	if err != nil {
		return item, err
	}

	return item, nil
}

// Item находит по id и возвращает rss-новость
func (p *Postgres) Item(ctx context.Context, id int64) (storage.Item, error) {

	stmt := `SELECT id, title, description, pub_date, link FROM news WHERE id = $1;`

	var item storage.Item

	return item, p.db.QueryRow(ctx, stmt, id).Scan(
		&item.Id, &item.Title, &item.Description,
		&item.PubDate, &item.Link)
}

// CountItems возвращает количество строк, которое будет задействовано в запросе.
func (p *Postgres) CountItems(ctx context.Context, filter storage.Filter) (int, error) {
	var stmt statement
	stmt.sql = `SELECT COUNT(id) FROM news`
	stmt.addWhereClause(&filter)

	var c int

	return c, p.db.QueryRow(ctx, stmt.sql, stmt.args...).Scan(&c)
}

// Items возвращает списком новости отобранные согласно фильтру.
func (p *Postgres) Items(ctx context.Context, filter storage.Filter) ([]storage.Item, error) {
	var stmt statement
	stmt.sql = `SELECT id, title, description, pub_date, link FROM news`
	stmt.addWhereClause(&filter)
	stmt.addOrderBy(&filter)
	stmt.addLimitOffsetClause(&filter)

	var items []storage.Item

	rows, err := p.db.Query(ctx, stmt.sql, stmt.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {

		var item storage.Item

		err := rows.Scan(&item.Id, &item.Title,
			&item.Description, &item.PubDate, &item.Link)
		if err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

func (stmt *statement) addLimitOffsetClause(f *storage.Filter) {
	l, o := calcLimitOffset(f.Page, storage.PageSize)
	if l > 0 {
		stmt.sql += fmt.Sprintf(" LIMIT $%d", len(stmt.args)+1)
		stmt.args = append(stmt.args, l)
	}
	if o > 0 {
		stmt.sql += fmt.Sprintf(" OFFSET $%d", len(stmt.args)+1)
		stmt.args = append(stmt.args, o)
	}
}

func (stmt *statement) addOrderBy(f *storage.Filter) {
	if f.SortBy == storage.Empty {
		stmt.sql += fmt.Sprintf(" ORDER BY %s DESC", storage.Date.String())
		return
	}
	if f.SortBy == storage.Rank && len(f.TitleSearch) > 0 {
		stmt.sql += fmt.Sprintf(" ORDER BY ts_rank(title_search, to_tsquery('russian', $%d)) DESC", len(stmt.args)+1)
		stmt.args = append(stmt.args, searchStr(f))
		return
	}
	stmt.sql += fmt.Sprintf(" ORDER BY %s DESC", f.SortBy.String())
}

func (stmt *statement) addWhereClause(f *storage.Filter) {
	if len(f.TitleSearch) > 0 {
		stmt.sql += fmt.Sprintf(` WHERE title_search @@ to_tsquery('russian', $%d)`, len(stmt.args)+1)
		stmt.args = append(stmt.args, searchStr(f))
	}
	if f.Date.Value > 0 {
		if len(stmt.args) > 0 {
			stmt.sql += fmt.Sprintf(" AND pub_date %s $%d", f.Date.Operator, len(stmt.args)+1)
		} else {
			stmt.sql += fmt.Sprintf(" WHERE pub_date %s $%d", f.Date.Operator, len(stmt.args)+1)
		}
		stmt.args = append(stmt.args, f.Date.Value)
	}
	if f.Date.Value > 0 && f.EndDate.Value > 0 {
		if len(stmt.args) > 0 {
			stmt.sql += fmt.Sprintf(" AND pub_date %s $%d", f.EndDate.Operator, len(stmt.args)+1)
		}
		stmt.args = append(stmt.args, f.EndDate.Value)
	}
}

func searchStr(f *storage.Filter) string {
	var b strings.Builder

	b.WriteString(strings.Join(f.TitleSearch, "&"))

	if len(f.Exclude) > 0 {
		b.WriteString("&!")
	}
	b.WriteString(strings.Join(f.Exclude, "&!"))
	return b.String()
}

func calcLimitOffset(pageNum, pageSize int) (int, int) {
	if pageNum < 1 {
		return 0, 0
	}
	return pageSize, (pageNum - 1) * pageSize
}

// AddItems добавляет в БД слайс rss-новостей,
// ингорирует те новости, что уже есть в БД
func (p *Postgres) AddItems(ctx context.Context, items []storage.Item) error {
	return p.addItemsByBatch(ctx, items)
}

// addItemsByBatch вносит в БД слайс rss-новостей,
// используя [*pgx.Batch]
func (p *Postgres) addItemsByBatch(ctx context.Context, items []storage.Item) error {

	return p.db.BeginFunc(ctx, func(tx pgx.Tx) error {

		b := new(pgx.Batch) // создаем объект pgx.Batch

		stmt := `
		INSERT INTO news(title, description, pub_date, link)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (link) DO NOTHING;`

		// добавляем все запросы в очередь
		for i := range items {
			b.Queue(stmt, items[i].Title, items[i].Description,
				items[i].PubDate, items[i].Link)
		}

		return tx.SendBatch(ctx, b).Close() // исполняем запросы и закрываем операцию

	})
}

// AddItem добавляет в БД rss-новость, если новость уже
// есть в БД, то no-op
func (p *Postgres) AddItem(ctx context.Context, item storage.Item) error {
	stmt := `
		INSERT INTO news(title, description, pub_date, link)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (link) DO NOTHING;`

	return p.exec(ctx, stmt, item.Title, item.Description, item.PubDate, item.Link)
}

// exec вспомогательная функция, выполняет
// *pgx.conn.Exec() в транзакции
func (p *Postgres) exec(ctx context.Context, sql string, args ...any) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = p.db.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
