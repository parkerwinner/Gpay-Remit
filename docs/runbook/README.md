# Gpay-Remit Operations Runbook

This directory contains standardized incident response runbooks and operational procedures for common system issues across the Gpay-Remit ecosystem.

---

## Incident Severity Levels

| Severity | Definition | Response SLA | Examples |
|---|---|---|---|
| **SEV-1 (Critical)** | Core remittance service completely down; funds stuck; database corrupted. | < 15 minutes | PostgreSQL down, Stellar network unreachable, contract unexpectedly paused. |
| **SEV-2 (Major)** | Major functionality degraded; remittances slow or failing intermittently. | < 30 minutes | Redis unavailable (cache down, rate limiter degraded), webhook delivery backlog. |
| **SEV-3 (Minor)** | Non-critical component failing; workaround available. | < 2 hours | Exchange rate oracle stale (cached rates active), reporting/analytics lag. |
| **SEV-4 (Low)** | Minor bug or cosmetic issue with negligible impact on payments. | Next business day | Minor dashboard UI glitch, non-blocking log format warning. |

---

## Escalation Matrix & Contact Directory

When an incident cannot be resolved within the response SLA, escalate immediately to the on-call engineer and incident commander.

| Role | Contact | Communication Channel | Escalation Trigger |
|---|---|---|---|
| **Primary On-Call** | `oncall@gpay-remit.example.com` | `#incidents-sev1` (Slack) / PagerDuty | Initial alert trigger (automated) |
| **Secondary On-Call** | `oncall-secondary@gpay-remit.example.com` | PagerDuty | Primary fails to ack in 10 mins |
| **Engineering Lead** | `eng-lead@gpay-remit.example.com` | Slack `@eng-lead` / Phone | SEV-1 incident ongoing > 30 mins |
| **Blockchain Specialist** | `stellar-ops@gpay-remit.example.com` | `#stellar-core` (Slack) | Soroban contract or Horizon issue |
| **Database Administrator** | `dba@gpay-remit.example.com` | `#db-ops` (Slack) | DB replication or pool exhaustion |

---

## Runbook Directory

- [Database Connection Failure](database_failure.md)
- [Redis Unavailable](redis_unavailable.md)
- [Stellar Horizon Outage](stellar_horizon_outage.md)
- [Contract Emergency Pause](contract_emergency_pause.md)
- [Webhook Delivery Queue Buildup](webhook_queue_buildup.md)
- [Rate Limit Exhaustion](rate_limit_exhaustion.md)
