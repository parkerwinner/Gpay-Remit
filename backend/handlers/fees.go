package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/errors"
	"github.com/yourusername/gpay-remit/services"
)

type FeeHandler struct {
	fees *services.FeeService
}

func NewFeeHandler(fees *services.FeeService) *FeeHandler {
	return &FeeHandler{fees: fees}
}

func (h *FeeHandler) Calculate(c *gin.Context) {
	amountStr := c.Query("amount")
	if amountStr == "" {
		c.Error(errors.NewValidationError("amount is required", "missing amount query param"))
		return
	}

	var amount float64
	if _, err := fmt.Sscanf(amountStr, "%f", &amount); err != nil || amount <= 0 {
		c.Error(errors.NewValidationError("invalid amount", "amount must be a positive number"))
		return
	}

	breakdown := h.fees.Calculate(amount)
	c.JSON(http.StatusOK, breakdown)
}
