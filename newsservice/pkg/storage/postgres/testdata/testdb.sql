DROP TABLE IF EXISTS news;

CREATE TABLE IF NOT EXISTS news (
    id BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL,
	description TEXT NOT NULL,
    pub_date BIGINT CHECK(pub_date > 0),
    link TEXT NOT NULL UNIQUE,
    title_search tsvector generated always as(to_tsvector('russian', title)) stored
);