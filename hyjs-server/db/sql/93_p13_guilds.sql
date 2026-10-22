-- P13独立持久化事实；原协议字段由service显式映射。
-- union_fight_auto_apply 由原 p13_union_fight_settings 并入，NULL 仅表示旧库尚未回填，正式读取按 false 处理。
-- 旧库由 InitializeP13Guild 补列/索引并按 NULL 哨兵分批回填，可中断重入；旧表核验及回退完成前保留。
CREATE TABLE IF NOT EXISTS p13_guilds (id VARCHAR(64) PRIMARY KEY,name VARCHAR(64) NOT NULL,level INT NOT NULL,notice TEXT NOT NULL,declaration TEXT NOT NULL,flag INT NOT NULL,coin BIGINT NOT NULL,badge INT NOT NULL,exp INT NOT NULL,donate_score INT NOT NULL,active_score INT NOT NULL,hero_statue_open INT NOT NULL DEFAULT 0,guard_tree_open INT NOT NULL DEFAULT 0,union_fight_auto_apply TINYINT(1) NULL,created_at BIGINT NOT NULL,UNIQUE KEY idx_p13_guilds_name(name),KEY idx_p13_guilds_union_fight_auto_apply(union_fight_auto_apply)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
