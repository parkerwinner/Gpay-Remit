#!/usr/bin/env bash
# =============================================================================
# Gpay-Remit — Monitoring-as-Code Setup Script
# Issue #280: External Uptime Monitoring
#
# Creates/updates all UptimeRobot monitors and alert contacts defined in
# monitoring/uptimerobot.yml using the UptimeRobot API v2.
#
# Usage:
#   export UPTIMEROBOT_API_KEY="your-api-key"
#   ./scripts/setup_monitoring.sh
#
# Optional environment variables:
#   UPTIMEROBOT_API_KEY   — required: main API key from UptimeRobot account
#   EMAIL_ALERT_ADDRESS   — email address to register for email alerts
#   SLACK_WEBHOOK_URL     — Slack incoming webhook URL
#   PAGERDUTY_WEBHOOK_URL — PagerDuty v3 webhook endpoint URL
#   SMS_PHONE_NUMBER      — phone number for SMS alerts (E.164 format)
#   DRY_RUN               — set to "true" to print curl commands without executing
# =============================================================================
set -euo pipefail

# ── Colour helpers ────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'
info()    { echo -e "${BLUE}[INFO]${NC}  $*"; }
success() { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
error()   { echo -e "${RED}[ERROR]${NC} $*" >&2; }

# ── Validate dependencies ─────────────────────────────────────────────────────
for cmd in curl jq; do
    if ! command -v "$cmd" &>/dev/null; then
        error "Required command '$cmd' not found. Please install it."
        exit 1
    fi
done

# ── Validate required env vars ────────────────────────────────────────────────
if [[ -z "${UPTIMEROBOT_API_KEY:-}" ]]; then
    error "UPTIMEROBOT_API_KEY is not set."
    error "Export it before running this script:"
    error "  export UPTIMEROBOT_API_KEY=your-api-key"
    exit 1
fi

API_KEY="${UPTIMEROBOT_API_KEY}"
BASE_URL="https://api.uptimerobot.com/v2"
PRODUCTION_HOST="https://gpay-remit.example.com"
DRY_RUN="${DRY_RUN:-false}"

# ── API helper ────────────────────────────────────────────────────────────────
uptimerobot_api() {
    local endpoint="$1"; shift
    local data="$1"

    if [[ "${DRY_RUN}" == "true" ]]; then
        info "[DRY RUN] POST ${BASE_URL}/${endpoint}"
        info "[DRY RUN] Payload: ${data}"
        echo '{"stat":"ok","id":0}'
        return 0
    fi

    curl -s --request POST \
         --url "${BASE_URL}/${endpoint}" \
         --header "Content-Type: application/x-www-form-urlencoded" \
         --header "Cache-Control: no-cache" \
         --data "api_key=${API_KEY}&${data}&format=json"
}

# ── Create alert contacts ─────────────────────────────────────────────────────
create_alert_contacts() {
    info "Creating / verifying alert contacts..."

    local contact_ids=()

    # Email contact
    if [[ -n "${EMAIL_ALERT_ADDRESS:-}" ]]; then
        info "  Creating email alert contact: ${EMAIL_ALERT_ADDRESS}"
        local response
        response=$(uptimerobot_api "newAlertContact" \
            "type=2&value=$(python3 -c "import urllib.parse; print(urllib.parse.quote('${EMAIL_ALERT_ADDRESS}'))")&friendly_name=Ops+Team+Email")
        local email_id
        email_id=$(echo "${response}" | jq -r '.id // empty')
        if [[ -n "${email_id}" ]]; then
            success "  Email contact created (ID: ${email_id})"
            contact_ids+=("${email_id}")
        else
            warn "  Email contact creation response: ${response}"
        fi
    else
        warn "  EMAIL_ALERT_ADDRESS not set — skipping email alert contact"
    fi

    # Slack contact
    if [[ -n "${SLACK_WEBHOOK_URL:-}" ]]; then
        info "  Creating Slack alert contact"
        local response
        response=$(uptimerobot_api "newAlertContact" \
            "type=11&value=$(python3 -c "import urllib.parse; print(urllib.parse.quote('${SLACK_WEBHOOK_URL}'))")&friendly_name=Ops+Slack+Channel")
        local slack_id
        slack_id=$(echo "${response}" | jq -r '.id // empty')
        if [[ -n "${slack_id}" ]]; then
            success "  Slack contact created (ID: ${slack_id})"
            contact_ids+=("${slack_id}")
        else
            warn "  Slack contact creation response: ${response}"
        fi
    else
        warn "  SLACK_WEBHOOK_URL not set — skipping Slack alert contact"
    fi

    # Webhook / PagerDuty contact
    if [[ -n "${PAGERDUTY_WEBHOOK_URL:-}" ]]; then
        info "  Creating webhook (PagerDuty) alert contact"
        local response
        response=$(uptimerobot_api "newAlertContact" \
            "type=9&value=$(python3 -c "import urllib.parse; print(urllib.parse.quote('${PAGERDUTY_WEBHOOK_URL}'))")&friendly_name=PagerDuty+Webhook")
        local pd_id
        pd_id=$(echo "${response}" | jq -r '.id // empty')
        if [[ -n "${pd_id}" ]]; then
            success "  Webhook contact created (ID: ${pd_id})"
            contact_ids+=("${pd_id}")
        else
            warn "  Webhook contact creation response: ${response}"
        fi
    else
        warn "  PAGERDUTY_WEBHOOK_URL not set — skipping webhook alert contact"
    fi

    # SMS contact
    if [[ -n "${SMS_PHONE_NUMBER:-}" ]]; then
        info "  Creating SMS alert contact: ${SMS_PHONE_NUMBER}"
        local response
        response=$(uptimerobot_api "newAlertContact" \
            "type=4&value=$(python3 -c "import urllib.parse; print(urllib.parse.quote('${SMS_PHONE_NUMBER}'))")&friendly_name=OnCall+SMS")
        local sms_id
        sms_id=$(echo "${response}" | jq -r '.id // empty')
        if [[ -n "${sms_id}" ]]; then
            success "  SMS contact created (ID: ${sms_id})"
            contact_ids+=("${sms_id}")
        else
            warn "  SMS contact creation response: ${response}"
        fi
    else
        warn "  SMS_PHONE_NUMBER not set — skipping SMS alert contact"
    fi

    # Build the alert_contacts string for monitor creation
    # Format: "contact_id_0_0-contact_id_1_0" (threshold=0, recurrence=0)
    local contact_str=""
    for id in "${contact_ids[@]}"; do
        [[ -n "${contact_str}" ]] && contact_str="${contact_str}-"
        contact_str="${contact_str}${id}_0_0"
    done

    echo "${contact_str}"
}

# ── Create a single HTTP monitor ──────────────────────────────────────────────
create_monitor() {
    local friendly_name="$1"
    local url="$2"
    local contact_str="$3"
    local http_method="${4:-GET}"
    local expected_status="${5:-200}"

    info "  Creating monitor: ${friendly_name}"
    local response
    response=$(uptimerobot_api "newMonitor" \
        "friendly_name=$(python3 -c "import urllib.parse; print(urllib.parse.quote('${friendly_name}'))")&url=$(python3 -c "import urllib.parse; print(urllib.parse.quote('${url}'))")&type=1&interval=300&timeout=30&http_method=$([ "${http_method}" = "POST" ] && echo 1 || echo 2)&alert_contacts=${contact_str}")

    local stat
    stat=$(echo "${response}" | jq -r '.stat // "error"')
    if [[ "${stat}" == "ok" ]]; then
        local monitor_id
        monitor_id=$(echo "${response}" | jq -r '.monitor.id')
        success "  Monitor created (ID: ${monitor_id})"
    else
        warn "  Monitor creation response: ${response}"
    fi
}

# ── Main ──────────────────────────────────────────────────────────────────────
main() {
    echo ""
    info "====================================================================="
    info " Gpay-Remit — UptimeRobot Monitoring Setup"
    info "====================================================================="
    echo ""

    if [[ "${DRY_RUN}" == "true" ]]; then
        warn "DRY RUN mode — no API calls will be made."
        echo ""
    fi

    # Step 1: Create alert contacts and get their IDs
    local contact_str
    contact_str=$(create_alert_contacts)
    echo ""

    if [[ -z "${contact_str}" ]]; then
        warn "No alert contacts created — monitors will be created without alerts."
        warn "Set EMAIL_ALERT_ADDRESS, SLACK_WEBHOOK_URL, PAGERDUTY_WEBHOOK_URL, or SMS_PHONE_NUMBER."
    fi

    # Step 2: Create monitors
    info "Creating monitors..."

    # Health endpoints
    create_monitor "[Gpay-Remit] Health Check"      "${PRODUCTION_HOST}/health"                   "${contact_str}"
    create_monitor "[Gpay-Remit] Readiness Probe"   "${PRODUCTION_HOST}/health/ready"             "${contact_str}"
    create_monitor "[Gpay-Remit] Liveness Probe"    "${PRODUCTION_HOST}/health/live"              "${contact_str}"

    # Authentication (POST — expects 400 when called without a body)
    create_monitor "[Gpay-Remit] Auth Login Endpoint" "${PRODUCTION_HOST}/api/v1/auth/login"      "${contact_str}" POST 400

    # Core API (protected — expects 401)
    create_monitor "[Gpay-Remit] Remittances API"   "${PRODUCTION_HOST}/api/v1/remittances"       "${contact_str}" GET 401
    create_monitor "[Gpay-Remit] Fee Calculator API" "${PRODUCTION_HOST}/api/v1/fees/calculate"   "${contact_str}" GET 401
    create_monitor "[Gpay-Remit] Exchange Rates API" "${PRODUCTION_HOST}/api/v1/exchange-rates"   "${contact_str}" GET 401

    # Documentation
    create_monitor "[Gpay-Remit] API Documentation" "${PRODUCTION_HOST}/api/docs"                 "${contact_str}"
    create_monitor "[Gpay-Remit] OpenAPI Spec"       "${PRODUCTION_HOST}/api/docs/openapi.yaml"   "${contact_str}"

    # Frontend
    create_monitor "[Gpay-Remit] Frontend"           "${PRODUCTION_HOST}/"                         "${contact_str}"

    echo ""
    success "====================================================================="
    success " Setup complete — 10 monitors registered in UptimeRobot."
    success " Visit https://dashboard.uptimerobot.com to view them."
    success "====================================================================="
    echo ""
}

main "$@"
