// Package testdata provides factories for building realistic model instances
// in tests.
//
// The problem these solve is not typing effort — it is that hand-built structs
// drift. A test that sets only the three fields it cares about silently depends
// on the zero value of everything else, so adding a required column later
// breaks tests that have nothing to do with it. Factories give every model one
// place to define what a valid instance looks like.
//
// Two conventions run through the package:
//
//   - **Every factory takes functional options.** A test overrides exactly the
//     fields it is asserting on and inherits a valid value for the rest, so the
//     intent of the test is visible in the two lines that differ.
//   - **Values satisfy the model's own constraints.** Stellar addresses are 56
//     characters starting with G, currencies are real ISO codes, statuses come
//     from the set the column actually accepts. A factory that emits data the
//     application would reject makes tests pass that should not.
//
// Determinism is opt-in via Seed. Random data finds edge cases; a seed makes a
// failure reproducible once it does.
package testfixtures

import (
	"fmt"
	"strings"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/yourusername/gpay-remit/models"
)

// Seed fixes the generator so a failing test can be replayed exactly.
//
// Call it from TestMain when a suite needs reproducibility. Without it every
// run draws fresh values, which is what makes factories useful for finding
// assumptions the code did not know it had.
func Seed(seed uint64) {
	gofakeit.Seed(seed)
}

// ── Domain-accurate primitives ──────────────────────────────────────────────

// stellarAddressAlphabet is the RFC 4648 base32 alphabet Stellar keys are
// encoded with: A-Z plus the digits 2-7. It excludes 0, 1, 8 and 9, so an
// address containing those would be rejected by any real decoder.
const stellarAddressAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

// StellarAddress returns a syntactically valid public key: 'G' followed by 55
// base32 characters, matching the size:56 column constraint.
//
// Not a real keypair — nothing here signs anything — but it survives length and
// prefix validation, which a plain gofakeit.UUID() would not.
func StellarAddress() string {
	var b strings.Builder
	b.WriteByte('G')
	for i := 0; i < 55; i++ {
		b.WriteByte(stellarAddressAlphabet[gofakeit.IntRange(0, len(stellarAddressAlphabet)-1)])
	}
	return b.String()
}

// Currency returns an ISO 4217 code the platform actually supports.
func Currency() string {
	return gofakeit.RandomString([]string{"USD", "EUR", "GBP", "NGN", "KES", "GHS", "ZAR"})
}

// AssetCode returns a Stellar asset code (<= 12 characters).
func AssetCode() string {
	return gofakeit.RandomString([]string{"USDC", "XLM", "EURC", "NGNC"})
}

// CountryCode returns an ISO 3166-1 alpha-2 code, matching the size:2 column.
func CountryCode() string {
	return gofakeit.RandomString([]string{"US", "GB", "NG", "KE", "GH", "ZA", "DE", "FR"})
}

// Amount returns a positive monetary value rounded to two decimal places, so
// float comparisons in assertions behave predictably.
func Amount() float64 {
	return float64(gofakeit.IntRange(100, 500000)) / 100
}

// TxHash returns a 64-character hex string shaped like a Stellar transaction hash.
func TxHash() string {
	return gofakeit.Regex("[a-f0-9]{64}")
}

// ── User ────────────────────────────────────────────────────────────────────

// UserOption mutates a User during construction.
type UserOption func(*models.User)

// WithEmail overrides the generated email.
func WithEmail(email string) UserOption {
	return func(u *models.User) { u.Email = email }
}

// WithRole overrides the user's role.
func WithRole(role string) UserOption {
	return func(u *models.User) { u.Role = role }
}

// WithKYCStatus overrides the KYC state, setting KYCVerifiedAt to match so the
// two never contradict each other.
func WithKYCStatus(status string) UserOption {
	return func(u *models.User) {
		u.KYCStatus = status
		if status == "verified" {
			now := time.Now().UTC()
			u.KYCVerifiedAt = &now
		} else {
			u.KYCVerifiedAt = nil
		}
	}
}

// WithInactive marks the user disabled.
func WithInactive() UserOption {
	return func(u *models.User) { u.IsActive = false }
}

// WithLockedUntil marks the account locked, which several auth paths branch on.
func WithLockedUntil(until time.Time) UserOption {
	return func(u *models.User) {
		u.LockedUntil = &until
		u.FailedLoginAttempts = 5
	}
}

