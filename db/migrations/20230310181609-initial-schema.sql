-- +migrate Up
CREATE TABLE trackings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    tracking_number TEXT NOT NULL,
    display_name TEXT,
    last_polled_at INTEGER,
    payload TEXT
);

CREATE UNIQUE INDEX trackings_user_id_tracking_number ON trackings (user_id, tracking_number);

CREATE TABLE users_chats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL UNIQUE,
    chat_id INTEGER NOT NULL UNIQUE
);


-- +migrate Down
DROP TABLE trackings;
DROP TABLE users_chats;
