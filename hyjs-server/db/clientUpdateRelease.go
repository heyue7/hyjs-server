package db

// ClientUpdateRelease 保存客户端更新的独立运行事实。
type ClientUpdateRelease struct {
	ID                 int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	BuildCode          int64  `gorm:"column:build_code;not null" json:"buildCode"`
	VersionName        string `gorm:"column:version_name;size:80" json:"versionName"`
	Title              string `gorm:"column:title;size:240" json:"title"`
	Content            string `gorm:"column:content;type:text" json:"content"`
	Mode               string `gorm:"column:mode;size:32" json:"mode"`
	MinLoginBuild      int64  `gorm:"column:min_login_build;not null" json:"minLoginBuild"`
	MinRunBuild        int64  `gorm:"column:min_run_build;not null" json:"minRunBuild"`
	MaintenanceStartAt int64  `gorm:"column:maintenance_start_at;not null" json:"maintenanceStartAt"`
	MaintenanceEndAt   int64  `gorm:"column:maintenance_end_at;not null" json:"maintenanceEndAt"`
	State              string `gorm:"column:state;size:32" json:"state"`
	PublishedAt        int64  `gorm:"column:published_at;not null" json:"publishedAt"`
	UpdatedAt          int64  `gorm:"column:updated_at;autoUpdateTime:false" json:"updatedAt"`
	PublishedBy        string `gorm:"column:published_by;size:80" json:"publishedBy"`
}

func (ClientUpdateRelease) TableName() string { return "client_update_releases" }
