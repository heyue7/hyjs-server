CREATE TABLE IF NOT EXISTS p13_union_fight_field_players (
 map_projection VARCHAR(32) NOT NULL DEFAULT '',
 match_id CHAR(64) NOT NULL,player_id BIGINT NOT NULL,guild_id VARCHAR(64) NOT NULL,camp BIGINT NOT NULL,
 frame_state BIGINT NOT NULL DEFAULT 0,
 frame LONGBLOB NOT NULL,frame_hash CHAR(64) NOT NULL,battle LONGBLOB NOT NULL,battle_hash CHAR(64) NOT NULL,
 x FLOAT NOT NULL,y FLOAT NOT NULL,z FLOAT NOT NULL,session_hash CHAR(64) NOT NULL DEFAULT '',
 joined_at BIGINT NOT NULL,updated_at BIGINT NOT NULL,left_at BIGINT NOT NULL DEFAULT 0,hooked BOOLEAN NOT NULL DEFAULT FALSE,movement_debt DOUBLE NOT NULL DEFAULT 0,source TEXT NOT NULL,
 PRIMARY KEY(match_id,player_id),INDEX idx_p13_field_player(player_id),INDEX p13_field_natural_revive(frame_state,left_at,match_id,player_id),INDEX p13_field_lifecycle(left_at,player_id,match_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
