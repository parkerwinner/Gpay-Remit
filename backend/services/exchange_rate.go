package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/yourusername/gpay-remit/config"
)

// ExchangeRate is the normalized response returned to API consumers.
type ExchangeRate struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	Rate      float64   `json:"rate"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`
}

type oracleResponse struct {
	Result string             `json:"result"`
	Rates  map[string]float64 `json:"rates"`
}

// ExchangeRateService fetches currency conversion rates from the configured
// oracle/pricing provider.
type ExchangeRateService struct {
	baseURL    string
	httpClient *http.Client
}

func NewExchangeRateService(cfg *config.Config) *ExchangeRateService {
	return &ExchangeRateService{
		baseURL: cfg.ExchangeRateAPIURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetRate fetches the conversion rate from one currency to another via the
// oracle/pricing service.
func (s *ExchangeRateService) GetRate(from, to string) (*ExchangeRate, error) {
	from = strings.ToUpper(from)
	to = strings.ToUpper(to)

	url := fmt.Sprintf("%s/%s", strings.TrimRight(s.baseURL, "/"), from)
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to reach exchange rate provider: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exchange rate provider returned status %d", resp.StatusCode)
	}

	var parsed oracleResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode exchange rate response: %w", err)
	}

	rate, ok := parsed.Rates[to]
	if !ok {
		return nil, fmt.Errorf("no rate available for %s -> %s", from, to)
	}

	return &ExchangeRate{
		From:      from,
		To:        to,
		Rate:      rate,
		Timestamp: time.Now(),
		Source:    s.baseURL,
	}, nil
}
