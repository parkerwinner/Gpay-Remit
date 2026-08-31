package testfixtures

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/gpay-remit/models"
)

// The factories exist so tests stop depending on the zero value of fields they
// never set. These cases pin the two properties that makes them worth using:
// generated values satisfy the model's own column constraints, and options
// leave dependent fields consistent rather than contradictory.

func TestStellarAddressMatchesColumnConstraint(t *testing.T) {
	for i := 0; i < 50; i++ {
		addr := StellarAddress()

		// size:56 with a G prefix — an address that fails this would be
		// rejected by the application, making any test using it meaningless.
		require.Len(t, addr, 56)
		require.True(t, strings.HasPrefix(addr, "G"))

		// RFC 4648 base32 is A-Z plus 2-7. Emitting 0, 1, 8 or 9 would produce
		// addresses no real decoder accepts.
		for _, c := range addr[1:] {
			require.Contains(t, "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567", string(c),
				"address contains a non-base32 character")
		}
	}
}

func TestStellarAddressesAreDistinct(t *testing.T) {
	// StellarAddress backs uniqueIndex columns; collisions would surface as
	// confusing constraint violations rather than as a factory problem.
	seen := make(map[string]bool, 200)
	for i := 0; i < 200; i++ {
		addr := StellarAddress()
		require.False(t, seen[addr], "duplicate address generated")
		seen[addr] = true
	}
}

func TestSeedMakesGenerationReproducible(t *testing.T) {
	Seed(42)
	first := []string{StellarAddress(), Currency(), TxHash()}

	Seed(42)
	second := []string{StellarAddress(), Currency(), TxHash()}

	// Without this a failing randomised test cannot be replayed.
	assert.Equal(t, first, second)
}

func TestAmountIsPositiveAndRounded(t *testing.T) {
	for i := 0; i < 100; i++ {
		amount := Amount()
		require.Greater(t, amount, 0.0)
		// Two decimal places, so assertions comparing money do not fail on a
		// trailing float artefact. Checked as "cents is a whole number" rather
		// than by truncating, which would itself trip on float representation.
		cents := amount * 100
		assert.InDelta(t, math.Round(cents), cents, 1e-6)
	}
}

func TestNewUserProducesAValidActiveUser(t *testing.T) {
	u := NewUser()

	assert.NotEmpty(t, u.Email)
	assert.NotEmpty(t, u.Name)
	assert.Len(t, u.StellarAddress, 56)
	assert.Equal(t, "user", u.Role)
	assert.Equal(t, "pending", u.KYCStatus)
	assert.True(t, u.IsActive)
	assert.Len(t, u.Country, 2)
	// Non-null jsonb column — an empty string would fail the insert.
	assert.Equal(t, "{}", u.Preferences)
}

func TestUserOptionsOverrideOnlyWhatTheyName(t *testing.T) {
	u := NewUser(WithEmail("known@example.com"), WithRole("admin"))

	assert.Equal(t, "known@example.com", u.Email)
	assert.Equal(t, "admin", u.Role)
	// Everything else keeps a valid generated value.
	assert.NotEmpty(t, u.StellarAddress)
	assert.True(t, u.IsActive)
}

func TestWithKYCStatusKeepsTheVerifiedTimestampConsistent(t *testing.T) {
	verified := NewUser(WithKYCStatus("verified"))
	require.Equal(t, "verified", verified.KYCStatus)
	// A verified user without a timestamp is a state the application never
	// produces, so the factory must not either.
	require.NotNil(t, verified.KYCVerifiedAt)

	pending := NewUser(WithKYCStatus("pending"))
	assert.Nil(t, pending.KYCVerifiedAt)
}

func TestWithLockedUntilAlsoSetsTheFailureCount(t *testing.T) {
	u := NewUser(WithLockedUntil(nowPlus(1)))

	require.NotNil(t, u.LockedUntil)
	// A lock with zero failed attempts could not have happened.
	assert.Greater(t, u.FailedLoginAttempts, 0)
}

