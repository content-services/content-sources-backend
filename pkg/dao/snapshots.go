package dao

import (
	"gorm.io/gorm"
)

type snapshotDaoImpl struct {
	db *gorm.DB
}

func (sDao snapshotDaoImpl) Create(s *Snapshot) error {
	trans := sDao.db.Create(s)
	if trans.Error != nil {
		return trans.Error
	}
	return nil
}

func (sDao snapshotDaoImpl) List(repoConfigUuid string) ([]Snapshot, error) {
	var snaps []Snapshot

	result := sDao.db.Model(&Snapshot{}).
		Joins("inner join repository_configurations on repository_configurations.repository_uuid = snapshots.repository_uuid").
		Where("repository_configurations.uuid = ?", repoConfigUuid).
		Where("snapshots.org_id = repository_configurations.org_id").
		Find(&snaps)
	if result.Error != nil {
		return snaps, result.Error
	}
	return snaps, nil
}
