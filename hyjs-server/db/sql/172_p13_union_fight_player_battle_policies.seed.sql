INSERT IGNORE INTO `p13_union_fight_player_battle_policies`
 (`id`,`enabled`,`version`,`source`,`published_at`,`starts_at`,`ends_at`,`revive_rule`)
 VALUES (1,FALSE,'ordinary-player14-factor1-v1','',0,0,0,'');

-- Separate current-state capability; never upgrades an old fullHP offer.
INSERT IGNORE INTO p13_union_fight_player_battle_policies (id,enabled,version,source,published_at,starts_at,ends_at,revive_rule) VALUES (2,0,'ordinary-player14-current-state-v1','',0,0,0,'');
