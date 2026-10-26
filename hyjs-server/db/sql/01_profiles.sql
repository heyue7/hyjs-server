CREATE TABLE IF NOT EXISTS profiles (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 server_id INTEGER NOT NULL DEFAULT 1,
 account_id INTEGER NOT NULL DEFAULT 0,
 account_auth_revision INTEGER NOT NULL DEFAULT 0,
 uid TEXT NOT NULL UNIQUE,
 session BLOB NOT NULL,
 player TEXT,
 game TEXT,
 last_sprout_login_at INTEGER NOT NULL DEFAULT 0,
 return_sprout_started_at INTEGER NOT NULL DEFAULT 0,
 created_at INTEGER,
 password_salt BLOB,
 password_hash BLOB
);
