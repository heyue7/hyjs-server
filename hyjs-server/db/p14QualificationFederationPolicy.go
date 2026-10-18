package db

// P14QualificationFederationPolicy 独立门禁发布原服互信的真实晋级证书，初始关闭。
type P14QualificationFederationPolicy struct {
	ID      int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled bool   `gorm:"not null;default:false" json:"enabled"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (P14QualificationFederationPolicy) TableName() string {
	return "p14_qualification_federation_policies"
}
