package streamwriter

import (
	"context"
	"io"
	"log"

	"testing"
	"time"

	"github.com/rtemka/agg/news/pkg/storage/memdb"
)

func TestStreamWriter_WriteToStorage(t *testing.T) {

	sw := NewStreamWriter(log.New(io.Discard, "", 0), memdb.New())

	cont := container{Items: []item{
		{
			Id:          1,
			PubDate:     5555555,
			Title:       "sample item",
			Description: "sample discription",
			Link:        "https://test.com",
		},
		{
			Id:          1,
			PubDate:     5555555,
			Title:       "sample item",
			Description: "sample discription",
			Link:        "https://test.com",
		},
	},
	}

	var want uint
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ch := make(chan container)

	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				want++
				ch <- cont
			}
		}
	}()

	stats, err := sw.WriteToStorage(ctx, ch)
	if err != nil {
		t.Fatalf("StreamWriter.WriteToStorage() error = %v", err)
	}

	if stats.Containers != want {
		t.Errorf("StreamWriter.WriteToStorage() got containers = %d, want = %d",
			stats.Containers, want)
	}

	if stats.Items != want*2 {
		t.Errorf("StreamWriter.WriteToStorage() got items = %d, want = %d",
			stats.Items, want*2)
	}
}
