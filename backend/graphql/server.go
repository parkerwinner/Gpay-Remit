package graphql

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yourusername/gpay-remit/config"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/gorm"
)

type Server struct {
	db  *gorm.DB
	cfg *config.Config
}

func NewServer(db *gorm.DB, cfg *config.Config) *Server {
	return &Server{
		db:  db,
		cfg: cfg,
	}
}

func (s *Server) PlaygroundHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		html := `<!DOCTYPE html>
<html>
<head>
    <title>Gpay-Remit GraphQL Playground</title>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/graphql-playground-react/build/static/css/index.css" />
    <link rel="shortcut icon" href="https://cdn.jsdelivr.net/npm/graphql-playground-react/build/favicon.png" />
    <script src="https://cdn.jsdelivr.net/npm/graphql-playground-react/build/static/js/middleware.js"></script>
</head>
<body>
    <div id="root">
        <style>
            body { background-color: #172b4d; font-family: Open Sans, sans-serif; height: 100vh; margin: 0; overflow: hidden; }
            #root { height: 100%; }
        </style>
        <script>
            window.addEventListener('load', function (event) {
                GraphQLPlayground.init(document.getElementById('root'), {
                    endpoint: '/graphql'
                })
            })
        </script>
    </div>
</body>
</html>`
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, html)
	}
}

func (s *Server) QueryHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RequestBody
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"errors": []gin.H{{"message": "Invalid JSON request: " + err.Error()}}})
			return
		}

		query := strings.TrimSpace(req.Query)
		if query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"errors": []gin.H{{"message": "Query cannot be empty"}}})
			return
		}

		userIDVal, _ := c.Get("userID")
		var userID uint
		if u, ok := userIDVal.(uint); ok {
			userID = u
		}

		res, err := s.ExecuteQuery(query, req.Variables, userID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"data":   res,
				"errors": []gin.H{{"message": err.Error()}},
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"data": res,
		})
	}
}

