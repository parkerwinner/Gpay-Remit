package handlers

import (
    "fmt"
    "net/http"
    "strconv"
    "strings"

    "github.com/gin-gonic/gin"
    "github.com/yourusername/gpay-remit/config"
    "github.com/yourusername/gpay-remit/errors"
    "github.com/yourusername/gpay-remit/models"
    "gorm.io/gorm"
)

type SearchHandler struct {
    db     *gorm.DB
    config *config.Config
}

func NewSearchHandler(db *gorm.DB, cfg *config.Config) *SearchHandler {
    return &SearchHandler{db: db, config: cfg}
}

// SearchTransactions handles GET /api/v1/search/transactions?q=...
func (h *SearchHandler) SearchTransactions(c *gin.Context) {
    q := strings.TrimSpace(c.Query("q"))
    if q == "" {
        c.Error(errors.NewValidationError("Missing query parameter", "q is required"))
        return
    }

    // Pagination
    page := 1
    pageSize := 20
    if p := c.Query("page"); p != "" {
        if v, err := strconv.Atoi(p); err == nil && v > 0 {
            page = v
        }
    }
    if ps := c.Query("page_size"); ps != "" {
        if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 100 {
            pageSize = v
        }
    }
    offset := (page - 1) * pageSize

    var minAmount *float64
    if minAmtStr := c.Query("min_amount"); minAmtStr != "" {
        if val, err := strconv.ParseFloat(minAmtStr, 64); err == nil {
            minAmount = &val
        }
    }

    var maxAmount *float64
    if maxAmtStr := c.Query("max_amount"); maxAmtStr != "" {
        if val, err := strconv.ParseFloat(maxAmtStr, 64); err == nil {
            maxAmount = &val
        }
    }

    var filterCurrency string
    if cStr := c.Query("currency"); cStr != "" {
        filterCurrency = cStr
    }

    // Sorting - validate and sanitize sort parameters
    sortBy := c.DefaultQuery("sort_by", "created_at")
    sortOrder := strings.ToUpper(c.DefaultQuery("sort_order", "DESC"))
    if sortOrder != "ASC" && sortOrder != "DESC" {
        sortOrder = "DESC"
    }
    allowedSort := map[string]bool{"amount": true, "created_at": true, "status": true, "currency": true}
    if !allowedSort[sortBy] {
        sortBy = "created_at"
    }

    // Build query using parameterized queries to prevent SQL injection
    var total int64
    var rows []map[string]interface{}

    dialect := h.db.Dialector.Name()
    if dialect == "postgres" {
        // Use full-text search with parameterized query
        // Count total results
        countQuery := h.db.Model(&models.Payment{}).
            Where("search_vector @@ plainto_tsquery(?)", q)
        if minAmount != nil {
            countQuery = countQuery.Where("amount >= ?", *minAmount)
        }
        if maxAmount != nil {
            countQuery = countQuery.Where("amount <= ?", *maxAmount)
        }
        if filterCurrency != "" {
            countQuery = countQuery.Where("currency = ?", filterCurrency)
        }
        countQuery.Count(&total)

        // Query with parameterized values and safe column names
        var payments []models.Payment
        query := h.db.Model(&models.Payment{}).
            Select("id, sender_id, recipient_id, amount, currency, status, notes, created_at").
            Where("search_vector @@ plainto_tsquery(?)", q)
        
        if minAmount != nil {
            query = query.Where("amount >= ?", *minAmount)
        }
        if maxAmount != nil {
            query = query.Where("amount <= ?", *maxAmount)
        }
        if filterCurrency != "" {
            query = query.Where("currency = ?", filterCurrency)
        }

        query = query.Order(fmt.Sprintf("%s %s", sortBy, sortOrder)).
            Limit(pageSize).
            Offset(offset)
        
        if err := query.Find(&payments).Error; err != nil {
            c.Error(errors.NewInternalError("Search failed", err))
            return
        }

        // Build response with highlighted results
        for _, p := range payments {
            // Use GORM's parameterized query for ts_headline
            var highlightResult struct {
                Highlight string
            }
            h.db.Raw(
                "SELECT ts_headline('english', ?, plainto_tsquery(?), 'StartSel=<em>, StopSel=</em>') as highlight",
                p.Notes,
                q,
            ).Scan(&highlightResult)

            rows = append(rows, map[string]interface{}{
                "id":              p.ID,
                "sender_id":       p.SenderID,
                "recipient_id":    p.RecipientID,
                "amount":          p.Amount,
                "currency":        p.Currency,
                "status":          p.Status,
                "notes":           p.Notes,
                "notes_highlight": highlightResult.Highlight,
                "created_at":      p.CreatedAt,
            })
        }
    } else {
        // Fallback: use parameterized LIKE queries
        likeQ := "%" + q + "%"
        
        countQuery := h.db.Model(&models.Payment{})
        // Count with parameterized query
        if amt, err := strconv.ParseFloat(q, 64); err == nil {
            countQuery = countQuery.Where("notes LIKE ? OR currency LIKE ? OR status LIKE ? OR amount = ?", likeQ, likeQ, likeQ, amt)
        } else {
            countQuery = countQuery.Where("notes LIKE ? OR currency LIKE ? OR status LIKE ?", likeQ, likeQ, likeQ)
        }
        
        if minAmount != nil {
            countQuery = countQuery.Where("amount >= ?", *minAmount)
        }
        if maxAmount != nil {
            countQuery = countQuery.Where("amount <= ?", *maxAmount)
        }
        if filterCurrency != "" {
            countQuery = countQuery.Where("currency = ?", filterCurrency)
        }
        countQuery.Count(&total)

        // Query with parameterized values
        query := h.db.Model(&models.Payment{}).
            Select("id, sender_id, recipient_id, amount, currency, status, notes, created_at")
            
        if amt, err := strconv.ParseFloat(q, 64); err == nil {
            query = query.Where("notes LIKE ? OR currency LIKE ? OR status LIKE ? OR amount = ?", likeQ, likeQ, likeQ, amt)
        } else {
            query = query.Where("notes LIKE ? OR currency LIKE ? OR status LIKE ?", likeQ, likeQ, likeQ)
        }
        
        if minAmount != nil {
            query = query.Where("amount >= ?", *minAmount)
        }
        if maxAmount != nil {
            query = query.Where("amount <= ?", *maxAmount)
        }
        if filterCurrency != "" {
            query = query.Where("currency = ?", filterCurrency)
        }
        
        query = query.Order(fmt.Sprintf("%s %s", sortBy, sortOrder)).
            Limit(pageSize).
            Offset(offset)

        var payments []models.Payment
        if err := query.Find(&payments).Error; err != nil {
            c.Error(errors.NewInternalError("Search failed", err))
            return
        }
        
        // Build rows with simple highlight
        for _, p := range payments {
            notes := p.Notes
            highlight := notes
            if q != "" && strings.Contains(strings.ToLower(notes), strings.ToLower(q)) {
                // Simple case-insensitive highlighting
                lowerNotes := strings.ToLower(notes)
                lowerQ := strings.ToLower(q)
                idx := strings.Index(lowerNotes, lowerQ)
                if idx != -1 {
                    highlight = notes[:idx] + "<em>" + notes[idx:idx+len(q)] + "</em>" + notes[idx+len(q):]
                }
            }
            rows = append(rows, map[string]interface{}{
                "id":              p.ID,
                "sender_id":       p.SenderID,
                "recipient_id":    p.RecipientID,
                "amount":          p.Amount,
                "currency":        p.Currency,
                "status":          p.Status,
                "notes":           p.Notes,
                "notes_highlight": highlight,
                "created_at":      p.CreatedAt,
            })
        }
    }

    c.JSON(http.StatusOK, gin.H{
        "meta": gin.H{"total": total, "page": page, "page_size": pageSize},
        "data": rows,
    })
}
