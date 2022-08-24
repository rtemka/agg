package storage

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestItem_UnmarshalXML(t *testing.T) {
	blob := `
		<root>
			<nested>
				<item>
					<title>Тестовый заголовок</title>
					<link>https://test.com</link>
					<description>Тестовое описание</description>
					<pubDate>Thu, 16 Jun 2022 10:14:28 +0300</pubDate>
				</item>
				<item>
					<title>Тестовый заголовок</title>
					<link>https://test.com</link>
					<description>Тестовое описание</description>
					<pubDate>Thu, 16 Jun 2022 10:14:28 +0300</pubDate>
				</item>
			</nested>
		</root>
		`

	r := struct {
		Items []Item `xml:">item"`
	}{}

	err := xml.NewDecoder(strings.NewReader(blob)).Decode(&r)
	if err != nil {
		t.Fatalf("Item.UnmarshalXML() error = %v", err)
	}

	want := Item{
		Id:          0,
		Title:       "Тестовый заголовок",
		PubDate:     1655363668,
		Description: "Тестовое описание",
		Link:        "https://test.com",
	}

	for _, got := range r.Items {
		if got != want {
			t.Fatalf("Item.UnmarshalXML() got = %v, want %v", got, want)
		}
	}

}
