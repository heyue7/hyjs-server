package constUtil

// AdminActivityRewardQueryChunkSize 限制单条 IN 查询的池数量，避免超出数据库参数上限。
const AdminActivityRewardQueryChunkSize = 500
