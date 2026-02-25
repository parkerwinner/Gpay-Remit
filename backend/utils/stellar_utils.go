package utils

import (
	"fmt"
	"log"
	"strings"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/txnbuild"
)

// StellarClient wraps the Horizon client and network settings.
type StellarClient struct {
	client            *horizonclient.Client
	networkPassphrase string
}

// NewStellarClient initializes a new StellarClient.
func NewStellarClient(horizonURL, networkPassphrase string) *StellarClient {
	return &StellarClient{
		client:            &horizonclient.Client{HorizonURL: horizonURL},
		networkPassphrase: networkPassphrase,
	}
}

// SignTx signs a transaction envelope XDR with the provided secret key.
// It returns the signed XDR string. If signing fails, it returns the original XDR (as per requirements) and an error.
func SignTx(envelopeXDR string, secretKey string, networkPassphrase string) (string, error) {
	// Mask secret key in logs
	maskedKey := "REDACTED"
	if len(secretKey) > 8 {
		maskedKey = secretKey[:4] + "..." + secretKey[len(secretKey)-4:]
	}
	log.Printf("Signing transaction with key: %s on network: %s", maskedKey, networkPassphrase)

	genericTx, err := txnbuild.TransactionFromXDR(envelopeXDR)
	if err != nil {
		return envelopeXDR, fmt.Errorf("failed to parse envelope XDR: %w", err)
	}

	tx, ok := genericTx.Transaction()
	if !ok {
		return envelopeXDR, fmt.Errorf("XDR is not a transaction envelope")
	}

	kp, err := keypair.ParseFull(secretKey)
	if err != nil {
		return envelopeXDR, fmt.Errorf("invalid secret key: %w", err)
	}

	signedTx, err := tx.Sign(networkPassphrase, kp)
	if err != nil {
		return envelopeXDR, fmt.Errorf("failed to sign transaction: %w", err)
	}

	signedXDR, err := signedTx.Base64()
	if err != nil {
		return envelopeXDR, fmt.Errorf("failed to encode signed transaction: %w", err)
	}

	return signedXDR, nil
}

// SignTx is a wrapper that uses the client's network passphrase.
func (s *StellarClient) SignTx(envelopeXDR string, secretKey string) (string, error) {
	return SignTx(envelopeXDR, secretKey, s.networkPassphrase)
}

// BuildPaymentTx creates an unsigned payment transaction.
func (s *StellarClient) BuildPaymentTx(sourceAccount txnbuild.Account, destination, assetCode, issuer, amount string) (*txnbuild.Transaction, error) {
	var asset txnbuild.Asset
	if strings.ToUpper(assetCode) == "XLM" || assetCode == "" {
		asset = txnbuild.NativeAsset{}
	} else {
		asset = txnbuild.CreditAsset{Code: assetCode, Issuer: issuer}
	}

	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        sourceAccount,
			IncrementSequenceNum: true,
			BaseFee:              txnbuild.MinBaseFee,
			Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewInfiniteTimeout()},
			Operations: []txnbuild.Operation{
				&txnbuild.Payment{
					Destination: destination,
					Amount:      amount,
					Asset:       asset,
				},
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build payment transaction: %w", err)
	}

	return tx, nil
}

// SubmitPayment builds, signs, and submits a payment transaction in one go.
func (s *StellarClient) SubmitPayment(sourceSecret, destination, assetCode, issuer string, amount string) (string, error) {
	sourceKP, err := keypair.ParseFull(sourceSecret)
	if err != nil {
		return "", fmt.Errorf("invalid source secret: %w", err)
	}

	sourceAccount, err := s.client.AccountDetail(horizonclient.AccountRequest{
		AccountID: sourceKP.Address(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to load source account: %w", err)
	}

	tx, err := s.BuildPaymentTx(&sourceAccount, destination, assetCode, issuer, amount)
	if err != nil {
		return "", err
	}

	signedXDR, err := s.SignTx(tx.Base64(), sourceSecret)
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Re-parse signed XDR to submit
	genericTx, err := txnbuild.TransactionFromXDR(signedXDR)
	if err != nil {
		return "", fmt.Errorf("failed to parse signed XDR: %w", err)
	}
	signedTx, _ := genericTx.Transaction()

	txResp, err := s.client.SubmitTransaction(signedTx)
	if err != nil {
		return "", fmt.Errorf("failed to submit transaction: %w", err)
	}

	return txResp.Hash, nil
}
