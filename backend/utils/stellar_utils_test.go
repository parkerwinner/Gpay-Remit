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
	
	// Use a definitely valid test address
	destination := "GC7S3S67JVRYCOY6Z7HJSJ6B676B6J6B6J6B6J6B6J6B6J6B6J6B6J6B"
	// Wait, let's just generate another random kp for the destination to be safe.
	destKP, _ := keypair.Random()
	destination = destKP.Address()

	tx, err := txnbuild.NewTransaction(
		txnbuild.TransactionParams{
			SourceAccount:        &sourceAccount,
			IncrementSequenceNum: true,
			BaseFee:              txnbuild.MinBaseFee,
			Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewInfiniteTimeout()},
			Operations: []txnbuild.Operation{
				&txnbuild.Payment{
					Destination: destination,
					Amount:      "10",
					Asset:       txnbuild.NativeAsset{},
				},
			},
		},
	)
	if !assert.NoError(t, err) || tx == nil {
		t.FailNow()
	}

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
	sourceKP, _ := keypair.Random()
	sourceAccount := &txnbuild.SimpleAccount{AccountID: sourceKP.Address(), Sequence: 1}


	destKP, _ := keypair.Random()
	destination := destKP.Address()
	issuerKP, _ := keypair.Random()
	issuer := issuerKP.Address()

	t.Run("Native payment", func(t *testing.T) {
		tx, err := client.BuildPaymentTx(sourceAccount, destination, "XLM", "", "100")
		assert.NoError(t, err)
		assert.NotNil(t, tx)
		assert.Len(t, tx.Operations(), 1)
		
		op := tx.Operations()[0].(*txnbuild.Payment)
		assert.Equal(t, "100", op.Amount)
		assert.IsType(t, txnbuild.NativeAsset{}, op.Asset)
	})

	t.Run("Credit asset payment", func(t *testing.T) {
		tx, err := client.BuildPaymentTx(sourceAccount, destination, "USDC", issuer, "50")
		assert.NoError(t, err)
		assert.NotNil(t, tx)
		
		op := tx.Operations()[0].(*txnbuild.Payment)
		asset := op.Asset.(txnbuild.CreditAsset)
		assert.Equal(t, "USDC", asset.Code)
		assert.Equal(t, issuer, asset.Issuer)
	})

}
