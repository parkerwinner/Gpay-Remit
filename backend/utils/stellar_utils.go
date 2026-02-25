package utils

import (
	"fmt"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/txnbuild"
)

type StellarClientInterface interface {
	SubmitPayment(sourceSecret, destination, assetCode, issuer, amount string) (string, error)
	ValidateAccount(accountID string) error
	BuildEscrowTx(sender, recipient, assetCode, issuer, amount string) (string, error)
}

type StellarClient struct {
	client            *horizonclient.Client
	networkPassphrase string
}

func NewStellarClient(horizonURL, networkPassphrase string) StellarClientInterface {
	return &StellarClient{
		client:            &horizonclient.Client{HorizonURL: horizonURL},
		networkPassphrase: networkPassphrase,
	}
}

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

	var asset txnbuild.Asset
	if assetCode == "XLM" {
		asset = txnbuild.NativeAsset{}
	} else {
		asset = txnbuild.CreditAsset{Code: assetCode, Issuer: issuer}
	}

	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &sourceAccount,
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
		return "", fmt.Errorf("failed to build transaction: %w", err)
	}

	tx, err = tx.Sign(s.networkPassphrase, sourceKP)
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	txResp, err := s.client.SubmitTransaction(tx)
	if err != nil {
		return "", fmt.Errorf("failed to submit transaction: %w", err)
	}

	return txResp.Hash, nil
}

func (s *StellarClient) ValidateAccount(accountID string) error {
	_, err := s.client.AccountDetail(horizonclient.AccountRequest{AccountID: accountID})
	if err != nil {
		return fmt.Errorf("invalid or non-existent account: %w", err)
	}
	return nil
}

func (s *StellarClient) BuildEscrowTx(sender, recipient, assetCode, issuer, amount string) (string, error) {
	sourceAccount, err := s.client.AccountDetail(horizonclient.AccountRequest{AccountID: sender})
	if err != nil {
		return "", fmt.Errorf("failed to load source account: %w", err)
	}

	var asset txnbuild.Asset
	if assetCode == "XLM" {
		asset = txnbuild.NativeAsset{}
	} else {
		asset = txnbuild.CreditAsset{Code: assetCode, Issuer: issuer}
	}

	// This is a simplified version of escrow creation.
	// In a real scenario, this would likely involve a Soroban contract call
	// or a multi-sig escrow account setup.
	// For this task, we'll build a simple payment transaction that can be used
	// as a placeholder for the escrow transaction envelope.
	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &sourceAccount,
			IncrementSequenceNum: true,
			BaseFee:              txnbuild.MinBaseFee,
			Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewInfiniteTimeout()},
			Operations: []txnbuild.Operation{
				&txnbuild.Payment{
					Destination: recipient,
					Amount:      amount,
					Asset:       asset,
				},
			},
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to build escrow transaction: %w", err)
	}

	xdr, err := tx.Base64()
	if err != nil {
		return "", fmt.Errorf("failed to encode transaction to XDR: %w", err)
	}

	return xdr, nil
}
