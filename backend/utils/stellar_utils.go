package utils

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/txnbuild"
)

type ctxKey string

const (
	ctxRequestIDKey ctxKey = "requestID"
	ctxUserIDKey    ctxKey = "userID"
)

type StellarClientInterface interface {
	SubmitPayment(ctx context.Context, sourceSecret string, destination string, assetCode string, issuer string, amount string) (string, error)
	ValidateAccount(ctx context.Context, accountID string) error
	BuildEscrowTx(ctx context.Context, sender string, recipient string, assetCode string, issuer string, amount string) (string, error)
	BuildPaymentTx(ctx context.Context, sourceAccount txnbuild.Account, destination string, assetCode string, issuer string, amount string) (*txnbuild.Transaction, error)
	SignTx(ctx context.Context, envelopeXDR string, secretKey string) (string, error)
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

func WithRequestContext(ctx context.Context, requestID string, userID interface{}) context.Context {
	ctx = context.WithValue(ctx, ctxRequestIDKey, requestID)
	if userID != nil {
		ctx = context.WithValue(ctx, ctxUserIDKey, userID)
	}
	return ctx
}

func requestContextFields(ctx context.Context) logrus.Fields {
	fields := logrus.Fields{}
	if requestID, ok := ctx.Value(ctxRequestIDKey).(string); ok && requestID != "" {
		fields["request_id"] = requestID
	}
	if userID := ctx.Value(ctxUserIDKey); userID != nil {
		fields["user_id"] = userID
	}
	return fields
}

func logWithContext(ctx context.Context, operation string) *logrus.Entry {
	return logrus.WithFields(requestContextFields(ctx)).WithField("stellar_operation", operation)
}

// SignTx signs a transaction envelope XDR with the provided secret key.
// It returns the signed XDR string. If signing fails, it returns the original XDR and an error.
func SignTx(ctx context.Context, envelopeXDR string, secretKey string, networkPassphrase string) (string, error) {
	fields := requestContextFields(ctx)
	maskedKey := "REDACTED"
	if len(secretKey) > 8 {
		maskedKey = secretKey[:4] + "..." + secretKey[len(secretKey)-4:]
	}
	fields["network_passphrase"] = networkPassphrase
	fields["secret_key_masked"] = maskedKey
	logrus.WithFields(fields).Info("Signing Stellar transaction")

	genericTx, err := txnbuild.TransactionFromXDR(envelopeXDR)
	if err != nil {
		logrus.WithFields(fields).WithError(err).Error("Failed to parse envelope XDR")
		return envelopeXDR, fmt.Errorf("failed to parse envelope XDR: %w", err)
	}

	tx, ok := genericTx.Transaction()
	if !ok {
		logrus.WithFields(fields).Error("XDR is not a transaction envelope")
		return envelopeXDR, fmt.Errorf("XDR is not a transaction envelope")
	}

	kp, err := keypair.ParseFull(secretKey)
	if err != nil {
		logrus.WithFields(fields).WithError(err).Error("Invalid secret key")
		return envelopeXDR, fmt.Errorf("invalid secret key: %w", err)
	}

	signedTx, err := tx.Sign(networkPassphrase, kp)
	if err != nil {
		logrus.WithFields(fields).WithError(err).Error("Failed to sign transaction")
		return envelopeXDR, fmt.Errorf("failed to sign transaction: %w", err)
	}

	signedXDR, err := signedTx.Base64()
	if err != nil {
		logrus.WithFields(fields).WithError(err).Error("Failed to encode signed transaction")
		return envelopeXDR, fmt.Errorf("failed to encode signed transaction: %w", err)
	}

	logrus.WithFields(fields).Info("Transaction signed successfully")
	return signedXDR, nil
}

// SignTx is a wrapper that uses the client's network passphrase.
func (s *StellarClient) SignTx(ctx context.Context, envelopeXDR string, secretKey string) (string, error) {
	return SignTx(ctx, envelopeXDR, secretKey, s.networkPassphrase)
}

// BuildPaymentTx creates an unsigned payment transaction.
func (s *StellarClient) BuildPaymentTx(ctx context.Context, sourceAccount txnbuild.Account, destination string, assetCode string, issuer string, amount string) (*txnbuild.Transaction, error) {
	logWithContext(ctx, "build_payment_tx").Info("Building payment transaction")

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
		logWithContext(ctx, "build_payment_tx").WithError(err).Error("Failed to build payment transaction")
		return nil, fmt.Errorf("failed to build payment transaction: %w", err)
	}
	return tx, nil
}


