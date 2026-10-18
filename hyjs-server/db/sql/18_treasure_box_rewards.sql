CREATE TABLE IF NOT EXISTS treasure_box_rewards (
 item_id VARCHAR(64) NOT NULL,
 id BIGINT NOT NULL,
 resource_id VARCHAR(64) NOT NULL,
 amount INTEGER NOT NULL,
 weight BIGINT NOT NULL,
 source VARCHAR(255) NOT NULL,
 PRIMARY KEY(item_id,id)
);
