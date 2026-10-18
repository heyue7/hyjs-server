-- Operator must publish the named policy before the actual phase7 window.
INSERT IGNORE INTO p14_promotion_rank_policies(id,enabled,version,source,published_at) VALUES(1,FALSE,'','local-signed-accepted-promotion-score-v1; originaltype2; own-active-spends-only; qualified-roster-power; prepublished-score-desc-power-desc-server-asc-player-asc',0);
