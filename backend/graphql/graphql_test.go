package graphql

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/gpay-remit/config"
	"github.com/yourusername/gpay-remit/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestGraphQLServer() (*Server, *gin.Engine, *gorm.DB) {
	gin.SetMode(gin.TestMode)
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	db.AutoMigrate(&models.User{}, &models.Contact{}, &models.Payment{})

	cfg := &config.Config{}
	server := NewServer(db, cfg)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", uint(1))
		c.Next()
	})

	router.GET("/playground", server.PlaygroundHandler())
	router.POST("/graphql", server.QueryHandler())

	return server, router, db
}

func TestGraphQL_Playground(t *testing.T) {
	_, router, _ := setupTestGraphQLServer()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/playground", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "GraphQL Playground")
}

func TestGraphQL_Introspection(t *testing.T) {
	_, router, _ := setupTestGraphQLServer()

	body, _ := json.Marshal(RequestBody{
		Query: "{ __schema { types { name kind } } }",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/graphql", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "User")
}

func TestGraphQL_MeQuery(t *testing.T) {
	_, router, db := setupTestGraphQLServer()

	user := models.User{
		Email:          "alice@example.com",
		Name:           "Alice Gpay",
		StellarAddress: "GBZC6YRFWINCGYH6FFIK3VY4KF3WZJQR7CD3S5Y4GVNIKU5RM3JY7YEX",
		Role:           "user",
		IsActive:       true,
	}
	db.Create(&user)

	body, _ := json.Marshal(RequestBody{
		Query: "{ me { id email name stellarAddress } }",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/graphql", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "alice@example.com")
}

func TestGraphQL_ContactMutationAndQuery(t *testing.T) {
	_, router, _ := setupTestGraphQLServer()

	// 1. Create Contact Mutation
	body, _ := json.Marshal(RequestBody{
		Query: `mutation { createContact(input: { nickname: "Bob", stellarAddress: "GBZC6YRFWINCGYH6FFIK3VY4KF3WZJQR7CD3S5Y4GVNIKU5RM3JY7YEX" }) { id nickname stellarAddress } }`,
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/graphql", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Bob")

	// 2. Query Contacts
	body2, _ := json.Marshal(RequestBody{
		Query: "{ contacts { id nickname stellarAddress currency } }",
	})
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPost, "/graphql", bytes.NewBuffer(body2))
	req2.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Contains(t, w2.Body.String(), "Bob")
}

func TestGraphQL_ExchangeRateQuery(t *testing.T) {
	_, router, _ := setupTestGraphQLServer()

	body, _ := json.Marshal(RequestBody{
		Query: `{ exchangeRate(base: "USD", target: "EUR") { base target rate timestamp } }`,
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/graphql", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "USD")
	assert.Contains(t, w.Body.String(), "EUR")
}
