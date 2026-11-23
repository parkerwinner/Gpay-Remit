package services

import (
	"encoding/json"
	"fmt"

	"github.com/Gpay-Remit/gpay-remit/models"
	"gorm.io/gorm"
)

// RemittanceCreateParams holds validated parameters for creating a pending remittance record.
type RemittanceCreateParams struct {
	SenderID         uint
	SenderAccount    string
	RecipientID      uint
	RecipientAccount string
	Amount           int64
	AssetCode        string
	TargetCurrency   string
	Conditions       map[string]interface{}
	Notes            string
}

// CreateRemittanceInTx creates a pending Payment record inside the provided database transaction.
// External side effects (Stellar submission, email notifications) are intentionally excluded so
// database atomicity is preserved.
func CreateRemittanceInTx(tx *gorm.DB, fees *FeeService, params RemittanceCreateParams) (*models.Payment, FeeBreakdown, error) {
	if params.Amount <= 0 {
		return nil, FeeBreakdown{}, fmt.Errorf("amount must be positive")
	}
	if params.AssetCode == "" {
		return nil, FeeBreakdown{}, fmt.Errorf("asset_code is required")
	}

	var feeBreakdown FeeBreakdown
	if fees != nil {
		feeBreakdown = fees.Calculate(params.Amount)
	}

	var conditionsStr string
	if len(params.Conditions) > 0 {
		raw, err := json.Marshal(params.Conditions)
		if err != nil {
			return nil, FeeBreakdown{}, fmt.Errorf("failed to marshal conditions: %w", err)
		}
		conditionsStr = string(raw)
	}

	payment := models.Payment{
		SenderID:         params.SenderID,
		SenderAccount:    params.SenderAccount,
		RecipientID:      params.RecipientID,
		RecipientAccount: params.RecipientAccount,
		Amount:           params.Amount,
		Currency:         params.AssetCode,
		TargetCurrency:   params.TargetCurrency,
		Status:           "pending",
		Fee:              feeBreakdown.TotalFee,
		PlatformFee:      feeBreakdown.PlatformFee,
		ForexFee:         feeBreakdown.ForexFee,
		ComplianceFee:    feeBreakdown.ComplianceFee,
		NetworkFee:       feeBreakdown.NetworkFee,
		Conditions:       conditionsStr,
		Notes:            params.Notes,
	}

	if err := tx.Create(&payment).Error; err != nil {
		return nil, FeeBreakdown{}, err
	}

	return &payment, feeBreakdown, nil
}
