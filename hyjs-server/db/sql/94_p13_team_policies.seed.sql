-- 具名本地兼容配置由用户授权，原容量/TTL未知不伪造原来源；默认关闭。
INSERT IGNORE INTO p13_team_policies(id,enabled,max_size,invite_ttl_millis,source) VALUES(1,0,5,0,'原UI5头位本地容量5/TTL0，无时钟过期；邀请由队长授权和双方成员代数撤销；原容量/过期及共享战斗待审计');
