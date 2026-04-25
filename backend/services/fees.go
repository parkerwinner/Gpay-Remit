package services

import (
	"math"

	"github.com/yourusername/gpay-remit/config"
)

type FeeBreakdown struct {
	PlatformFee   float64 `json:"platform_fee"`
	ForexFee      float64 `json:"forex_fee"`
	ComplianceFee float64 `json:"compliance_fee"`
	NetworkFee    float64 `json:"network_fee"`
	TotalFee      float64 `json:"total_fee"`
}

type FeeService struct {
	cfg *config.Config
}

func NewFeeService(cfg *config.Config) *FeeService {
	return &FeeService{cfg: cfg}
}

func bps(amount float64, bps int) float64 {
	return amount * float64(bps) / 10000.0
}

func roundMoney(v float64) float64 {
	// Keep responses stable and DB-friendly for typical fiat-style amounts.
	return math.Round(v*100) / 100
}

// Calculate returns a fee breakdown. Fee config is intended to mirror the on-chain
// escrow contract fee structure (PaymentEscrow).
func (s *FeeService) Calculate(amount float64) FeeBreakdown {
	platform := bps(amount, s.cfg.PlatformFeeBps)
	forex := bps(amount, s.cfg.ForexFeeBps)
	compliance := bps(amount, s.cfg.ComplianceFeeBps)
	network := bps(amount, s.cfg.NetworkFeeBps)

	total := platform + forex + compliance + network

	if s.cfg.MinFee > 0 && total < s.cfg.MinFee {
		total = s.cfg.MinFee
	}
	if s.cfg.MaxFee > 0 && total > s.cfg.MaxFee {
		total = s.cfg.MaxFee
	}

	// When min/max clamps apply, preserve relative components but keep total stable.
	// If no clamp applied, the sum equals total already.
	sum := platform + forex + compliance + network
	if sum > 0 && total != sum {
		ratio := total / sum
		platform *= ratio
		forex *= ratio
		compliance *= ratio
		network *= ratio
	}

	return FeeBreakdown{
		PlatformFee:   roundMoney(platform),
		ForexFee:      roundMoney(forex),
		ComplianceFee: roundMoney(compliance),
		NetworkFee:    roundMoney(network),
		TotalFee:      roundMoney(total),
	}
}
