package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/joho/godotenv"
	"github.com/rtemka/agg/news/pkg/storage"
)

var tdb *Postgres // тестовая БД

const dbEnv = "TEST_DB_URL"

func restoreTestDB(testdb *Postgres) error {

	b, err := os.ReadFile(filepath.Join("testdata", "testdb.sql"))
	if err != nil {
		return err
	}

	return tdb.exec(context.Background(), string(b))
}

func TestMain(m *testing.M) {
	_ = godotenv.Load(".env") // загружаем переменные окружения из файла
	connstr, ok := os.LookupEnv(dbEnv)
	if !ok {
		os.Exit(m.Run()) // тест будет пропущен
	}

	var err error
	tdb, err = New(connstr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := restoreTestDB(tdb); err != nil {
		tdb.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	defer tdb.Close()

	os.Exit(m.Run())
}

func TestPostgres(t *testing.T) {
	if _, ok := os.LookupEnv(dbEnv); !ok {
		t.Skipf("environment variable %s not set, skipping tests", dbEnv)
	}

	t.Run("AddItems()", func(t *testing.T) {
		wantItems := []storage.Item{testItem1, testItem2, testItem3, testItem4}

		err := tdb.AddItems(context.Background(), wantItems)
		if err != nil {
			t.Fatalf("AddItems() error = %v", err)
		}

		gotItems, err := tdb.Items(context.Background(), storage.Filter{Page: 1})
		if err != nil {
			t.Fatalf("Items() error = %v", err)
		}

		if len(gotItems) != len(wantItems) {
			t.Fatalf("Items() got items = %d, want = %d", len(gotItems), len(wantItems))
		}

		for i := range wantItems {
			if gotItems[i] != wantItems[i] {
				t.Fatalf("AddItems() got = %v, want = %v", gotItems[i], wantItems[i])
			}
		}
	})

	t.Run("ItemByLink()", func(t *testing.T) {
		want := testItem1

		got, err := tdb.ItemByLink(context.Background(), want.Link)
		if err != nil {
			t.Fatalf("ItemByLink() error = %v", err)
		}

		if got != want {
			t.Fatalf("ItemByLink() got = %v, want = %v", got, want)
		}
	})

	t.Run("Item()", func(t *testing.T) {
		want := testItem1

		got, err := tdb.Item(context.Background(), want.Id)
		if err != nil {
			t.Fatalf("Item() error = %v", err)
		}

		if got != want {
			t.Fatalf("Item() got = %v, want = %v", got, want)
		}
	})

	t.Run("Items()_title_search", func(t *testing.T) {
		want := testItem3

		v, err := tdb.Items(context.Background(), storage.Filter{TitleSearch: []string{"голэнг", "go"}})
		if err != nil {
			t.Fatalf("Items() error = %v", err)
		}

		if len(v) != 1 {
			t.Fatalf("Items() got items = %d, want = %d", len(v), 1)
		}

		got := v[0]

		if got != want {
			t.Fatalf("Items() got = %v, want = %v", got, want)
		}
	})

	t.Run("Items()_title_search_exclude", func(t *testing.T) {
		want := []storage.Item{testItem1, testItem2}

		got, err := tdb.Items(context.Background(), storage.Filter{TitleSearch: []string{"go"}, Exclude: []string{"голэнг"}})
		if err != nil {
			t.Fatalf("Items() error = %v", err)
		}

		if len(got) != len(want) {
			t.Fatalf("Items() got items = %d, want = %d", len(got), len(want))
		}

		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("Items() got = %v, want = %v", got[i], want[i])
			}
		}
	})

	t.Run("Items()_date_search_==", func(t *testing.T) {
		want := testItem4

		v, err := tdb.Items(context.Background(), storage.Filter{Date: storage.TimeFilter{Value: 1659344500, Operator: "="}})
		if err != nil {
			t.Fatalf("Items() error = %v", err)
		}

		if len(v) != 1 {
			t.Fatalf("Items() got items = %d, want = %d", len(v), 1)
		}

		got := v[0]

		if got != want {
			t.Fatalf("Items() got = %v, want = %v", got, want)
		}
	})

	t.Run("Items()_date_search_>=", func(t *testing.T) {
		want := 4

		v, err := tdb.Items(context.Background(), storage.Filter{Date: storage.TimeFilter{Value: 1659344500, Operator: ">="}})
		if err != nil {
			t.Fatalf("Items() error = %v", err)
		}

		if len(v) != want {
			t.Fatalf("Items() got items = %d, want = %d", len(v), want)
		}

	})

	t.Run("Items()_date_search_IN", func(t *testing.T) {
		want := []storage.Item{testItem2, testItem3}

		got, err := tdb.Items(context.Background(),
			storage.Filter{
				Date:    storage.TimeFilter{Value: 1659344500, Operator: ">"},
				EndDate: storage.TimeFilter{Value: 1659517300, Operator: "<="},
			})
		if err != nil {
			t.Fatalf("Items() error = %v", err)
		}

		if len(got) != len(want) {
			t.Fatalf("Items() got items = %d, want = %d", len(got), len(want))
		}

		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("Items() got = %v, want = %v", got[i], want[i])
			}
		}

	})

	t.Run("Items()_sort_by_date", func(t *testing.T) {
		want := []storage.Item{testItem4, testItem3, testItem2, testItem1}

		got, err := tdb.Items(context.Background(),
			storage.Filter{Page: 1, SortBy: storage.Date})
		if err != nil {
			t.Fatalf("Items() error = %v", err)
		}

		if len(got) != len(want) {
			t.Fatalf("Items() got items = %d, want = %d", len(got), len(want))
		}

		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("Items() got = %v, want = %v", got[i], want[i])
			}
		}

	})

	t.Run("Items()_sort_by_rank", func(t *testing.T) {
		want := []storage.Item{testItem1, testItem2, testItem3}

		got, err := tdb.Items(context.Background(),
			storage.Filter{Page: 1, SortBy: storage.Rank, TitleSearch: []string{"go"}})
		if err != nil {
			t.Fatalf("Items() error = %v", err)
		}

		if len(got) != len(want) {
			t.Fatalf("Items() got items = %d, want = %d", len(got), len(want))
		}

		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("Items() got = %v, want = %v", got[i], want[i])
			}
		}

	})
}

var testItem1 = storage.Item{
	Id:          1,
	Title:       "Заголовок 1; go go go go",
	Description: "Описание 1",
	PubDate:     1659603700,
	Link:        "https://test.com/14987527",
}

var testItem2 = storage.Item{
	Id:          2,
	Title:       "Заголовок 2; база данных база данных база данных go go",
	Description: "Описание 2",
	PubDate:     1659517300,
	Link:        "https://test.com/14987528",
}

var testItem3 = storage.Item{
	Id:          3,
	Title:       "Заголовок 3; голэнг go",
	Description: "Описание 3",
	PubDate:     1659430900,
	Link:        "https://test.com/14987529",
}

var testItem4 = storage.Item{
	Id:          4,
	Title:       "Заголовок 4; индепотентность",
	Description: "Описание 4",
	PubDate:     1659344500,
	Link:        "https://test.com/149875210",
}
