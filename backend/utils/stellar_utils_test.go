package utils

import (
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
)

func TestSignTx(t *testing.T) {
	// Generate a random keypair for testing
	kp, _ := keypair.Random()
	secret := kp.Seed()
	address := kp.Address()

	// Create a dummy transaction
	sourceAccount := txnbuild.SimpleAccount{AccountID: address, Sequence: 1}
	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &sourceAccount,
			IncrementSequenceNum: true,
			BaseFee:              txnbuild.MinBaseFee,
			Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewInfiniteTimeout()},
			Operations: []txnbuild.Operation{
				&txnbuild.Payment{
					Destination: "GDQNY3Y7PNO5UAB6STH6YTP6S44R3S6SPJ7YNCK37N7I6U6YVCOV56V2",
					Amount:      "10",
					Asset:       txnbuild.NativeAsset{},
				},
			},
		},
	)
	assert.NoError(t, err)

	envelopeXDR, err := tx.Base64()
	assert.NoError(t, err)

	t.Run("Valid signature", func(t *testing.T) {
		signedXDR, err := SignTx(envelopeXDR, secret, network.TestNetworkPassphrase)
		assert.NoError(t, err)
		assert.NotEmpty(t, signedXDR)
		assert.NotEqual(t, envelopeXDR, signedXDR)

		// Verify signature
		genericTx, err := txnbuild.TransactionFromXDR(signedXDR)
		assert.NoError(t, err)
		stx, ok := genericTx.Transaction()
		assert.True(t, ok)
		assert.Len(t, stx.Signatures(), 1)
	})

	t.Run("Invalid secret key", func(t *testing.T) {
		signedXDR, err := SignTx(envelopeXDR, "invalid_key", network.TestNetworkPassphrase)
		assert.Error(t, err)
		assert.Equal(t, envelopeXDR, signedXDR) // Should return original XDR on error
	})

	t.Run("Invalid XDR", func(t *testing.T) {
		signedXDR, err := SignTx("invalid_xdr", secret, network.TestNetworkPassphrase)
		assert.Error(t, err)
		assert.Equal(t, "invalid_xdr", signedXDR)
	})
}

func TestBuildPaymentTx(t *testing.T) {
	client := NewStellarClient("https://horizon-testnet.stellar.org", network.TestNetworkPassphrase)
	sourceAccount := &txnbuild.SimpleAccount{AccountID: "GDQNY3Y7PNO5UAB6STH6YTP6S44R3S6SPJ7YNCK37N7I6U6YVCOV56V2", Sequence: 1}

	t.Run("Native payment", func(t *testing.T) {
		tx, err := client.BuildPaymentTx(sourceAccount, "GABC...", "XLM", "", "100")
		assert.NoError(t, err)
		assert.NotNil(t, tx)
		assert.Len(t, tx.Operations(), 1)
		
		op := tx.Operations()[0].(*txnbuild.Payment)
		assert.Equal(t, "100", op.Amount)
		assert.IsType(t, txnbuild.NativeAsset{}, op.Asset)
	})

	t.Run("Credit asset payment", func(t *testing.T) {
		tx, err := client.BuildPaymentTx(sourceAccount, "GABC...", "USDC", "GISS...", "50")
		assert.NoError(t, err)
		assert.NotNil(t, tx)
		
		op := tx.Operations()[0].(*txnbuild.Payment)
		asset := op.Asset.(txnbuild.CreditAsset)
		assert.Equal(t, "USDC", asset.Code)
		assert.Equal(t, "GISS...", asset.Issuer)
	})
}