func TestWithMFAEnabledSetsCompletionTimestamp(t *testing.T) {
	u := NewUser(WithMFAEnabled())

	assert.True(t, u.MFAEnabled)
	assert.NotNil(t, u.MFASetupCompletedAt)
}

func TestNewUsersGeneratesDistinctEmails(t *testing.T) {
	users := NewUsers(25)
	require.Len(t, users, 25)

	// Email carries a uniqueIndex.
	seen := make(map[string]bool, 25)
	for _, u := range users {
		require.False(t, seen[u.Email], "duplicate email %s", u.Email)
		seen[u.Email] = true
	}
}

func TestNewPaymentKeepsFeeEqualToItsComponents(t *testing.T) {
	p := NewPayment()

	// The model documents Fee as the total of its components; a factory that
	// set them independently would emit records the application says cannot
	// exist.
	sum := p.PlatformFee + p.NetworkFee + p.ComplianceFee + p.ForexFee
	assert.InDelta(t, p.Fee, sum, 0.01)
}

func TestSameCurrencyPaymentChargesNoForexFee(t *testing.T) {
	p := NewPayment(WithPaymentAmount(100, "USD"))

	assert.Zero(t, p.ForexFee)
}

func TestCrossCurrencyPaymentChargesForexFee(t *testing.T) {
	p := NewPayment(WithPaymentAmount(100, "USD"), WithCrossCurrency("EUR", 0.92))

	assert.Greater(t, p.ForexFee, 0.0)
	assert.Equal(t, "EUR", p.TargetCurrency)
	assert.InDelta(t, 92.0, p.ConvertedAmount, 0.01)
	// The total must still reconcile once the forex component appears.
	assert.InDelta(t, p.Fee, p.PlatformFee+p.NetworkFee+p.ComplianceFee+p.ForexFee, 0.01)
}

func TestCompletedPaymentAlwaysCarriesATransactionHash(t *testing.T) {
	p := NewPayment(WithPaymentStatus("completed"))

	assert.Equal(t, "completed", p.Status)
	// A completed on-chain payment with no hash is not a state the system
	// reaches, and a test relying on one would prove nothing.
	require.Len(t, p.TxHash, 64)
}

func TestWithPartiesWiresBothSides(t *testing.T) {
	sender := NewUser()
	sender.ID = 7
	recipient := NewUser()
	recipient.ID = 9

	p := NewPayment(WithParties(sender, recipient))

	assert.Equal(t, uint(7), p.SenderID)
	assert.Equal(t, sender.StellarAddress, p.SenderAccount)
	assert.Equal(t, uint(9), p.RecipientID)
	assert.Equal(t, recipient.StellarAddress, p.RecipientAccount)
}

func TestPaymentSearchableTextIsUsable(t *testing.T) {
	// Guards against a factory emitting values the model's own helper chokes on.
	p := NewPayment()
	assert.NotEmpty(t, p.SearchableText())
}

func TestNewContactStartsUnverified(t *testing.T) {
	c := NewContact()

	assert.Equal(t, models.ContactVerificationPending, c.VerificationStatus)
	assert.False(t, c.IsVerified)
	assert.Len(t, c.StellarAddress, 56)
}

func TestWithContactVerifiedKeepsBothRepresentationsInStep(t *testing.T) {
	c := NewContact(WithContactVerified())

	// IsVerified and VerificationStatus encode the same fact; letting them
	// disagree is exactly the drift the factory exists to prevent.
	assert.True(t, c.IsVerified)
	assert.Equal(t, models.ContactVerificationVerified, c.VerificationStatus)
}

func TestNewInvoiceGeneratesUniqueInvoiceNumbers(t *testing.T) {
	// InvoiceNo carries a uniqueIndex.
	seen := make(map[string]bool, 50)
	for i := 0; i < 50; i++ {
		no := NewInvoice().InvoiceNo
		require.False(t, seen[no], "duplicate invoice number %s", no)
		seen[no] = true
	}
}

