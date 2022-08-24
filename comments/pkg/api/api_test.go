// пакет api предоставляет маршрутизатор REST API

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rtemka/agg/comments/domain"
	"github.com/rtemka/agg/comments/pkg/memdb"
	"go.uber.org/zap"
)

func TestAPI(t *testing.T) {
	api := New(&memdb.MemDB{}, zap.NewNop())
	ts := httptest.NewServer(api)
	defer ts.Close()

	t.Run("post_comments", func(t *testing.T) {
		b, err := json.Marshal(memdb.Testcom)
		if err != nil {
			t.Fatalf("API() = err %v", err)
		}
		resp, err := http.Post(ts.URL+"/comments", "application/json", bytes.NewReader(b))
		if err != nil {
			t.Fatalf("API() = err %v", err)
		}

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("API() = response code %d, want %d", resp.StatusCode, http.StatusCreated)
		}

		var got map[string]map[string]int
		err = json.NewDecoder(resp.Body).Decode(&got)
		if err != nil {
			t.Fatalf("API() = err %v", err)
		}

		want := 1

		if got["response"]["id"] != want {
			t.Fatalf("API() = id %d, want %d", got["response"]["id"], want)
		}
	})

	t.Run("get_comments", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/comments?news-id=1")
		if err != nil {
			t.Fatalf("API() = err %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("API() = response code %d, want %d", resp.StatusCode, http.StatusOK)
		}

		var coms []domain.Comment
		var got = map[string][]domain.Comment{"response": coms}
		err = json.NewDecoder(resp.Body).Decode(&got)
		if err != nil {
			t.Fatalf("API() = err %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("API() = response code %d, want %d", resp.StatusCode, http.StatusOK)
		}

		if len(got["response"]) != 1 {
			t.Fatalf("API() = %d records, want %d records", len(got["response"]), 1)
		}

		if got["response"][0] != memdb.Testcom {
			t.Fatalf("API() = %v, want %v", got["response"][0], memdb.Testcom)
		}
	})
}
