package rsscollector

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

const xmlblob = `
		<rss>
			<channel>
				<item>
					<title>Тестовый заголовок</title>
					<link>https://test.com</link>
					<description>Тестовое описание</description>
					<pubDate>Thu, 16 Jun 2022 10:14:28 +0300</pubDate>
				</item>
			</channel>
		</rss>
		`

func Test_poll(t *testing.T) {

	want := item{
		Id:          0,
		Title:       "Тестовый заголовок",
		PubDate:     1655363668,
		Description: "Тестовое описание",
		Link:        "https://test.com",
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprintln(w, xmlblob)
	}))
	defer ts.Close()

	got, err := poll(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("poll() error = %v", err)
	}

	if len(got.Items) != 1 {
		t.Fatalf("poll() got results = %d, want = %d", len(got.Items), 1)
	}

	if got.Items[0] != want {
		t.Fatalf("poll() got = %v, want = %v", got, want)
	}
}

func TestCollector_Poll(t *testing.T) {

	var m sync.Mutex
	var want int

	timeout := 200 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		select {
		case <-ctx.Done():
			http.Error(w, "", http.StatusTeapot)
		default:
			m.Lock()
			want++
			m.Unlock()
			w.Header().Set("Content-Type", "text/xml")
			fmt.Fprintln(w, xmlblob)
		}

	}))
	defer ts.Close()

	collector := New(log.New(io.Discard, "", 0))

	t.Run("oшибка_ноль_ссылок", func(t *testing.T) {
		_, _, err := collector.Poll(context.Background(), time.Second, nil)
		if err == nil {
			t.Fatal("Collector.Poll() expected error, got nothing")
		}
	})

	t.Run("подсчёт_количества_опросов", func(t *testing.T) {

		// контекст с небольшой задержкой, чтобы успеть прочитать
		// все данные из канала при закрытии
		ctx2, cancel2 := context.WithTimeout(context.Background(), timeout+(20*time.Millisecond))
		defer cancel2()

		values, errs, err := collector.Poll(ctx2, time.Millisecond,
			[]string{ts.URL, ts.URL, ts.URL, ts.URL, ts.URL, ts.URL, ts.URL, ts.URL})
		if err != nil {
			t.Fatalf("Collector.Poll() error = %v", err)
		}

		var wg sync.WaitGroup
		wg.Add(2)

		got := 0

		go func() {
			for range values {
				got++
			}
			wg.Done()
		}()

		go func() {
			for range errs {
			}
			wg.Done()
		}()

		wg.Wait()

		if got != want {
			t.Fatalf("Collector.Poll() got values = %d, want values = %d", got, want)
		}
	})
}
