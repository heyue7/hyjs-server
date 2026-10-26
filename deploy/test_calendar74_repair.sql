-- Explicit repair for the authorized temporary day-7 calendar. No rewards, purchases or receipts are reset.
START TRANSACTION;
SET @test_shift=(SELECT offset_days*86400000 FROM server_game_clocks WHERE server_id=1 FOR UPDATE);
SET @test_now=CAST(UNIX_TIMESTAMP(NOW(3))*1000 AS SIGNED)+@test_shift;
INSERT INTO admin_audits(time,actor,action,target,detail)
SELECT UNIX_TIMESTAMP(NOW(3))*1000,'authorized-ops','growth74.test.policy','guild-donate-box',CONCAT('Temporary test calendar: published_at ',published_at,' shift ',@test_shift,'; unchanged rules, rewards and immutable history') FROM p13_guild_donate_box_policies WHERE id=1 AND enabled=1 AND @test_shift<0 AND published_at>@test_now AND published_at+@test_shift>0;
UPDATE p13_guild_donate_box_policies SET published_at=published_at+@test_shift WHERE id=1 AND enabled=1 AND @test_shift<0 AND published_at>@test_now AND published_at+@test_shift>0;
SET @test_day=DATE(FROM_UNIXTIME((@test_now+28800000)/1000));
SET @test_week=DATE_FORMAT(DATE_SUB(@test_day,INTERVAL WEEKDAY(@test_day) DAY),'%Y%m%d');
INSERT INTO admin_audits(time,actor,action,target,detail)
SELECT UNIX_TIMESTAMP(NOW(3))*1000,'authorized-ops','growth74.test.timer',CONCAT('memory/',s.player_id,'/',CONVERT(s.rule_id USING utf8mb4)),CONCAT('Temporary test calendar: freeAt ',JSON_UNQUOTE(JSON_EXTRACT(CONVERT(s.payload USING utf8mb4),'$.freeAt')),' shift ',@test_shift,'; all offers and quotas retained')
FROM p4_commerce_states s JOIN p4_commerce_rules r ON r.id=s.rule_id
WHERE r.kind='memory' AND @test_shift<0 AND JSON_VALID(CONVERT(s.payload USING utf8mb4)) AND CAST(JSON_UNQUOTE(JSON_EXTRACT(CONVERT(s.payload USING utf8mb4),'$.freeAt')) AS SIGNED)>@test_now;
UPDATE p4_commerce_states s JOIN p4_commerce_rules r ON r.id=s.rule_id
SET s.payload=JSON_SET(CONVERT(s.payload USING utf8mb4),'$.freeAt',CAST(JSON_UNQUOTE(JSON_EXTRACT(CONVERT(s.payload USING utf8mb4),'$.freeAt')) AS SIGNED)+@test_shift)
WHERE r.kind='memory' AND @test_shift<0 AND JSON_VALID(CONVERT(s.payload USING utf8mb4)) AND CAST(JSON_UNQUOTE(JSON_EXTRACT(CONVERT(s.payload USING utf8mb4),'$.freeAt')) AS SIGNED)>@test_now;
SET @memory_fixed=ROW_COUNT();
INSERT INTO admin_audits(time,actor,action,target,detail)
SELECT UNIX_TIMESTAMP(NOW(3))*1000,'authorized-ops','growth74.test.week',CONCAT('pet-market/',player_id),CONCAT('Temporary test calendar: week ',week_key,' -> ',@test_week,'; frozen offers, bought, generation and refresh_used retained') FROM p9_pet_market_states WHERE @test_shift<0 AND week_key>@test_week;
UPDATE p9_pet_market_states SET week_key=@test_week WHERE @test_shift<0 AND week_key>@test_week;
SET @pet_fixed=ROW_COUNT();
COMMIT;
SELECT @memory_fixed,@pet_fixed,@test_week;
