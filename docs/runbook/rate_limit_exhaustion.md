# Runbook: Rate Limit Exhaustion

## Severity
- **SEV-2** (if affecting legitimate anchor or partner traffic)
- **SEV-3** (if isolated to public unauthenticated endpoints)

## Symptoms
- Alert: Spike in `429 Too Many Requests` responses.
- Legitimate clients or corporate NAT IP addresses blocked from API.
- Logs report: `rate limit exceeded for key: ip:<client_ip>`.

## Diagnostic Steps
1. Identify high-frequency client IPs:
   ```bash
   docker compose logs backend | grep "429" | awk '{print $NF}' | sort | uniq -c | sort -nr | head -n 10
   ```
2. Check Redis rate limit keys:
   ```bash
   redis-cli --scan --pattern "ratelimit:*"
   ```
3. Verify if requests are legitimate bursts or DDoS/credential stuffing attempts.

## Mitigation & Recovery
1. **Reset Rate Limit for Legitimate Client**:
   - If an enterprise partner hit the threshold during high volume:
     ```bash
     redis-cli del "ratelimit:ip:<partner_ip>"
     ```
2. **Whitelist / Exempt Trusted IPs**:
   - Add partner IP or CIDR block to `TRUSTED_PROXIES` or `RATE_LIMIT_EXEMPT_IPS` in `.env`:
     ```bash
     RATE_LIMIT_EXEMPT_IPS=192.168.1.50,10.0.0.0/8
     ```
3. **Block Malicious Actors**:
   - If traffic is malicious, drop traffic at the load balancer / WAF layer before it reaches application containers:
     ```bash
     iptables -A INPUT -s <malicious_ip> -j DROP
     ```
