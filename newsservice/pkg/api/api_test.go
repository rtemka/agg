package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"

	"testing"

	"github.com/rtemka/agg/news/pkg/storage/memdb"
)

func TestApi_itemsHandler(t *testing.T) {
	api := New(memdb.New(), log.New(io.Discard, "", 0))
	wantLen := 10

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/news/%d", wantLen), nil)
	rr := httptest.NewRecorder()

	api.r.ServeHTTP(rr, req)

	resp := rr.Result()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Api.itemsHandler() got response code = %d, want = %d", resp.StatusCode, http.StatusOK)
	}
	var items []item
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		t.Fatalf("Api.itemsHandler() got error = %v", err)
	}

	if len(items) != wantLen {
		t.Errorf("Api.itemsHandler() got items = %d, want = %d", len(items), wantLen)
	}

	if len(items) > 0 {
		if items[0] != memdb.SampleItem {
			t.Errorf("Api.itemsHandler() got items[0] = %v, want = %v", items[0], memdb.SampleItem)
		}
	}
}
