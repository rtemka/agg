package sqlite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rtemka/agg/comments/domain"
)

var tdb *SQLite

func restoreDB(tdb *SQLite) error {
	b, err := os.ReadFile(filepath.Join("testdata", "t.sql"))
	if err != nil {
		return err
	}

	return tdb.exec(context.Background(), string(b))
}

func TestMain(m *testing.M) {

	var err error
	tdb, err = New("file:test.db?cache=shared&mode=memory&_fk=on")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := restoreDB(tdb); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestSQLite(t *testing.T) {
	if tdb == nil {
		t.Skip("you must open connection to SQLite DB to run this test")
	}

	_, err := tdb.Read(context.Background(), 0)
	if err != ErrNoRows {
		t.Errorf("Read() = err %v, wnatErr %v", err, ErrNoRows)
	}

	want := []domain.Comment{testcom, testcom2, testcom3}
	for i := range want {
		_, err := tdb.Create(context.Background(), &want[i])
		if err != nil {
			t.Fatalf("Create() = err %v", err)
		}
	}

	got, err := tdb.Read(context.Background(), want[0].NewsID)
	if err != nil {
		t.Fatalf("Read() = err %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("Read() = %d records, want %d records", len(got), len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Read() = %v, want %v", got[i], want[i])
		}
	}

	// когда нет id новости, зато есть reply_id
	norepl := testcom4
	norepl.NewsID = 1
	want = append(want, norepl)
	_, err = tdb.Create(context.Background(), &testcom4)
	if err != nil {
		t.Fatalf("Create() = err %v", err)
	}

	got, err = tdb.Read(context.Background(), 1)
	if err != nil {
		t.Fatalf("Read() = err %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("Read() = %d records, want %d records", len(got), len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Read() = %v, want %v", got[i], want[i])
		}
	}
}

var testcom = domain.Comment{
	ID:       1,
	NewsID:   1,
	ReplyID:  10,
	PostedAt: 1659947255,
	Text:     "this is simple test comment",
	Author:   domain.Author{ID: 1, Name: "alice"},
}
var testcom2 = domain.Comment{
	ID:       2,
	NewsID:   1,
	ReplyID:  1,
	PostedAt: 1659947256,
	Text:     "this is another test comment",
	Author:   domain.Author{ID: 3, Name: "john"},
}
var testcom3 = domain.Comment{
	ID:       3,
	NewsID:   1,
	ReplyID:  0,
	PostedAt: 1659947257,
	Text:     "this is simple another test comment",
	Author:   domain.Author{ID: 2, Name: "bob"},
}
var testcom4 = domain.Comment{
	ID:       4,
	NewsID:   0,
	ReplyID:  2,
	PostedAt: 1659947258,
	Text:     "this is test comment as reply to john comment",
	Author:   domain.Author{ID: 4, Name: "gary"},
}
