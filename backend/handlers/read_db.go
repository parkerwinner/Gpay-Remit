package handlers

import (
	"gorm.io/gorm"
)

const maxReadReplicaLagSeconds = 5.0

// preferredReadDB checks replica freshness before routing a read. An
// unavailable, stale, or non-Postgres replica falls back to the primary.
func preferredReadDB(primary, replica *gorm.DB) *gorm.DB {
	if replica == nil || replica == primary {
		return primary
	}
	var lagSeconds float64
	err := replica.Raw(`SELECT CASE WHEN pg_last_xact_replay_timestamp() IS NULL THEN 0 ELSE EXTRACT(EPOCH FROM (clock_timestamp() - pg_last_xact_replay_timestamp())) END`).Scan(&lagSeconds).Error
	if err != nil || lagSeconds > maxReadReplicaLagSeconds {
		return primary
	}
	return replica
}
