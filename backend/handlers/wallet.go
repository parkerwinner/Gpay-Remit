package handlers

import (
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/Gpay-Remit/gpay-remit/config"
	"github.com/Gpay-Remit/gpay-remit/errors"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/Gpay-Remit/gpay-remit/services"
	"github.com/Gpay-Remit/gpay-remit/utils"
	"github.com/Gpay-Remit/gpay-remit/validators"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RateLookup abstracts ExchangeRateService for testability.
type RateLookup interface {
	GetRate(from, to string) (*services.ExchangeRate, error)
}

type WalletHandler struct {
	db            *gorm.DB
	config        *config.Config
	stellarClient utils.StellarClientInterface
	rateService   RateLookup
}

func NewWalletHandler(db *gorm.DB, cfg *config.Config, stellarClient utils.StellarClientInterface) *WalletHandler {
	return &WalletHandler{
		db:            db,
		config:        cfg,
		stellarClient: stellarClient,
		rateService:   services.NewExchangeRateService(cfg),
	}
}

type WalletBalanceItem struct {
	AssetCode       string     `json:"asset_code"`
	AssetIssuer     string     `json:"asset_issuer,omitempty"`
	Balance         string     `json:"balance"`
	EstimatedValue  string     `json:"estimated_value,omitempty"`
	RateAvailable   bool       `json:"rate_available"`
	RateTimestamp   *time.Time `json:"rate_timestamp,omitempty"`
	RateSource      string     `json:"rate_source,omitempty"`
}

type UpdateWalletPreferenceRequest struct {
	PreferredCurrency string `json:"preferred_currency" binding:"required"`
}

func (h *WalletHandler) GetBalances(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	userID := userIDVal.(uint)

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewUnauthorizedError("Authenticated user not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to load user", err))
		}
		return
	}

	preferred := strings.ToUpper(strings.TrimSpace(user.DefaultCurrency))
	if !validators.IsSupportedCurrency(preferred) {
		preferred = "USD"
	}

	var rawBalances []utils.AccountBalance
	if user.StellarAddress != "" && h.stellarClient != nil {
		ctx := utils.WithRequestContext(c.Request.Context(), c.GetString("requestID"), userID)
		fetched, err := h.stellarClient.GetAccountBalances(ctx, user.StellarAddress)
		if err != nil {
			c.Error(errors.NewInternalError("Failed to retrieve wallet balances", err))
			return
		}
		rawBalances = fetched
	}

	items := make([]WalletBalanceItem, 0, len(rawBalances))
	totalRat := new(big.Rat)
	hasUnconverted := false
	var latestRateTime *time.Time

	for _, b := range rawBalances {
		code := strings.ToUpper(strings.TrimSpace(b.AssetCode))
		item := WalletBalanceItem{
			AssetCode:   code,
			AssetIssuer: b.AssetIssuer,
			Balance:     b.Balance,
		}

		balRat, ok := new(big.Rat).SetString(strings.TrimSpace(b.Balance))
		if !ok {
			hasUnconverted = true
			items = append(items, item)
			continue
		}

		if code == preferred {
			item.EstimatedValue = balRat.FloatString(2)
			item.RateAvailable = true
			totalRat.Add(totalRat, balRat)
			items = append(items, item)
			continue
		}

		if h.rateService != nil {
			rate, err := h.rateService.GetRate(code, preferred)
			if err == nil && rate != nil && rate.Scale > 0 && rate.Rate > 0 {
				rateRat := big.NewRat(rate.Rate, rate.Scale)
				converted := new(big.Rat).Mul(balRat, rateRat)
				item.EstimatedValue = converted.FloatString(2)
				item.RateAvailable = true
				ts := rate.Timestamp.UTC()
				item.RateTimestamp = &ts
				item.RateSource = rate.Origin
				totalRat.Add(totalRat, converted)
				if latestRateTime == nil || ts.After(*latestRateTime) {
					latestRateTime = &ts
				}
				items = append(items, item)
				continue
			}
		}

		hasUnconverted = true
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"balances":                 items,
		"preferred_currency":       preferred,
		"supported_currencies":     validators.SupportedCurrencies,
		"estimated_total":          totalRat.FloatString(2),
		"has_unconverted_balances": hasUnconverted,
		"rate_timestamp":           latestRateTime,
		"is_estimate":              true,
	})
}

func (h *WalletHandler) GetPreferences(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	userID := userIDVal.(uint)

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewUnauthorizedError("Authenticated user not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to load user", err))
		}
		return
	}

	preferred := strings.ToUpper(strings.TrimSpace(user.DefaultCurrency))
	if !validators.IsSupportedCurrency(preferred) {
		preferred = "USD"
	}

	c.JSON(http.StatusOK, gin.H{
		"preferred_currency":   preferred,
		"supported_currencies": validators.SupportedCurrencies,
	})
}

func (h *WalletHandler) UpdatePreferences(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}
	userID := userIDVal.(uint)

	var req UpdateWalletPreferenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	currency := strings.ToUpper(strings.TrimSpace(req.PreferredCurrency))
	if err := validators.ValidateCurrency(currency); err != nil {
		c.Error(errors.NewValidationError("Unsupported preferred_currency", err.Error()))
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewUnauthorizedError("Authenticated user not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to load user", err))
		}
		return
	}

	if err := h.db.Model(&user).Update("default_currency", currency).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to update currency preference", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"preferred_currency":   currency,
		"supported_currencies": validators.SupportedCurrencies,
	})
}
