package models

type LightwellAdvisoryRelease struct {
	AdvisoryUUID   string `json:"advisory_uuid" gorm:"primaryKey"`
	ReleaseVersion string `json:"release_version" gorm:"primaryKey"`
	RhlwBaseline   int    `json:"rhlw_baseline" gorm:"not null;default:0"`
	RhlwNovel      int    `json:"rhlw_novel" gorm:"not null;default:0"`
	RhlwHotfix     int    `json:"rhlw_hotfix" gorm:"not null;default:0"`
}

func (*LightwellAdvisoryRelease) TableName() string {
	return "lightwell_advisory_releases"
}