// SubmitPayment builds, signs, and submits a payment transaction in one go.
func (s *StellarClient) SubmitPayment(ctx context.Context, sourceSecret string, destination string, assetCode string, issuer string, amount string) (string, error) {
	logWithContext(ctx, "submit_payment").Info("Starting submit payment flow")

	sourceKP, err := keypair.ParseFull(sourceSecret)
	if err != nil {
		logWithContext(ctx, "submit_payment").WithError(err).Error("Invalid source secret")
		return "", fmt.Errorf("invalid source secret: %w", err)
	}

	logWithContext(ctx, "submit_payment").WithField("source_account", sourceKP.Address()).Info("Loading source account")
	sourceAccount, err := s.client.AccountDetail(horizonclient.AccountRequest{
		AccountID: sourceKP.Address(),
	})
	if err != nil {
		logWithContext(ctx, "submit_payment").WithError(err).Error("Failed to load source account")
		return "", fmt.Errorf("failed to load source account: %w", err)
	}

	logWithContext(ctx, "submit_payment").Info("Building payment transaction")
	tx, err := s.BuildPaymentTx(ctx, &sourceAccount, destination, assetCode, issuer, amount)
	if err != nil {
		return "", err
	}

	xdr, err := tx.Base64()
	if err != nil {
		logWithContext(ctx, "submit_payment").WithError(err).Error("Failed to encode transaction")
		return "", fmt.Errorf("failed to encode transaction: %w", err)
	}

	logWithContext(ctx, "submit_payment").Info("Signing transaction")
	signedXDR, err := s.SignTx(ctx, xdr, sourceSecret)
	if err != nil {
		return "", err
	}

	// Re-parse signed XDR to submit
	genericTx, err := txnbuild.TransactionFromXDR(signedXDR)
	if err != nil {
		logWithContext(ctx, "submit_payment").WithError(err).Error("Failed to parse signed XDR")
		return "", fmt.Errorf("failed to parse signed XDR: %w", err)
	}
	signedTx, _ := genericTx.Transaction()

	logWithContext(ctx, "submit_payment").Info("Submitting transaction to Horizon")
	txResp, err := s.client.SubmitTransaction(signedTx)
	if err != nil {
		logWithContext(ctx, "submit_payment").WithError(err).Error("Failed to submit transaction")
		return "", fmt.Errorf("failed to submit transaction: %w", err)
	}

	logWithContext(ctx, "submit_payment").WithField("tx_hash", txResp.Hash).Info("Transaction submitted successfully")
	return txResp.Hash, nil
}

func (s *StellarClient) ValidateAccount(ctx context.Context, accountID string) error {
	logWithContext(ctx, "validate_account").WithField("account_id", accountID).Info("Validating Stellar account")
	_, err := s.client.AccountDetail(horizonclient.AccountRequest{AccountID: accountID})
	if err != nil {
		logWithContext(ctx, "validate_account").WithError(err).Error("Invalid or non-existent account")
		return fmt.Errorf("invalid or non-existent account: %w", err)
	}
	return nil
}

func (s *StellarClient) BuildEscrowTx(ctx context.Context, sender string, recipient string, assetCode string, issuer string, amount string) (string, error) {
	logWithContext(ctx, "build_escrow_tx").WithFields(logrus.Fields{
		"sender":     sender,
		"recipient":  recipient,
		"asset_code": assetCode,
	}).Info("Building escrow transaction envelope")

	sourceAccount, err := s.client.AccountDetail(horizonclient.AccountRequest{AccountID: sender})
	if err != nil {
		logWithContext(ctx, "build_escrow_tx").WithError(err).Error("Failed to load source account")
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
		logWithContext(ctx, "build_escrow_tx").WithError(err).Error("Failed to build escrow transaction")
		return "", fmt.Errorf("failed to build escrow transaction: %w", err)
	}

	xdr, err := tx.Base64()
	if err != nil {
		logWithContext(ctx, "build_escrow_tx").WithError(err).Error("Failed to encode transaction to XDR")
		return "", fmt.Errorf("failed to encode transaction to XDR: %w", err)
	}

	logWithContext(ctx, "build_escrow_tx").Info("Escrow transaction envelope built successfully")
	return xdr, nil
}
