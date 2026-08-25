package models

import (
	"time"

	"gorm.io/gorm"
)

type PaymentTemplate struct {
	ID               uint           `gorm:"primaryKey" json:"id"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
	UserID           uint           `gorm:"index;not null" json:"user_id"`
	TemplateName     string         `gorm:"size:255;not null" json:"template_name"`
	RecipientAccount string         `gorm:"size:56;not null" json:"recipient_account"`
	Amount           float64        `gorm:"not null" json:"amount"`
	AssetCode        string         `gorm:"size:12;not null" json:"asset_code"`
	AssetIssuer      string         `gorm:"size:56" json:"asset_issuer"`
	Notes            string         `gorm:"size:500" json:"notes"`
}

// TableName overrides the table name.
func (PaymentTemplate) TableName() string {
	return "payment_templates"
}

func CreatePaymentTemplate(db *gorm.DB, template *PaymentTemplate) error {
	return db.Create(template).Error
}

func GetPaymentTemplate(db *gorm.DB, id uint, userID uint) (*PaymentTemplate, error) {
	var template PaymentTemplate
	err := db.Where("id = ? AND user_id = ?", id, userID).First(&template).Error
	return &template, err
}

func ListPaymentTemplates(db *gorm.DB, userID uint) ([]PaymentTemplate, error) {
	var templates []PaymentTemplate
	err := db.Where("user_id = ?", userID).Find(&templates).Error
	return templates, err
}

func UpdatePaymentTemplate(db *gorm.DB, template *PaymentTemplate) error {
	return db.Save(template).Error
}

func DeletePaymentTemplate(db *gorm.DB, id uint, userID uint) error {
	return db.Where("id = ? AND user_id = ?", id, userID).Delete(&PaymentTemplate{}).Error
}
