package workers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Gpay-Remit/gpay-remit/config"
	"github.com/Gpay-Remit/gpay-remit/models"
	"github.com/Gpay-Remit/gpay-remit/services"
	"github.com/Gpay-Remit/gpay-remit/utils"
	"github.com/stellar/go/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	workerSenderAddr    = "GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF"
	workerRecipientAddr = "GCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC"
)

type stubStellarClient struct {
	validateErr error
}

func (s *stubStellarClient) SubmitPayment(ctx context.Context, sourceSecret, destination, assetCode, issuer, amount string) (string, error) {
	return "", nil
}
func (s *stubStellarClient) ValidateAccount(ctx context.Context, accountID string) error {
	return s.validateErr
}
func (s *stubStellarClient) BuildEscrowTx(ctx context.Context, sender, recipient, assetCode, issuer, amount string) (string, error) {
	return "", nil
}
func (s *stubStellarClient) BuildPaymentTx(ctx context.Context, sourceAccount txnbuild.Account, destination, assetCode, issuer, amount string) (*txnbuild.Transaction, error) {
	return nil, nil
}
func (s *stubStellarClient) SignTx(ctx context.Context, envelopeXDR, secretKey string) (string, error) {
	return "", nil
}
func (s *stubStellarClient) GetAccountBalances(ctx context.Context, accountID string) ([]utils.AccountBalance, error) {
	return nil, nil
}

func setupWorkerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Payment{}, &models.RecurringSchedule{}, &models.RecurringScheduleExecution{}))
	return db
}

func TestComputeNextRunAtMonthEndAndLeapYear(t *testing.T) {
	// Jan 31 -> Feb 28 (non-leap year 2025)
	jan31 := time.Date(2025, 1, 31, 12, 0, 0, 0, time.UTC)
	nextNonLeap, err := models.ComputeNextRunAt(jan31, models.RecurringFreqMonthly)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2025, 2, 28, 12, 0, 0, 0, time.UTC), nextNonLeap)

	// Jan 31 -> Feb 29 (leap year 2024)
	jan31Leap := time.Date(2024, 1, 31, 12, 0, 0, 0, time.UTC)
	nextLeap, err := models.ComputeNextRunAt(jan31Leap, models.RecurringFreqMonthly)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC), nextLeap)

	// Daily & Weekly
	nextDay, err := models.ComputeNextRunAt(jan31, models.RecurringFreqDaily)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC), nextDay)

	nextWeek, err := models.ComputeNextRunAt(jan31, models.RecurringFreqWeekly)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2025, 2, 7, 12, 0, 0, 0, time.UTC), nextWeek)
}

func TestRecurringWorkerExecutionAndDeduplication(t *testing.T) {
	db := setupWorkerDB(t)
	user := models.User{
		ID:             1,
		Email:          "rec@example.com",
		Name:           "Rec User",
		StellarAddress: workerSenderAddr,
		PasswordHash:   "h",
		IsActive:       true,
	}
	require.NoError(t, db.Create(&user).Error)

	now := time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC)
	due := now.Add(-1 * time.Hour)

	activeSched := models.RecurringSchedule{
		UserID:           user.ID,
		SenderAccount:    workerSenderAddr,
		RecipientAccount: workerRecipientAddr,
		Amount:           10000000,
		Currency:         "USDC",
		Frequency:        models.RecurringFreqDaily,
		Status:           models.RecurringStatusActive,
		NextRunAt:        due,
	}
	pausedSched := models.RecurringSchedule{
		UserID:           user.ID,
		SenderAccount:    workerSenderAddr,
		RecipientAccount: workerRecipientAddr,
		Amount:           10000000,
		Currency:         "USDC",
		Frequency:        models.RecurringFreqDaily,
		Status:           models.RecurringStatusPaused,
		NextRunAt:        due,
	}
	cancelledSched := models.RecurringSchedule{
		UserID:           user.ID,
		SenderAccount:    workerSenderAddr,
		RecipientAccount: workerRecipientAddr,
		Amount:           10000000,
		Currency:         "USDC",
		Frequency:        models.RecurringFreqDaily,
		Status:           models.RecurringStatusCancelled,
		NextRunAt:        due,
	}
	require.NoError(t, db.Create(&activeSched).Error)
	require.NoError(t, db.Create(&pausedSched).Error)
	require.NoError(t, db.Create(&cancelledSched).Error)

	cfg := &config.Config{RecurringWorkerBatchSize: 10, RecurringMaxFailures: 2}
	fees := services.NewFeeService(cfg)

	// Run twice at the same timestamp; only 1 payment should be created.
	count1, err := ProcessDueRecurringSchedules(context.Background(), db, cfg, fees, &stubStellarClient{}, nil, now)
	require.NoError(t, err)
	assert.Equal(t, 1, count1)

	count2, err := ProcessDueRecurringSchedules(context.Background(), db, cfg, fees, &stubStellarClient{}, nil, now)
	require.NoError(t, err)
	assert.Equal(t, 0, count2)

	var payments []models.Payment
	require.NoError(t, db.Find(&payments).Error)
	assert.Len(t, payments, 1)

	// Unique constraint rejects duplicate execution for the same scheduled_for.
	dupErr := db.Create(&models.RecurringScheduleExecution{
		ScheduleID:   activeSched.ID,
		ScheduledFor: due,
		Status:       "processing",
	}).Error
	assert.Error(t, dupErr)
}

func TestRecurringWorkerAutoPauseOnFailureThreshold(t *testing.T) {
	db := setupWorkerDB(t)
	user := models.User{
		ID:             1,
		Email:          "fail@example.com",
		Name:           "Fail User",
		StellarAddress: workerSenderAddr,
		PasswordHash:   "h",
		IsActive:       true,
	}
	require.NoError(t, db.Create(&user).Error)

	t0 := time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC)
	sched := models.RecurringSchedule{
		UserID:           user.ID,
		SenderAccount:    workerSenderAddr,
		RecipientAccount: workerRecipientAddr,
		Amount:           10000000,
		Currency:         "USDC",
		Frequency:        models.RecurringFreqDaily,
		Status:           models.RecurringStatusActive,
		NextRunAt:        t0,
	}
	require.NoError(t, db.Create(&sched).Error)

	cfg := &config.Config{RecurringWorkerBatchSize: 10, RecurringMaxFailures: 2}
	fees := services.NewFeeService(cfg)
	failingStellar := &stubStellarClient{validateErr: errors.New("recipient invalid")}

	_, err := ProcessDueRecurringSchedules(context.Background(), db, cfg, fees, failingStellar, nil, t0)
	require.NoError(t, err)

	var reloaded models.RecurringSchedule
	require.NoError(t, db.First(&reloaded, sched.ID).Error)
	assert.Equal(t, 1, reloaded.ConsecutiveFailures)
	assert.Equal(t, models.RecurringStatusActive, reloaded.Status)

	// Second failure hits threshold = 2 and auto-pauses.
	t1 := reloaded.NextRunAt
	_, err = ProcessDueRecurringSchedules(context.Background(), db, cfg, fees, failingStellar, nil, t1)
	require.NoError(t, err)

	require.NoError(t, db.First(&reloaded, sched.ID).Error)
	assert.Equal(t, 2, reloaded.ConsecutiveFailures)
	assert.Equal(t, models.RecurringStatusPaused, reloaded.Status)
}