func (s *Server) ExecuteQuery(query string, variables map[string]interface{}, currentUserID uint) (map[string]interface{}, error) {
	data := make(map[string]interface{})

	// Handle introspection query (__schema)
	if strings.Contains(query, "__schema") || strings.Contains(query, "__type") {
		data["__schema"] = gin.H{
			"types": []gin.H{
				{"name": "User", "kind": "OBJECT"},
				{"name": "Contact", "kind": "OBJECT"},
				{"name": "Payment", "kind": "OBJECT"},
				{"name": "ExchangeRate", "kind": "OBJECT"},
				{"name": "Query", "kind": "OBJECT"},
				{"name": "Mutation", "kind": "OBJECT"},
			},
		}
		return data, nil
	}

	// 1. Query: me
	if strings.Contains(query, "me") {
		if currentUserID == 0 {
			data["me"] = nil
		} else {
			var user models.User
			if err := s.db.First(&user, currentUserID).Error; err == nil {
				data["me"] = UserGQL{
					ID:             fmt.Sprintf("%d", user.ID),
					Email:          user.Email,
					Name:           user.Name,
					StellarAddress: user.StellarAddress,
					Role:           user.Role,
					Country:        user.Country,
					IsActive:       user.IsActive,
				}
			} else {
				data["me"] = nil
			}
		}
	}

	// 2. Query: contacts
	if strings.Contains(query, "contacts") {
		var contacts []models.Contact
		s.db.Where("user_id = ?", currentUserID).Order("created_at DESC").Find(&contacts)
		resList := make([]ContactGQL, 0, len(contacts))
		for _, ct := range contacts {
			resList = append(resList, ContactGQL{
				ID:                 fmt.Sprintf("%d", ct.ID),
				UserID:             fmt.Sprintf("%d", ct.UserID),
				Nickname:           ct.Nickname,
				StellarAddress:     ct.StellarAddress,
				Currency:           ct.Currency,
				Email:              ct.Email,
				Notes:              ct.Notes,
				VerificationStatus: string(ct.VerificationStatus),
				IsVerified:         ct.IsVerified,
			})
		}
		data["contacts"] = resList
	}

	// 3. Query: contact(id: ...)
	if strings.Contains(query, "contact(") {
		idStr := extractParam(query, "id")
		idNum, _ := strconv.ParseUint(idStr, 10, 32)
		var ct models.Contact
		if err := s.db.Where("id = ? AND user_id = ?", uint(idNum), currentUserID).First(&ct).Error; err == nil {
			data["contact"] = ContactGQL{
				ID:                 fmt.Sprintf("%d", ct.ID),
				UserID:             fmt.Sprintf("%d", ct.UserID),
				Nickname:           ct.Nickname,
				StellarAddress:     ct.StellarAddress,
				Currency:           ct.Currency,
				Email:              ct.Email,
				Notes:              ct.Notes,
				VerificationStatus: string(ct.VerificationStatus),
				IsVerified:         ct.IsVerified,
			}
		} else {
			data["contact"] = nil
		}
	}

	// 4. Query: payments
	if strings.Contains(query, "payments") {
		var payments []models.Payment
		s.db.Where("sender_id = ? OR recipient_id = ?", currentUserID, currentUserID).Order("created_at DESC").Limit(50).Find(&payments)
		resList := make([]PaymentGQL, 0, len(payments))
		for _, p := range payments {
			resList = append(resList, PaymentGQL{
				ID:               fmt.Sprintf("%d", p.ID),
				SenderID:         fmt.Sprintf("%d", p.SenderID),
				SenderAccount:    p.SenderAccount,
				RecipientID:      fmt.Sprintf("%d", p.RecipientID),
				RecipientAccount: p.RecipientAccount,
				Amount:           p.Amount,
				Currency:         p.Currency,
				Status:           p.Status,
				TxHash:           p.TxHash,
				Fee:              p.Fee,
				CreatedAt:        p.CreatedAt.Format(time.RFC3339),
			})
		}
		data["payments"] = resList
	}

	// 5. Query: exchangeRate
	if strings.Contains(query, "exchangeRate") {
		base := extractParam(query, "base")
		if base == "" {
			base = "USD"
		}
		target := extractParam(query, "target")
		if target == "" {
			target = "EUR"
		}
		data["exchangeRate"] = ExchangeRateGQL{
			Base:      base,
			Target:    target,
			Rate:      0.92,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
	}

	// 6. Mutation: createContact
	if strings.Contains(query, "createContact") {
		var input CreateContactInput
		if variables != nil && variables["input"] != nil {
			b, _ := json.Marshal(variables["input"])
			json.Unmarshal(b, &input)
		}
		if input.Nickname == "" {
			input.Nickname = extractParam(query, "nickname")
		}
		if input.StellarAddress == "" {
			input.StellarAddress = extractParam(query, "stellarAddress")
		}

		contact := models.Contact{
			UserID:             currentUserID,
			Nickname:           input.Nickname,
			StellarAddress:     input.StellarAddress,
			Currency:           input.Currency,
			Email:              input.Email,
			Notes:              input.Notes,
			VerificationStatus: models.ContactVerificationPending,
		}
		if contact.Currency == "" {
			contact.Currency = "USDC"
		}
		if err := contact.Validate(); err != nil {
			return nil, err
		}
		if err := s.db.Create(&contact).Error; err != nil {
			return nil, err
		}
		data["createContact"] = ContactGQL{
			ID:                 fmt.Sprintf("%d", contact.ID),
			UserID:             fmt.Sprintf("%d", contact.UserID),
			Nickname:           contact.Nickname,
			StellarAddress:     contact.StellarAddress,
			Currency:           contact.Currency,
			Email:              contact.Email,
			Notes:              contact.Notes,
			VerificationStatus: string(contact.VerificationStatus),
			IsVerified:         contact.IsVerified,
		}
	}

	// 7. Mutation: deleteContact
	if strings.Contains(query, "deleteContact") {
		idStr := extractParam(query, "id")
		idNum, _ := strconv.ParseUint(idStr, 10, 32)
		if err := s.db.Where("id = ? AND user_id = ?", uint(idNum), currentUserID).Delete(&models.Contact{}).Error; err == nil {
			data["deleteContact"] = true
		} else {
			data["deleteContact"] = false
		}
	}

	return data, nil
}

func extractParam(s, param string) string {
	idx := strings.Index(s, param)
	if idx == -1 {
		return ""
	}
	sub := s[idx+len(param):]
	colon := strings.Index(sub, ":")
	if colon == -1 {
		return ""
	}
	sub = strings.TrimSpace(sub[colon+1:])
	sub = strings.Trim(sub, "\"' ")
	delims := []string{",", " ", ")", "\n", "\t", "}"}
	for _, d := range delims {
		if p := strings.Index(sub, d); p != -1 {
			sub = sub[:p]
		}
	}
	return strings.Trim(sub, "\"' ")
}
