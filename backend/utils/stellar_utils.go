package utils

import (
	"fmt"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/txnbuild"
)

type StellarClient struct {
	client            *horizonclient.Client
	networkPassphrase string
}

func NewStellarClient(horizonURL, networkPassphrase string) *StellarClient {
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

	tx, err = tx.Sign(network.TestNetworkPassphrase, sourceKP)
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	txResp, err := s.client.SubmitTransaction(tx)
	if err != nil {
		return "", fmt.Errorf("failed to submit transaction: %w", err)
	}

	return txResp.Hash, nil
}
