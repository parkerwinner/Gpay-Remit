# Uptime Monitoring — Gpay-Remit

> Issue [#280](https://github.com/parkerwinner/Gpay-Remit/issues/280) — External uptime monitoring via UptimeRobot

## Status

| Monitor | Status |
|---|---|
| Overall | [![Uptime Robot status](https://img.shields.io/uptimerobot/status/REPLACE_WITH_MONITOR_ID?label=Overall&style=flat-square)](https://status.gpay-remit.example.com) |
| Health Check | [![Uptime Robot ratio](https://img.shields.io/uptimerobot/ratio/REPLACE_WITH_HEALTH_MONITOR_ID?style=flat-square&label=Health)](https://status.gpay-remit.example.com) |
| Frontend | [![Uptime Robot ratio](https://img.shields.io/uptimerobot/ratio/REPLACE_WITH_FRONTEND_MONITOR_ID?style=flat-square&label=Frontend)](https://status.gpay-remit.example.com) |
| API | [![Uptime Robot ratio](https://img.shields.io/uptimerobot/ratio/REPLACE_WITH_API_MONITOR_ID?style=flat-square&label=API)](https://status.gpay-remit.example.com) |

Replace each `REPLACE_WITH_*_MONITOR_ID` badge URL parameter with the numeric
monitor ID from your UptimeRobot dashboard after running the setup script.

Public status page: https://status.gpay-remit.example.com

---

## Overview

All critical Gpay-Remit endpoints are monitored every **5 minutes** from
UptimeRobot's global probe network. Downtime triggers alerts via multiple
channels to ensure fast response times.

### Monitored Endpoints

| Endpoint | Type | Expected Response | Criticality |
|---|---|---|---|
| `GET /health` | Health | `200 OK` | Critical |
| `GET /health/ready` | Health | `200 OK` | Critical |
| `GET /health/live` | Health | `200 OK` | Critical |
| `POST /api/v1/auth/login` | Auth | `400/422` (reachability) | Critical |
| `GET /api/v1/remittances` | API | `401` (service up) | Critical |
| `GET /api/v1/fees/calculate` | API | `401` (service up) | Critical |
| `GET /api/v1/exchange-rates` | API | `401` (service up) | Critical |
| `GET /api/docs` | Docs | `200 OK` | Normal |
| `GET /api/docs/openapi.yaml` | Docs | `200 OK` + `openapi:` keyword | Normal |
| `GET /` | Frontend | `200 OK` | Critical |

### Alert Channels

| Channel | Triggers on | Escalation |
|---|---|---|
| Email (`ops-team@`) | All monitors | Manager after 15 min |
| Slack `#infra-alerts` | All monitors | `#incidents` for critical |
| PagerDuty webhook | All monitors | Auto-resolve on recovery |
| SMS (on-call) | Critical monitors only | After 5 min downtime |

---

## Setup

### Prerequisites

- UptimeRobot account (free tier supports up to 50 monitors at 5-min intervals)
- API key from: Settings → API Settings → Main API Key
- `curl` and `jq` installed

### 1. Set environment variables

```bash
export UPTIMEROBOT_API_KEY="your-main-api-key"
export EMAIL_ALERT_ADDRESS="ops-team@your-company.com"
export SLACK_WEBHOOK_URL="https://hooks.slack.com/services/T.../B.../..."
export PAGERDUTY_WEBHOOK_URL="https://events.pagerduty.com/v2/enqueue"
export SMS_PHONE_NUMBER="+1XXXXXXXXXX"
```

### 2. Run the setup script

```bash
# Preview (no API calls made)
DRY_RUN=true ./scripts/setup_monitoring.sh

# Apply
./scripts/setup_monitoring.sh
```

The script will:
1. Create alert contacts for each channel
2. Create 10 monitors covering all critical endpoints
3. Print the monitor IDs to update the badges above

### 3. (Optional) Set up a GitHub Actions secret

Store `UPTIMEROBOT_API_KEY` as a repository secret so the CI workflow can
validate the monitoring configuration on every PR.

---

## Configuration Files

| File | Purpose |
|---|---|
| `monitoring/uptimerobot.yml` | Declarative monitor definitions (IaC) |
| `monitoring/alerting.yml` | Alert channel configuration |
| `scripts/setup_monitoring.sh` | Provisioning script (UptimeRobot API v2) |
| `.github/workflows/monitoring-validation.yml` | CI: lint + content checks |

---

## CI Validation

The GitHub Actions workflow `.github/workflows/monitoring-validation.yml`
runs on every PR that touches the `monitoring/` directory and verifies:

- YAML syntax is valid (`yamllint`)
- All monitors use a 5-minute (300s) interval
- All monitors have at least one alert contact
- All required critical endpoints are present
- Setup script passes `shellcheck`
- Setup script executes successfully in `DRY_RUN` mode

---

## Runbook: Responding to Alerts

### Monitor goes DOWN

1. Check the UptimeRobot alert email/Slack for the failing URL
2. Verify manually: `curl -I https://gpay-remit.example.com/health`
3. Check pod logs: `kubectl logs -l app=gpay-remit-backend --tail=100`
4. Check k8s deployment status: `kubectl get pods -l app=gpay-remit-backend`
5. If the database is unreachable, check PostgreSQL pod status
6. Escalate to on-call if not resolved within 15 minutes

### Monitor comes back UP

UptimeRobot sends a recovery alert automatically. PagerDuty incidents are
auto-resolved. Document the incident in the team log.

### False positive (monitor shows DOWN but service is up)

1. Check UptimeRobot probe locations for network issues
2. If localised, add a comment to the incident: "False positive — regional probe issue"
3. Consider adding a second monitoring service (e.g. Freshping) for cross-validation

---

## Updating Monitors

Edit `monitoring/uptimerobot.yml` then re-run the setup script. The script
creates new monitors; to delete old ones use the UptimeRobot dashboard or
`deleteMonitor` API endpoint.
