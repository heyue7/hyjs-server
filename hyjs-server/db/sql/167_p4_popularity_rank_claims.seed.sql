-- New rank-tab offer is disabled and unpublished. Existing monthly ordinary
-- popularity IDs, prices and quota are unchanged. Set a future window and
-- rankPublishedAt before enabling a new version; the first original list freezes.
INSERT IGNORE INTO p4_commerce_rules (id,shop_type,kind,enabled,start_at,end_at,claim_end_at,rules,frozen,source)
VALUES ('p4:popularity-rank:local-v1',4,'popularity-rank',FALSE,0,0,0,
'{"products":[{"id":"p4PopularityRank_local_v1_1","index":1,"rewards":"item_001,1","cost":"popularity,20","priority":1001,"dailyLimit":1,"totalLimit":1,"stock":-1,"minLevel":10,"minVip":0,"discountBps":10000,"weight":1,"maxRank":1000}],"rankPublishedAt":0}',NULL,
'local-popularity-rank-v1: original shop4 rank tab and currency; private server catalogue, price and rank thresholds unknown; prepublished local finite exchange');