// WithMFAEnabled turns on MFA and sets the completion timestamp with it.
func WithMFAEnabled() UserOption {
	return func(u *models.User) {
		u.MFAEnabled = true
		now := time.Now().UTC()
		u.MFASetupCompletedAt = &now
	}
}

// NewUser builds an active, KYC-pending user with a unique email and address.
//
// PasswordHash is left empty on purpose: bcrypt costs ~100ms per call, and a
// factory used in a loop would dominate the runtime of a suite that never
// checks a password. Tests that need a real hash should call the model's own
// hashing method, which is the code path worth exercising anyway.
func NewUser(opts ...UserOption) *models.User {
	u := &models.User{
		Email:              gofakeit.Email(),
		Name:               gofakeit.Name(),
		StellarAddress:     StellarAddress(),
		Role:               "user",
		Country:            CountryCode(),
		KYCStatus:          "pending",
		IsActive:           true,
		DefaultCurrency:    Currency(),
		EmailNotifications: true,
		Preferences:        "{}",
	}
	for _, opt := range opts {
		opt(u)
	}
	return u
}

// NewUsers builds n distinct users.
func NewUsers(n int, opts ...UserOption) []*models.User {
	users := make([]*models.User, 0, n)
	for i := 0; i < n; i++ {
		users = append(users, NewUser(opts...))
	}
	return users
}

// ── Payment ─────────────────────────────────────────────────────────────────

// PaymentOption mutates a Payment during construction.
type PaymentOption func(*models.Payment)

// WithPaymentStatus overrides the status and fills the fields that status
// implies, so a "completed" payment is never left without a transaction hash.
func WithPaymentStatus(status string) PaymentOption {
	return func(p *models.Payment) {
		p.Status = status
		if status == "completed" && p.TxHash == "" {
			p.TxHash = TxHash()
		}
	}
}

// WithPaymentAmount sets the amount and recalculates the derived fee total.
func WithPaymentAmount(amount float64, currency string) PaymentOption {
	return func(p *models.Payment) {
		p.Amount = amount
		p.Currency = currency
		applyFees(p)
	}
}

// WithParties wires the payment to specific sender and recipient users.
func WithParties(sender, recipient *models.User) PaymentOption {
	return func(p *models.Payment) {
		p.SenderID = sender.ID
		p.SenderAccount = sender.StellarAddress
		p.RecipientID = recipient.ID
		p.RecipientAccount = recipient.StellarAddress
	}
}

// WithCrossCurrency makes the payment a conversion, which exercises the forex
// fee and converted-amount paths.
func WithCrossCurrency(target string, rate float64) PaymentOption {
	return func(p *models.Payment) {
		p.TargetCurrency = target
		p.ConvertedAmount = round2(p.Amount * rate)
		applyFees(p)
	}
}

