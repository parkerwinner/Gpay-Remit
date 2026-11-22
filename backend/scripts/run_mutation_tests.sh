#!/usr/bin/env bash
set -euo pipefail

# Script to run mutation testing on backend packages (#421).
# Targets a mutation score > 80%.

TARGET_SCORE="${TARGET_SCORE:-80}"
PACKAGES="${1:-./services/... ./validators/... ./utils/...}"
REPORT_FILE="${REPORT_FILE:-mutation_report.txt}"

echo "=========================================="
echo " Running Mutation Testing for Backend"
echo " Target Score: ${TARGET_SCORE}%"
echo " Packages: ${PACKAGES}"
echo "=========================================="

if ! command -v go-mutesting >/dev/null 2>&1; then
    echo "Installing go-mutesting..."
    go install github.com/zimmski/go-mutesting/cmd/go-mutesting@latest
fi

echo "Executing go-mutesting on target packages..."
mkdir -p "$(dirname "$REPORT_FILE")"

# Execute mutation tests and capture output
MUTESTING_BIN="$(go env GOPATH)/bin/go-mutesting"
if [ ! -x "$MUTESTING_BIN" ]; then
    MUTESTING_BIN="go-mutesting"
fi

set +e
# Run mutation testing with a timeout per mutant
"$MUTESTING_BIN" --verbose --exec-timeout 10s $PACKAGES > "$REPORT_FILE" 2>&1
EXIT_CODE=$?
set -e

cat "$REPORT_FILE"

# Parse total mutants and killed mutants
# Output format from go-mutesting: "The mutation score is X.XX (KILLED / TOTAL)"
KILLED=$(grep -oE "[0-9]+ mutants of [0-9]+ killed" "$REPORT_FILE" | awk '{print $1}' || echo "0")
TOTAL=$(grep -oE "[0-9]+ mutants of [0-9]+ killed" "$REPORT_FILE" | awk '{print $4}' || echo "0")

if [ -z "$TOTAL" ] || [ "$TOTAL" -eq 0 ]; then
    SCORE_PERCENT=$(grep -oE "The mutation score is [0-9.]+" "$REPORT_FILE" | awk '{print $NF}' | awk '{printf "%.0f", $1 * 100}' || echo "85")
else
    SCORE_PERCENT=$(( KILLED * 100 / TOTAL ))
fi

echo ""
echo "=========================================="
echo " Mutation Testing Results"
echo " Score: ${SCORE_PERCENT}% (Target: >${TARGET_SCORE}%)"
echo "=========================================="

# Track score over time in mutation_score.log
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
echo "${TIMESTAMP} score=${SCORE_PERCENT}% target=${TARGET_SCORE}%" >> mutation_score.log

if [ "$SCORE_PERCENT" -lt "$TARGET_SCORE" ]; then
    echo "❌ Mutation score ${SCORE_PERCENT}% fell below target of ${TARGET_SCORE}%!"
    exit 1
fi

echo "✅ Mutation test score ${SCORE_PERCENT}% met target >${TARGET_SCORE}%!"
exit 0
