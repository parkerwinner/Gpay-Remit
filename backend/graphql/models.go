package graphql


type UserGQL struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	Name           string `json:"name"`
	StellarAddress string `json:"stellarAddress"`
	Role           string `json:"role"`
	Country        string `json:"country,omitempty"`
	IsActive       bool   `json:"isActive"`
}

type ContactGQL struct {
	ID                 string `json:"id"`
	UserID             string `json:"userId"`
	Nickname           string `json:"nickname"`
	StellarAddress     string `json:"stellarAddress"`
	Currency           string `json:"currency"`
	Email              string `json:"email,omitempty"`
	Notes              string `json:"notes,omitempty"`
	VerificationStatus string `json:"verificationStatus"`
	IsVerified         bool   `json:"isVerified"`
}

type PaymentGQL struct {
	ID               string  `json:"id"`
	SenderID         string  `json:"senderId"`
	SenderAccount    string  `json:"senderAccount"`
	RecipientID      string  `json:"recipientId"`
	RecipientAccount string  `json:"recipientAccount"`
	Amount           float64 `json:"amount"`
	Currency         string  `json:"currency"`
	Status           string  `json:"status"`
	TxHash           string  `json:"txHash,omitempty"`
	Fee              float64 `json:"fee,omitempty"`
	CreatedAt        string  `json:"createdAt"`
}

type ExchangeRateGQL struct {
	Base      string  `json:"base"`
	Target    string  `json:"target"`
	Rate      float64 `json:"rate"`
	Timestamp string  `json:"timestamp"`
}

type CreateContactInput struct {
	Nickname       string `json:"nickname"`
	StellarAddress string `json:"stellarAddress"`
	Currency       string `json:"currency,omitempty"`
	Email          string `json:"email,omitempty"`
	Notes          string `json:"notes,omitempty"`
}

type CreatePaymentInput struct {
	SenderAccount    string  `json:"senderAccount"`
	RecipientAccount string  `json:"recipientAccount"`
	Amount           float64 `json:"amount"`
	Currency         string  `json:"currency"`
}

type RequestBody struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName,omitempty"`
	Variables     map[string]interface{} `json:"variables,omitempty"`
}
