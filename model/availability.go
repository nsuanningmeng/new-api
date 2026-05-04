package model

import (
	"context"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ModelAvailabilityBucket struct {
	Id           uint   `json:"id" gorm:"primaryKey"`
	BucketStart  int64  `json:"bucket_start" gorm:"not null;uniqueIndex:idx_availability_bucket;index:idx_availability_query,priority:2"`
	ModelName    string `json:"model_name" gorm:"type:varchar(191);not null;uniqueIndex:idx_availability_bucket;index:idx_availability_query,priority:1"`
	GroupName    string `json:"group_name" gorm:"type:varchar(191);not null;uniqueIndex:idx_availability_bucket"`
	SuccessCount int64  `json:"success_count" gorm:"not null;default:0"`
	TotalCount   int64  `json:"total_count" gorm:"not null;default:0"`
}

type rowAggregate struct {
	ModelName    string
	GroupName    string
	SuccessCount int64
	TotalCount   int64
}

func (ModelAvailabilityBucket) TableName() string {
	return "model_availability_buckets"
}

func UpsertAvailabilityBuckets(rows []ModelAvailabilityBucket) error {
	if len(rows) == 0 {
		return nil
	}
	successExpr := gorm.Expr("success_count + excluded.success_count")
	totalExpr := gorm.Expr("total_count + excluded.total_count")
	if common.UsingMySQL {
		successExpr = gorm.Expr("success_count + VALUES(success_count)")
		totalExpr = gorm.Expr("total_count + VALUES(total_count)")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "bucket_start"},
				{Name: "model_name"},
				{Name: "group_name"},
			},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"success_count": successExpr,
				"total_count":   totalExpr,
			}),
		}).CreateInBatches(rows, 200).Error
	})
}

func QueryAvailabilityRows(ctx context.Context, cutoffBucket int64, modelName string) ([]rowAggregate, error) {
	var rows []rowAggregate
	query := DB.WithContext(ctx).
		Model(&ModelAvailabilityBucket{}).
		Select("model_name, group_name, SUM(success_count) AS success_count, SUM(total_count) AS total_count").
		Where("bucket_start >= ?", cutoffBucket)
	if modelName != "" {
		query = query.Where("model_name = ?", modelName)
	}
	err := query.Group("model_name, group_name").Find(&rows).Error
	return rows, err
}
