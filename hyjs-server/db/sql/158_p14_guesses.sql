CREATE TABLE IF NOT EXISTS p14_guesses (
 season_index INT NOT NULL, guess_type INT NOT NULL, owner_server_id BIGINT NOT NULL,
 owner_player_id BIGINT NOT NULL, target_server_id BIGINT NOT NULL, target_player_id BIGINT NOT NULL,
 offer_hash CHAR(64) NOT NULL, cost_coin BIGINT NOT NULL, coin_before BIGINT NOT NULL,
 coin_after BIGINT NOT NULL, captured_at BIGINT NOT NULL, source_hash CHAR(64) NOT NULL,
 PRIMARY KEY(season_index,guess_type,owner_server_id,owner_player_id)
) ENGINE=InnoDB;
