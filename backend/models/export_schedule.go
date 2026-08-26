package models

import "gorm.io/gorm"

// ExportSchedule tracks automated scheduled transaction exports
type ExportSchedule struct {
	gorm.Model
	UserID      uint   `json:"user_id"`
	Frequency   string `json:"frequency"`   // daily, weekly, monthly
	Format      string `json:"format"`      // csv, pdf
	Destination string `json:"destination"` // email, s3
	IsActive    bool   `json:"is_active" gorm:"default:true"`
}