// applyFees keeps Fee equal to the sum of its components.
//
// The model documents Fee as "the total of all fee components", so a factory
// that set them independently would produce records the application's own
// invariants say are impossible.
func applyFees(p *models.Payment) {
	p.PlatformFee = round2(p.Amount * 0.01)
	p.NetworkFee = 0.00001
	p.ComplianceFee = round2(p.Amount * 0.001)
	if p.TargetCurrency != "" && p.TargetCurrency != p.Currency {
		p.ForexFee = round2(p.Amount * 0.005)
	}
	p.Fee = round2(p.PlatformFee + p.NetworkFee + p.ComplianceFee + p.ForexFee)
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

// NewPayment builds a pending payment between two generated accounts.
func NewPayment(opts ...PaymentOption) *models.Payment {
	p := &models.Payment{
		SenderID:         uint(gofakeit.IntRange(1, 1000)),
		SenderAccount:    StellarAddress(),
		RecipientID:      uint(gofakeit.IntRange(1001, 2000)),
		RecipientAccount: StellarAddress(),
		Amount:           Amount(),
		Currency:         Currency(),
		Status:           "pending",
		Notes:            gofakeit.Sentence(6),
	}
	applyFees(p)
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// NewPayments builds n payments.
func NewPayments(n int, opts ...PaymentOption) []*models.Payment {
	payments := make([]*models.Payment, 0, n)
	for i := 0; i < n; i++ {
		payments = append(payments, NewPayment(opts...))
	}
	return payments
}

// ── Contact ─────────────────────────────────────────────────────────────────

// ContactOption mutates a Contact during construction.
type ContactOption func(*models.Contact)

// WithContactOwner attaches the contact to a user.
func WithContactOwner(userID uint) ContactOption {
	return func(c *models.Contact) { c.UserID = userID }
}

// WithContactVerified marks the contact verified, keeping the boolean and the
// status enum consistent — they are two representations of one fact.
func WithContactVerified() ContactOption {
	return func(c *models.Contact) {
		c.VerificationStatus = models.ContactVerificationVerified
		c.IsVerified = true
	}
}

// NewContact builds an unverified contact.
func NewContact(opts ...ContactOption) *models.Contact {
	c := &models.Contact{
		UserID:             uint(gofakeit.IntRange(1, 1000)),
		Nickname:           gofakeit.FirstName(),
		StellarAddress:     StellarAddress(),
		Currency:           AssetCode(),
		Email:              gofakeit.Email(),
		Notes:              gofakeit.Sentence(4),
		VerificationStatus: models.ContactVerificationPending,
		IsVerified:         false,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ── Invoice ─────────────────────────────────────────────────────────────────

// InvoiceOption mutates an Invoice during construction.
type InvoiceOption func(*models.Invoice)

// WithInvoiceStatus overrides the status.
func WithInvoiceStatus(status string) InvoiceOption {
	return func(i *models.Invoice) { i.Status = status }
}

// WithInvoiceOverdue backdates the due date so the invoice is genuinely late,
// rather than merely labelled that way.
func WithInvoiceOverdue() InvoiceOption {
	return func(i *models.Invoice) {
		due := time.Now().UTC().Add(-30 * 24 * time.Hour)
		i.DueDate = &due
		i.Status = "overdue"
	}
}

// WithInvoicePayment links the invoice to a payment.
func WithInvoicePayment(paymentID uint) InvoiceOption {
	return func(i *models.Invoice) { i.PaymentID = paymentID }
}

// NewInvoice builds an unpaid invoice due in 30 days.
func NewInvoice(opts ...InvoiceOption) *models.Invoice {
	due := time.Now().UTC().Add(30 * 24 * time.Hour)
	i := &models.Invoice{
		PaymentID: uint(gofakeit.IntRange(1, 1000)),
		// Unique per invoice: InvoiceNo carries a uniqueIndex, so a constant
		// here would make any test inserting two invoices fail confusingly.
		InvoiceNo:   fmt.Sprintf("INV-%d-%s", time.Now().UTC().Year(), gofakeit.Regex("[A-Z0-9]{8}")),
		IssuerID:    uint(gofakeit.IntRange(1, 1000)),
		RecipientID: uint(gofakeit.IntRange(1001, 2000)),
		Amount:      Amount(),
		Currency:    Currency(),
		DueDate:     &due,
		Status:      "unpaid",
		Description: gofakeit.Sentence(8),
	}
	for _, opt := range opts {
		opt(i)
	}
	return i
}

// ── PaymentRequest ──────────────────────────────────────────────────────────

// PaymentRequestOption mutates a PaymentRequest during construction.
type PaymentRequestOption func(*models.PaymentRequest)

// WithRequestStatus sets the status and the timestamp that status implies, so
// an "accepted" request always carries an AcceptedAt.
func WithRequestStatus(status string) PaymentRequestOption {
	return func(r *models.PaymentRequest) {
		r.Status = status
		now := time.Now().UTC()
		switch status {
		case "accepted":
			r.AcceptedAt = &now
		case "rejected":
			r.RejectedAt = &now
			r.RejectionReason = gofakeit.Sentence(5)
		case "paid":
			r.AcceptedAt = &now
			r.PaidAt = &now
		}
	}
}

// WithRequestExpired backdates the expiry so the request is actually expired.
func WithRequestExpired() PaymentRequestOption {
	return func(r *models.PaymentRequest) {
		expired := time.Now().UTC().Add(-24 * time.Hour)
		r.ExpiresAt = &expired
		r.Status = "expired"
	}
}

// NewPaymentRequest builds a pending request expiring in seven days.
func NewPaymentRequest(opts ...PaymentRequestOption) *models.PaymentRequest {
	expires := time.Now().UTC().Add(7 * 24 * time.Hour)
	r := &models.PaymentRequest{
		RequesterID:  uint(gofakeit.IntRange(1, 1000)),
		TargetUserID: uint(gofakeit.IntRange(1001, 2000)),
		Amount:       Amount(),
		Currency:     Currency(),
		AssetCode:    AssetCode(),
		AssetIssuer:  StellarAddress(),
		Description:  gofakeit.Sentence(6),
		Reference:    gofakeit.UUID(),
		Status:       "pending",
		ExpiresAt:    &expires,
		Notes:        gofakeit.Sentence(4),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// ── Webhook ─────────────────────────────────────────────────────────────────

// WebhookOption mutates a Webhook during construction.
type WebhookOption func(*models.Webhook)

// WithWebhookEvents overrides the subscribed event list.
func WithWebhookEvents(events ...string) WebhookOption {
	return func(w *models.Webhook) { w.Events = strings.Join(events, ",") }
}

// WithWebhookInactive disables the webhook.
func WithWebhookInactive() WebhookOption {
	return func(w *models.Webhook) { w.IsActive = false }
}

// NewWebhook builds an active webhook subscribed to payment events.
func NewWebhook(opts ...WebhookOption) *models.Webhook {
	w := &models.Webhook{
		UserID: uint(gofakeit.IntRange(1, 1000)),
		URL:    gofakeit.URL(),
		// Long enough to be a plausible HMAC key rather than a short token that
		// would pass a length check the real secret would not.
		Secret:      gofakeit.Regex("[a-zA-Z0-9]{48}"),
		Events:      "payment.completed,payment.failed",
		IsActive:    true,
		Description: gofakeit.Sentence(5),
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// WebhookDeliveryOption mutates a WebhookDelivery during construction.
type WebhookDeliveryOption func(*models.WebhookDelivery)

// WithDeliveryFailed marks the delivery failed and populates the fields the
// retry path reads: an attempt count, an error, and a scheduled retry.
func WithDeliveryFailed(attempts int) WebhookDeliveryOption {
	return func(d *models.WebhookDelivery) {
		d.Status = "failed"
		d.AttemptCount = attempts
		d.ResponseCode = gofakeit.RandomInt([]int{500, 502, 503, 504})
		d.ErrorMessage = gofakeit.Sentence(5)
		next := time.Now().UTC().Add(time.Duration(attempts) * time.Minute)
		d.NextRetryAt = &next
	}
}

// WithDeliverySucceeded marks the delivery complete.
func WithDeliverySucceeded() WebhookDeliveryOption {
	return func(d *models.WebhookDelivery) {
		d.Status = "success"
		d.ResponseCode = 200
		d.AttemptCount = 1
		now := time.Now().UTC()
		d.CompletedAt = &now
		d.ErrorMessage = ""
		d.NextRetryAt = nil
	}
}

// NewWebhookDelivery builds a pending delivery.
func NewWebhookDelivery(opts ...WebhookDeliveryOption) *models.WebhookDelivery {
	d := &models.WebhookDelivery{
		WebhookID: uint(gofakeit.IntRange(1, 1000)),
		Event:     gofakeit.RandomString([]string{"payment.completed", "payment.failed", "invoice.paid"}),
		Payload:   `{"id":` + fmt.Sprint(gofakeit.IntRange(1, 9999)) + `}`,
		Status:    "pending",
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// ── AuditLog ────────────────────────────────────────────────────────────────

// AuditLogOption mutates an AuditLog during construction.
type AuditLogOption func(*models.AuditLog)

// WithAuditActor attaches the entry to a user.
func WithAuditActor(userID uint) AuditLogOption {
	return func(a *models.AuditLog) { a.UserID = &userID }
}

// WithAuditAction overrides the recorded action.
func WithAuditAction(action, resource string) AuditLogOption {
	return func(a *models.AuditLog) {
		a.Action = action
		a.Resource = resource
	}
}

// NewAuditLog builds an audit entry for a payment creation.
func NewAuditLog(opts ...AuditLogOption) *models.AuditLog {
	a := &models.AuditLog{
		Action:    "payment.create",
		Resource:  fmt.Sprintf("payment:%d", gofakeit.IntRange(1, 9999)),
		OldValue:  "{}",
		NewValue:  `{"status":"pending"}`,
		IPAddress: gofakeit.IPv4Address(),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// nowPlus returns a timestamp offset from now by the given number of days.
// Used by tests to express "in the past" / "in the future" readably.
func nowPlus(days int) time.Time {
	return time.Now().UTC().AddDate(0, 0, days)
}
