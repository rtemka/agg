package storage

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	strip "github.com/grokify/html-strip-tags-go"
)

const PageSize = 10

type Sort int

const (
	Empty Sort = iota
	Date
	Title
	Rank
)

func (i Sort) String() string {
	return []string{"", "pub_date", "title", "rank"}[i]
}

// Filter - структура для фильтрации новостей.
type Filter struct {
	Exclude     []string   // Фразы, которые стоит исключить.
	SortBy      Sort       // Полe для сортировки (по дате, по названию, кол-во совпадений).
	Page        int        // Номер страницы.
	Date        TimeFilter // Начальная дата или просто дата.
	EndDate     TimeFilter // Конечная дата.
	TitleSearch []string   // Поиск по заголовку.
	// FullMatch bool     // требуется полное совпадение.
	// HeaderFullMatch  bool     // требуется полное совпадение заголовка.
	// Content          string   // по тексту.
	// ContentFullMatch bool     // требуется полное совпадение текста.
}

// TimeFilter содержит время в UNIX формате,
// а также оператор для сравнения ('<', '>=' и т.д.)
type TimeFilter struct {
	Value    int64
	Operator string
}

// Storage - контракт на работу с БД
type Storage interface {
	Items(ctx context.Context, filter Filter) ([]Item, error)   // Получить все новости списком.
	CountItems(ctx context.Context, filter Filter) (int, error) // Получить общее количество элементов по запросу (для пагинации).
	Item(ctx context.Context, id int64) (Item, error)           // Получить новость по id.
	AddItems(context.Context, []Item) error                     // Добавить новости списком.
	Close() error                                               // закрыть БД.
}

// Item - модель данных rss-новости
type Item struct {
	Id          int64  `json:"id" bson:"-"`
	Title       string `json:"title" bson:"title"`
	PubDate     int64  `json:"pubTime" bson:"pubDate"`
	Description string `json:"content" bson:"description"`
	Link        string `json:"link" bson:"link"`
}

func (i Item) String() string {
	return fmt.Sprintf("Id: %d, Title: %s, Description: %s, Link: %s",
		i.Id, i.Title, i.Description, i.Link)
}

// ItemContainer - контейнер содержащий rss-новости.
// Используется для декодирования xml
type ItemContainer struct {
	Items []Item `xml:"channel>item"`
}

// xmlItem - копия Item, единственная польза
// от которой декодирование xml для Item.
// Боремся с проблемой конвертирования времени
// при десериализации
type xmlItem struct {
	XMLName     xml.Name `xml:"item"`
	Title       string   `xml:"title"`
	PubDate     unix     `xml:"pubDate"`
	Description string   `xml:"description"`
	Link        string   `xml:"link"`
}

func (xi *xmlItem) toItem() Item {
	return Item{
		Id:          0,
		Title:       xi.Title,
		PubDate:     int64(xi.PubDate),
		Description: strip.StripTags(xi.Description),
		Link:        xi.Link,
	}
}

func (i *Item) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var xi xmlItem
	err := d.DecodeElement(&xi, &start)
	if err != nil {
		return err
	}
	*i = xi.toItem()
	return nil
}

// для конвертирования из RFC1123Z, RFC1123...
// 'Mon, 02 Jan 2006 15:04:05 -0700' и подобных
// в unix timestamp
type unix int64

var layouts = []string{time.RFC1123Z, time.RFC1123,
	time.UnixDate, "02 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 -0700",
	time.ANSIC, time.RFC850, time.RFC822, time.RFC822Z}

func (t *unix) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return err
	}

	var pt time.Time
	var err error

	for i := range layouts {
		pt, err = time.Parse(layouts[i], s)
		if err == nil {
			break
		}
	}
	*t = unix(pt.Unix())

	return err
}
