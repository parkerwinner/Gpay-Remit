package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/config"
	"github.com/yourusername/gpay-remit/errors"
	"github.com/yourusername/gpay-remit/services"
	"github.com/yourusername/gpay-remit/utils"
)

const exchangeRateCacheTTL = 5 * time.Minute

type ExchangeRateHandler struct {
	service *services.ExchangeRateService
}

func NewExchangeRateHandler(cfg *config.Config) *ExchangeRateHandler {
	return &ExchangeRateHandler{
		service: services.NewExchangeRateService(cfg),
	}
}

// GetRate handles GET /exchange-rates?from=USD&to=EUR
func (h *ExchangeRateHandler) GetRate(c *gin.Context) {
	from := strings.ToUpper(strings.TrimSpace(c.Query("from")))
	to := strings.ToUpper(strings.TrimSpace(c.Query("to")))

	if from == "" || to == "" {
		c.Error(errors.NewValidationError("from and to are required", "missing from/to query params"))
		return
	}

	if len(from) != 3 || len(to) != 3 {
		c.Error(errors.NewValidationError("invalid currency code", "from/to must be 3-letter ISO currency codes"))
		return
	}

	cacheKey := fmt.Sprintf("exchange-rate:%s:%s", from, to)

	var cached services.ExchangeRate
	if found, err := utils.GetCached(cacheKey, &cached); err == nil && found {
		c.JSON(http.StatusOK, cached)
		return
	}

	rate, err := h.service.GetRate(from, to)
	if err != nil {
		c.Error(errors.NewInternalError("Failed to retrieve exchange rate", err))
		return
	}

	utils.SetCached(cacheKey, rate, exchangeRateCacheTTL)

	c.JSON(http.StatusOK, rate)
}
