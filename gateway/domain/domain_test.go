package domain

import (
	"reflect"
	"testing"
)

func Test_ToTree(t *testing.T) {
	in := []Comment{
		{Id: 4, ReplyID: 1},
		{Id: 5, ReplyID: 2},
		{Id: 6, ReplyID: 3},
		{Id: 1, ReplyID: 0},
		{Id: 2, ReplyID: 0},
		{Id: 3, ReplyID: 0},
		{Id: 7, ReplyID: 4},
		{Id: 8, ReplyID: 5},
		{Id: 9, ReplyID: 6},
		{Id: 10, ReplyID: 0},
		{Id: 11, ReplyID: 1},
		{Id: 12, ReplyID: 2},
		{Id: 13, ReplyID: 3},
		{Id: 14, ReplyID: 7},
	}

	want := []Comment{
		{Id: 1, Replies: []Comment{{Id: 4, ReplyID: 1,
			Replies: []Comment{{Id: 7, ReplyID: 4, Replies: []Comment{{Id: 14, ReplyID: 7}}}}}, {Id: 11, ReplyID: 1}}},
		{Id: 2, Replies: []Comment{{Id: 5, ReplyID: 2, Replies: []Comment{{Id: 8, ReplyID: 5}}}, {Id: 12, ReplyID: 2}}},
		{Id: 3, Replies: []Comment{{Id: 6, ReplyID: 3, Replies: []Comment{{Id: 9, ReplyID: 6}}}, {Id: 13, ReplyID: 3}}},
		{Id: 10},
	}

	got := ToTree(in)

	if len(got) != len(want) {
		t.Fatalf("ToTree() len = %d, want %d", len(got), len(want))
	}

	for i := range got {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("ToTree() = %v, want %v", got[i], want[i])
		}
	}
}
