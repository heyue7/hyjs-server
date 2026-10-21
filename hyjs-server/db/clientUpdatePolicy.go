package db

// ClientUpdatePolicy 保存客户端更新的独立运行事实。
type ClientUpdatePolicy struct {
	ID                 int64  `gorm:"column:id;primaryKey;autoIncrement:false" json:"id"`
	ReleaseID          int64  `gorm:"column:release_id;not null" json:"releaseId"`
	LatestBuild        int64  `gorm:"column:latest_build;not null" json:"latestBuild"`
	MinLoginBuild      int64  `gorm:"column:min_login_build;not null" json:"minLoginBuild"`
	MinRunBuild        int64  `gorm:"column:min_run_build;not null" json:"minRunBuild"`
	Revision           int64  `gorm:"column:revision;not null" json:"revision"`
	NoticeBatch        int64  `gorm:"column:notice_batch;not null" json:"noticeBatch"`
	NoticeEnabled      bool   `gorm:"column:notice_enabled;not null" json:"noticeEnabled"`
	Maintenance        bool   `gorm:"column:maintenance;not null" json:"maintenance"`
	MaintenanceStartAt int64  `gorm:"column:maintenance_start_at;not null" json:"maintenanceStartAt"`
	MaintenanceEndAt   int64  `gorm:"column:maintenance_end_at;not null" json:"maintenanceEndAt"`
	MaintenanceEpoch   int64  `gorm:"column:maintenance_epoch;not null" json:"maintenanceEpoch"`
	LoginGateAt        int64  `gorm:"column:login_gate_at;not null" json:"loginGateAt"`
	VersionName        string `gorm:"column:version_name;size:80" json:"versionName"`
	Title              string `gorm:"column:title;size:240" json:"title"`
	Content            string `gorm:"column:content;type:text" json:"content"`
	Mode               string `gorm:"column:mode;size:32" json:"mode"`
}

func (ClientUpdatePolicy) TableName() string { return "client_update_policy" }
