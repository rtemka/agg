DROP TABLE IF EXISTS authors;
DROP TABLE IF EXISTS comments;

CREATE TABLE IF NOT EXISTS authors (
    id INTEGER PRIMARY KEY,
    name VARCHAR(255) NOT NULL
);

CREATE TABLE IF NOT EXISTS comments (
    id INTEGER PRIMARY KEY,
    author_id INTEGER NOT NULL,
    news_id INTEGER NOT NULL,
    reply_id INTEGER DEFAULT 0,
    text TEXT NOT NULL,
    timestamp INTEGER CHECK(timestamp > 0),
    FOREIGN KEY(author_id) REFERENCES authors(id)
);

CREATE INDEX IF NOT EXISTS news_id_idx ON comments(news_id);
CREATE INDEX IF NOT EXISTS reply_id_idx ON comments(reply_id);