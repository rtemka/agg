package domain

// NewsFullDetailed - модель данных rss-новости.
type NewsFullDetailed struct {
	Id          int64     `json:"id,omitempty"`
	Title       string    `json:"title,omitempty"`
	PubDate     int64     `json:"pubTime,omitempty"`
	Description string    `json:"content,omitempty"`
	Link        string    `json:"link,omitempty"`
	Comments    []Comment `json:"comments,omitempty"`
}

// NewsShortDetailed - модель данных rss-новости
// в компактном виде.
type NewsShortDetailed struct {
	Id      int64  `json:",omitempty"`
	Title   string `json:"title,omitempty"`
	PubDate int64  `json:"pubTime,omitempty"`
	Link    string `json:"link,omitempty"`
}

// Comment - модель данных комментария к rss-новости.
type Comment struct {
	Id       int64     `json:"id,omitempty"`
	Author   string    `json:"author,omitempty"`
	Text     string    `json:"text,omitempty"`
	PostedAt int64     `json:"posted_at,omitempty"`
	ReplyID  int64     `json:"reply_id,omitempty"`
	Replies  []Comment `json:"replies,omitempty"`
}

// ToTree - возвращает дерево комментариев.
func ToTree(comments []Comment) []Comment {

	var m = make(map[int64][]*Comment, len(comments))
	var tops int

	for i := range comments {
		if comments[i].ReplyID == 0 {
			tops++
			if _, ok := m[comments[i].Id]; !ok {
				m[comments[i].Id] = nil
			}
			continue
		}
		m[comments[i].ReplyID] = append(m[comments[i].ReplyID], &comments[i])
	}

	for i := range comments {
		if len(m[comments[i].Id]) != 0 {
			for _, v := range m[comments[i].Id] {
				comments[i].Replies = append(comments[i].Replies, *dig(v, m))
			}
			m[comments[i].Id] = nil
		}
	}

	var out = make([]Comment, 0, tops)

	for i := range comments {
		if comments[i].ReplyID == 0 {
			out = append(out, comments[i])
		}
	}

	return out
}

func dig(c *Comment, m map[int64][]*Comment) *Comment {
	if len(m[c.Id]) == 0 {
		return c
	}

	for _, v := range m[c.Id] {
		c.Replies = append(c.Replies, *dig(v, m))
	}

	return c
}
