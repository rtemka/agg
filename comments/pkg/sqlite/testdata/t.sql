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

-- INSERT INTO authors(name) VALUES('alice'),('bob'),('john'),('peter');
-- INSERT INTO comments(author_id, news_id, reply_id, text, timestamp) 
-- VALUES (1,1,0,'this is alice comment', 1659695334),
-- (2,1,0,'this is bob comment', 1659695335),
-- (3,1,0,'this is john comment', 1659695336),
-- (4,1,0,'this is peter comment', 1659695336),
-- (3,1,1,'this is john reply to alice comment', 1659695356);