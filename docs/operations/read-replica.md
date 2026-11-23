# Read replica configuration

Set `DATABASE_READ_REPLICA_URL` to a PostgreSQL standby connection string to route analytics, remittance list/detail, and invoice reads away from the primary. Writes and authentication stay on `DATABASE_URL`. If the variable is unset or the replica cannot be opened, the application uses the primary for reads.

Before each routed read, the backend checks PostgreSQL replay lag. If the standby reports more than five seconds of lag, or the health query fails, that request uses the primary. This keeps read-after-write-sensitive endpoints from serving stale replica data while allowing the replica to absorb routine reporting and history traffic.

The read replica uses the existing `DB_MAX_IDLE_CONNS`, `DB_MAX_OPEN_CONNS`, and `DB_CONN_MAX_LIFETIME_MIN` pool settings. Grant its database role read-only access and keep its schema migrations synchronized with the primary.
