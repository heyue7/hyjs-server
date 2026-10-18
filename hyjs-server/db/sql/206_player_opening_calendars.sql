CREATE TABLE IF NOT EXISTS player_opening_calendars (
    player_id BIGINT NOT NULL PRIMARY KEY,
    start_at BIGINT NOT NULL,
    created_at BIGINT NOT NULL,
    source VARCHAR(96) NOT NULL
);
