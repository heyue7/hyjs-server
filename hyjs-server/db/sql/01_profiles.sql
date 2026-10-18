CREATE TABLE IF NOT EXISTS profiles (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 server_id INTEGER NOT NULL DEFAULT 1,
 uid TEXT NOT NULL UNIQUE,
 session BLOB NOT NULL,
 player TEXT,
 game TEXT,
 created_at INTEGER,
 password_salt BLOB,
 password_hash BLOB
);
