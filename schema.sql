DROP TABLE IF EXISTS news;

-- таблица с rss-новостями
CREATE TABLE IF NOT EXISTS news (
    id BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL,
	description TEXT NOT NULL,
    pub_date BIGINT CHECK(pub_date > 0),
    link TEXT NOT NULL UNIQUE,
    title_search tsvector generated always as(to_tsvector('russian', title)) stored
);

CREATE INDEX IF NOT EXISTS pub_date_idx ON news(pub_date DESC);
CREATE INDEX IF NOT EXISTS title_idx ON news USING GIN (title_search);