func TestWithInvoiceOverdueBackdatesTheDueDate(t *testing.T) {
	i := NewInvoice(WithInvoiceOverdue())

	assert.Equal(t, "overdue", i.Status)
	require.NotNil(t, i.DueDate)
	// Labelled overdue but due next month would not exercise any real overdue
	// logic.
	assert.True(t, i.DueDate.Before(nowPlus(0)))
}

func TestNewPaymentRequestDefaultsToPendingAndUnexpired(t *testing.T) {
	r := NewPaymentRequest()

	assert.Equal(t, "pending", r.Status)
	require.NotNil(t, r.ExpiresAt)
	assert.True(t, r.ExpiresAt.After(nowPlus(0)))
	assert.Nil(t, r.AcceptedAt)
	assert.Nil(t, r.PaidAt)
}

func TestRequestStatusSetsTheMatchingTimestamp(t *testing.T) {
	accepted := NewPaymentRequest(WithRequestStatus("accepted"))
	require.NotNil(t, accepted.AcceptedAt)

	rejected := NewPaymentRequest(WithRequestStatus("rejected"))
	require.NotNil(t, rejected.RejectedAt)
	// A rejection with no reason is not something the API allows.
	assert.NotEmpty(t, rejected.RejectionReason)

	paid := NewPaymentRequest(WithRequestStatus("paid"))
	require.NotNil(t, paid.PaidAt)
	// Paid implies it was accepted first.
	assert.NotNil(t, paid.AcceptedAt)
}

func TestWithRequestExpiredBackdatesExpiry(t *testing.T) {
	r := NewPaymentRequest(WithRequestExpired())

	assert.Equal(t, "expired", r.Status)
	require.NotNil(t, r.ExpiresAt)
	assert.True(t, r.ExpiresAt.Before(nowPlus(0)))
}

func TestNewWebhookIsActiveWithAPlausibleSecret(t *testing.T) {
	w := NewWebhook()

	assert.True(t, w.IsActive)
	assert.NotEmpty(t, w.URL)
	// Long enough to stand in for a real HMAC key.
	assert.GreaterOrEqual(t, len(w.Secret), 32)
	assert.NotEmpty(t, w.Events)
}

func TestWithWebhookEventsJoinsTheList(t *testing.T) {
	w := NewWebhook(WithWebhookEvents("invoice.paid", "payment.failed"))

	assert.Equal(t, "invoice.paid,payment.failed", w.Events)
}

func TestFailedDeliveryPopulatesTheRetryFields(t *testing.T) {
	d := NewWebhookDelivery(WithDeliveryFailed(3))

	assert.Equal(t, "failed", d.Status)
	assert.Equal(t, 3, d.AttemptCount)
	assert.GreaterOrEqual(t, d.ResponseCode, 500)
	assert.NotEmpty(t, d.ErrorMessage)
	// The retry worker reads NextRetryAt; a failed delivery without one would
	// never be picked up again.
	require.NotNil(t, d.NextRetryAt)
}

func TestSucceededDeliveryClearsTheRetryFields(t *testing.T) {
	d := NewWebhookDelivery(WithDeliverySucceeded())

	assert.Equal(t, "success", d.Status)
	assert.Equal(t, 200, d.ResponseCode)
	assert.Empty(t, d.ErrorMessage)
	// A succeeded delivery still scheduled for retry would be redelivered.
	assert.Nil(t, d.NextRetryAt)
	assert.NotNil(t, d.CompletedAt)
}

func TestNewAuditLogHasTheNotNullColumns(t *testing.T) {
	a := NewAuditLog()

	// Action, Resource and IPAddress are all NOT NULL.
	assert.NotEmpty(t, a.Action)
	assert.NotEmpty(t, a.Resource)
	assert.NotEmpty(t, a.IPAddress)
}

func TestWithAuditActorAttachesTheUser(t *testing.T) {
	a := NewAuditLog(WithAuditActor(42))

	require.NotNil(t, a.UserID)
	assert.Equal(t, uint(42), *a.UserID)
}
