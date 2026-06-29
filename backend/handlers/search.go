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

    // Sorting
    sortBy := c.DefaultQuery("sort_by", "created_at")
    sortOrder := strings.ToUpper(c.DefaultQuery("sort_order", "DESC"))
    if sortOrder != "ASC" && sortOrder != "DESC" {
        sortOrder = "DESC"
    }
    allowedSort := map[string]bool{"amount": true, "created_at": true}
    if !allowedSort[sortBy] {
        sortBy = "created_at"
    }

    // Build query depending on dialect
    var total int64
    var rows []map[string]interface{}

    dialect := h.db.Dialector.Name()
    if dialect == "postgres" {
        // Use full-text search on the persisted `search_vector` column
        tsQuery := q
        countSQL := "SELECT COUNT(*) FROM payments WHERE search_vector @@ plainto_tsquery(?)"
        h.db.Raw(countSQL, tsQuery).Scan(&total)

        sql := fmt.Sprintf(`SELECT id, sender_id, recipient_id, amount, currency, status, notes, created_at, ts_headline('english', notes, plainto_tsquery(?), 'StartSel=<em>, StopSel=</em>') AS notes_highlight FROM payments WHERE search_vector @@ plainto_tsquery(?) ORDER BY %s %s LIMIT ? OFFSET ?`, sortBy, sortOrder)
        h.db.Raw(sql, tsQuery, tsQuery, pageSize, offset).Scan(&rows)
    } else {
        // Fallback: simple LIKE search and amount equality if numeric
        like := "%%%s%%"
        likeQ := fmt.Sprintf(like, q)
        // count
        h.db.Model(&models.Payment{}).Where("notes LIKE ? OR currency LIKE ? OR status LIKE ?", likeQ, likeQ, likeQ).Count(&total)

        // query
        base := h.db.Model(&models.Payment{}).Select("id, sender_id, recipient_id, amount, currency, status, notes, created_at").Where("notes LIKE ? OR currency LIKE ? OR status LIKE ?", likeQ, likeQ, likeQ)
        if amt, err := strconv.ParseFloat(q, 64); err == nil {
            base = base.Or("amount = ?", amt)
        }
        base = base.Order(fmt.Sprintf("%s %s", sortBy, sortOrder)).Limit(pageSize).Offset(offset)
        var payments []models.Payment
        if err := base.Find(&payments).Error; err != nil {
            c.Error(errors.NewInternalError("Search failed", err))
            return
        }
        // build rows with highlight
        for _, p := range payments {
            notes := p.Notes
            highlight := notes
            if q != "" {
                highlight = strings.Replace(strings.ToLower(notes), strings.ToLower(q), "<em>"+q+"</em>", -1)
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
