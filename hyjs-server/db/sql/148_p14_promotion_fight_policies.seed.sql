-- No rules are implicitly published and no battle certificate is created by this seed.
INSERT IGNORE INTO p14_promotion_fight_policies
  (id, enabled, version, source, rules_json, rules_hash, published_at)
VALUES (1, FALSE, '', 'local-prepublished-promotion-fight-v1; originalpoint1/max40/initialscore1; scopedtyped17; foreign-distinct-playerid; localwin15-loss0-no-powerbonus-min1', '', '', 0);
