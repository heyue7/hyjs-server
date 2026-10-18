-- local-mainline-evening-double-v1 is a declared local policy, original private window unknown.
-- Existing active/explicitly disabled operator windows and monthly off choices stay authoritative.
SET @copy_beijing = CONVERT_TZ(UTC_TIMESTAMP(), '+00:00', '+08:00');
SET @copy_month = DATE_FORMAT(@copy_beijing, '%Y%m');
SET @copy_start_local = STR_TO_DATE(CONCAT(@copy_month,'01'), '%Y%m%d');
SET @copy_start = TIMESTAMPDIFF(SECOND,'1970-01-01 00:00:00',DATE_SUB(@copy_start_local, INTERVAL 8 HOUR))*1000;
SET @copy_end = TIMESTAMPDIFF(SECOND,'1970-01-01 00:00:00',DATE_SUB(DATE_ADD(@copy_start_local, INTERVAL 1 MONTH), INTERVAL 8 HOUR))*1000;
INSERT IGNORE INTO p4_copy_benefit_rules(id,enabled,start_at,end_at,weekday_mask,start_minute,end_minute,currency_factor,drop_factor,source)
SELECT CONCAT('p4:copy:evening:',@copy_month),FALSE,@copy_start,@copy_end,127,1140,1260,2,2,'local-mainline-evening-double-v1：原allCopyDouble静态2x页已恢复，原私服日期/星期/分钟和运营排期未知；具名本地北京时间月实例每天19:00至21:00，主线金币/经验和已确认原首掉各2倍，福利后应用料理；已存在有效重叠运营窗口则不自动另投，旧默认关闭行与手工关闭月实例保持；未知SpecialOdds/Pet不造概率，原开始/结算/限额/失败及历史回执保存'
WHERE NOT EXISTS(SELECT 1 FROM p4_copy_benefit_rules WHERE start_at<@copy_end AND end_at>@copy_start AND (enabled=TRUE OR source NOT LIKE '本地兼容策略（默认门禁关闭）：%'));
