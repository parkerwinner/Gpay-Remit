package handlers

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Gpay-Remit/gpay-remit/errors"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UpdateRemittanceTagsRequest struct {
	Tags []string `json:"tags" binding:"required"`
}

type TagAssetTotal struct {
	Asset string `json:"asset"`
	Total int64  `json:"total"`
}

type TagSummaryItem struct {
	Tag          string          `json:"tag"`
	PaymentCount int64           `json:"payment_count"`
	Totals       []TagAssetTotal `json:"totals"`
}

type TagAutocompleteItem struct {
	Tag   string `json:"tag"`
	Count int64  `json:"count"`
}

func populatePaymentTags(db *gorm.DB, payments []models.Payment, userID *uint) error {
	if len(payments) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(payments))
	for i := range payments {
		payments[i].Tags = []string{}
		ids = append(ids, payments[i].ID)
	}

	var rows []models.PaymentTag
	q := db.Where("payment_id IN ?", ids)
	if userID != nil {
		q = q.Where("user_id = ?", *userID)
	}
	if err := q.Order("tag ASC").Find(&rows).Error; err != nil {
		return err
	}

	byPayment := make(map[uint][]string, len(payments))
	for _, r := range rows {
		byPayment[r.PaymentID] = append(byPayment[r.PaymentID], r.Tag)
	}
	for i := range payments {
		if tags, ok := byPayment[payments[i].ID]; ok {
			payments[i].Tags = tags
		}
	}
	return nil
}

func extractUserID(c *gin.Context) (uint, bool) {
	uid, exists := c.Get("userID")
	if !exists {
		return 0, false
	}
	id, ok := uid.(uint)
	return id, ok && id > 0
}

func escapeLikePrefix(prefix string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(prefix) + "%"
}

// UpdateRemittanceTags replaces the authenticated user's tags on an existing payment.
func (h *RemittanceHandler) UpdateRemittanceTags(c *gin.Context) {
	userID, ok := extractUserID(c)
	if !ok {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}

	id64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id64 == 0 {
		c.Error(errors.NewValidationError("Invalid payment ID", nil))
		return
	}
	paymentID := uint(id64)

	var req UpdateRemittanceTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	normalizedTags, err := models.NormalizeAndValidateTags(req.Tags)
	if err != nil {
		c.Error(err)
		return
	}

	var payment models.Payment
	if err := h.db.Where("id = ? AND sender_id = ?", paymentID, userID).First(&payment).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.Error(errors.NewNotFoundError("Payment not found"))
		} else {
			c.Error(errors.NewInternalError("Failed to fetch payment", err))
		}
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("payment_id = ? AND user_id = ?", paymentID, userID).Delete(&models.PaymentTag{}).Error; err != nil {
			return err
		}
		for _, tag := range normalizedTags {
			if err := tx.Create(&models.PaymentTag{
				PaymentID: paymentID,
				UserID:    userID,
				Tag:       tag,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		c.Error(errors.NewInternalError("Failed to update payment tags", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"payment_id": paymentID,
		"tags":       normalizedTags,
	})
}

// GetTagSummary returns per-tag completed payment counts and per-asset totals
// strictly scoped to the authenticated user. Assets are never merged.
func (h *RemittanceHandler) GetTagSummary(c *gin.Context) {
	userID, ok := extractUserID(c)
	if !ok {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}

	query := preferredReadDB(h.db, h.readDB).
		Table("payment_tags pt").
		Select("pt.tag AS tag, p.currency AS asset, COUNT(DISTINCT p.id) AS payment_count, COALESCE(SUM(p.amount), 0) AS total").
		Joins("INNER JOIN payments p ON p.id = pt.payment_id").
		Where("pt.user_id = ? AND p.deleted_at IS NULL AND p.status = ?", userID, "completed")

	if fromStr := strings.TrimSpace(c.Query("from")); fromStr != "" {
		fromTime, err := parseTagSummaryDate(fromStr, false)
		if err != nil {
			c.Error(errors.NewValidationError("Invalid 'from' date parameter", err.Error()))
			return
		}
		query = query.Where("p.created_at >= ?", fromTime)
	}
	if toStr := strings.TrimSpace(c.Query("to")); toStr != "" {
		toTime, err := parseTagSummaryDate(toStr, true)
		if err != nil {
			c.Error(errors.NewValidationError("Invalid 'to' date parameter", err.Error()))
			return
		}
		query = query.Where("p.created_at <= ?", toTime)
	}

	type summaryRow struct {
		Tag          string
		Asset        string
		PaymentCount int64
		Total        int64
	}
	var rows []summaryRow
	if err := query.Group("pt.tag, p.currency").Order("pt.tag ASC, p.currency ASC").Scan(&rows).Error; err != nil {
		c.Error(errors.NewInternalError("Failed to compute tag summary", err))
		return
	}

	byTag := make(map[string]*TagSummaryItem)
	order := make([]string, 0)
	for _, r := range rows {
		item, exists := byTag[r.Tag]
		if !exists {
			item = &TagSummaryItem{
				Tag:    r.Tag,
				Totals: []TagAssetTotal{},
			}
			byTag[r.Tag] = item
			order = append(order, r.Tag)
		}
		item.PaymentCount += r.PaymentCount
		item.Totals = append(item.Totals, TagAssetTotal{
			Asset: r.Asset,
			Total: r.Total,
		})
	}

	sort.Strings(order)
	result := make([]TagSummaryItem, 0, len(order))
	for _, t := range order {
		result = append(result, *byTag[t])
	}

	c.JSON(http.StatusOK, gin.H{"tags": result})
}

func parseTagSummaryDate(raw string, endOfDay bool) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, err
	}
	if endOfDay {
		return t.Add(23*time.Hour + 59*time.Minute + 59*time.Second), nil
	}
	return t, nil
}

// AutocompleteTags returns prefix-matched tags for the authenticated user,
// ordered by usage count descending then alphabetically.
func (h *RemittanceHandler) AutocompleteTags(c *gin.Context) {
	userID, ok := extractUserID(c)
	if !ok {
		c.Error(errors.NewUnauthorizedError("Unauthorized"))
		return
	}

	prefix := models.NormalizeTag(c.Query("q"))
	if prefix == "" {
		c.JSON(http.StatusOK, gin.H{"tags": []TagAutocompleteItem{}})
		return
	}

	limit := 10
	if lStr := c.Query("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			if l > 25 {
				l = 25
			}
			limit = l
		}
	}

	var items []TagAutocompleteItem
	pattern := escapeLikePrefix(prefix)
	err := preferredReadDB(h.db, h.readDB).
		Table("payment_tags").
		Select("tag, COUNT(*) AS count").
		Where("user_id = ? AND tag LIKE ? ESCAPE '\\'", userID, pattern).
		Group("tag").
		Order("count DESC, tag ASC").
		Limit(limit).
		Scan(&items).Error
	if err != nil {
		c.Error(errors.NewInternalError("Failed to autocomplete tags", err))
		return
	}
	if items == nil {
		items = []TagAutocompleteItem{}
	}

	c.JSON(http.StatusOK, gin.H{"tags": items})
}
