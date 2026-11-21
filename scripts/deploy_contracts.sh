#!/usr/bin/env bash
# Build, deploy, initialise and smoke-test the Gpay-Remit Soroban contracts.
#
# Used by the `deploy-contracts-testnet` job in .github/workflows/ci.yml (#378)
# and runnable locally. Every step that can fail stops the script (set -e),
# so a non-zero exit means the deployment is not usable.
#
# Contracts (DEPLOY_CONTRACTS, space separated, default "escrow hub"):
#   escrow  PaymentEscrowContract, initialised with init_escrow(admin)
#   hub     RemittanceHubContract, initialised with init_hub(admin, oracles…).
#           Needs two price oracles: PRIMARY_ORACLE_ID / SECONDARY_ORACLE_ID,
#           or, on test networks only, two freshly deployed mock oracles.
#
# Environment:
#   NETWORK                  testnet (default), futurenet, local or mainnet
#   STELLAR_DEPLOYER_SECRET  deployer secret key (S…). Read from the
#                            environment only, never printed. Optional on test
#                            networks: without it a new identity is generated
#                            and funded by friendbot (it is then the contracts'
#                            admin, and its key is lost when the runner ends).
#   OUT_DIR                  where addresses and wasm go
#                            (default deployments/$NETWORK)
#   ORACLE_MAX_STALENESS     hub oracle max staleness, seconds (default 3600)
#   DRY_RUN=1                build and size-check only, print what would run
#   SIZE_LIMIT_MODE          fail (default) or warn when a wasm exceeds the
#                            network's contract size limit
#   MAX_WASM_BYTES           override the size limit read from the network
#   ALLOW_MAINNET=1          required, with STELLAR_DEPLOYER_SECRET and real
#                            oracle ids, to deploy to mainnet
#
# Outputs, in $OUT_DIR:
#   contracts.json  network, commit, deployer address and, per contract, id,
#                   wasm sha256 (the on-chain wasm hash) and size
#   contracts.env   the same ids as KEY=VALUE lines (e.g. ESCROW_CONTRACT_ID)
#   wasm/<name>.wasm  the exact artifacts deployed
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MANIFEST="$ROOT/contracts/Cargo.toml"
PACKAGE="gpay-remit-contracts"
WASM_PROFILE="release-wasm"

NETWORK="${NETWORK:-testnet}"
DEPLOY_CONTRACTS="${DEPLOY_CONTRACTS:-escrow hub}"
OUT_DIR="${OUT_DIR:-$ROOT/deployments/$NETWORK}"
WASM_DIR="$OUT_DIR/wasm"
IDENTITY="${DEPLOYER_IDENTITY:-gpay-deployer-$NETWORK}"
ORACLE_MAX_STALENESS="${ORACLE_MAX_STALENESS:-3600}"
DRY_RUN="${DRY_RUN:-0}"
SIZE_LIMIT_MODE="${SIZE_LIMIT_MODE:-fail}"
DEFAULT_MAX_WASM_BYTES=131072

log() { echo "==> $*" >&2; }
warn() {
  if [ -n "${GITHUB_ACTIONS:-}" ]; then echo "::warning::$*" >&2; else echo "warning: $*" >&2; fi
}
die() {
  if [ -n "${GITHUB_ACTIONS:-}" ]; then echo "::error::$*" >&2; else echo "error: $*" >&2; fi
  exit 1
}

is_test_network() { case "$NETWORK" in testnet | futurenet | local) return 0 ;; *) return 1 ;; esac; }

sha256() {
  if command -v sha256sum >/dev/null; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

# Only these feature names are valid; anything else in DEPLOY_CONTRACTS is a typo.
feature_for() {
  case "$1" in
    escrow) echo contract-escrow ;;
    hub) echo contract-hub ;;
    mocks) echo contract-mocks ;;
    *) die "unknown contract '$1' in DEPLOY_CONTRACTS (expected: escrow, hub)" ;;
  esac
}

# --- Preflight ---------------------------------------------------------------

command -v stellar >/dev/null || die "stellar CLI not found; see https://developers.stellar.org/docs/tools/cli"
command -v cargo >/dev/null || die "cargo not found"
command -v python3 >/dev/null || die "python3 not found (used to read CLI JSON output)"

for name in $DEPLOY_CONTRACTS; do feature_for "$name" >/dev/null; done

if [ "$NETWORK" = "mainnet" ]; then
  [ "${ALLOW_MAINNET:-0}" = "1" ] || die "refusing to deploy to mainnet without ALLOW_MAINNET=1"
  [ -n "${STELLAR_DEPLOYER_SECRET:-}" ] || die "mainnet deploys need STELLAR_DEPLOYER_SECRET"
elif ! is_test_network; then
  die "unknown NETWORK '$NETWORK' (expected testnet, futurenet, local or mainnet)"
fi

case "$SIZE_LIMIT_MODE" in fail | warn) ;; *) die "SIZE_LIMIT_MODE must be fail or warn" ;; esac

mkdir -p "$WASM_DIR"
log "network: $NETWORK · contracts: $DEPLOY_CONTRACTS · output: $OUT_DIR"

# --- Build -------------------------------------------------------------------

# Each contract is its own wasm, selected by a Cargo feature (see
# contracts/Cargo.toml): the escrow and the hub share entry-point names, so
# one wasm cannot export both.
build_wasm() {
  local name="$1" feature
  feature="$(feature_for "$name")"
  local dir="$WASM_DIR/.build-$name"
  log "building $name (--features $feature, profile $WASM_PROFILE, optimized)"
  stellar contract build \
    --manifest-path "$MANIFEST" --package "$PACKAGE" \
    --profile "$WASM_PROFILE" --features "$feature" \
    --locked --optimize --out-dir "$dir" >&2
  mv "$dir/gpay_remit_contracts.wasm" "$WASM_DIR/$name.wasm"
  rm -rf "$dir"
}

max_wasm_bytes() {
  if [ -n "${MAX_WASM_BYTES:-}" ]; then echo "$MAX_WASM_BYTES" && return; fi
  local limit
  limit="$(stellar network settings --network "$NETWORK" --output json 2>/dev/null |
    python3 -c 'import json,sys
try:
    entries = json.load(sys.stdin).get("updated_entry", [])
    print(next(e["contract_max_size_bytes"] for e in entries if "contract_max_size_bytes" in e))
except Exception:
    pass' || true)"
  echo "${limit:-$DEFAULT_MAX_WASM_BYTES}"
}

TO_BUILD="$DEPLOY_CONTRACTS"
case " $DEPLOY_CONTRACTS " in
  *" hub "*) [ -n "${PRIMARY_ORACLE_ID:-}" ] || TO_BUILD="$TO_BUILD mocks" ;;
esac

for name in $TO_BUILD; do build_wasm "$name"; done

LIMIT="$(max_wasm_bytes)"
for name in $TO_BUILD; do
  size="$(wc -c <"$WASM_DIR/$name.wasm" | tr -d ' ')"
  log "$name.wasm: $size bytes (network limit $LIMIT), sha256 $(sha256 "$WASM_DIR/$name.wasm")"
  if [ "$size" -gt "$LIMIT" ]; then
    msg="$name.wasm is $size bytes; $NETWORK accepts at most $LIMIT"
    if [ "$SIZE_LIMIT_MODE" = "warn" ]; then warn "$msg"; else die "$msg"; fi
  fi
done

if [ "$DRY_RUN" = "1" ]; then
  log "DRY_RUN=1: built and size-checked; nothing deployed"
  for name in $DEPLOY_CONTRACTS; do
    echo "would deploy: stellar contract deploy --wasm $WASM_DIR/$name.wasm --source $IDENTITY --network $NETWORK"
  done
  exit 0
fi

# --- Deployer identity -------------------------------------------------------

if [ -n "${STELLAR_DEPLOYER_SECRET:-}" ]; then
  log "importing deployer identity '$IDENTITY' from STELLAR_DEPLOYER_SECRET"
  STELLAR_SECRET_KEY="$STELLAR_DEPLOYER_SECRET" \
    stellar keys add "$IDENTITY" --secret-key --overwrite </dev/null >/dev/null
  if is_test_network; then
    stellar keys fund "$IDENTITY" --network "$NETWORK" >/dev/null 2>&1 ||
      log "friendbot funding skipped (account is probably funded already)"
  fi
else
  is_test_network || die "STELLAR_DEPLOYER_SECRET is required on $NETWORK"
  log "no STELLAR_DEPLOYER_SECRET: generating and funding a fresh identity '$IDENTITY'"
  stellar keys generate "$IDENTITY" --network "$NETWORK" --fund --overwrite >/dev/null
fi
ADMIN="$(stellar keys public-key "$IDENTITY")"
log "deployer/admin: $ADMIN"

# --- Deploy, initialise, smoke-test -----------------------------------------

invoke() { # invoke <contract-id> <send: yes|no> <fn> [args…]
  local id="$1" send="$2" fn="$3"
  shift 3
  stellar contract invoke --id "$id" --source "$IDENTITY" --network "$NETWORK" \
    --send "$send" -- "$fn" "$@"
}

deploy() { # deploy <name> -> prints the contract id
  local name="$1" id
  id="$(stellar contract deploy --wasm "$WASM_DIR/$name.wasm" \
    --source "$IDENTITY" --network "$NETWORK" 2>/dev/null | tail -n1 | tr -d '[:space:]')"
  [[ "$id" =~ ^C[A-Z2-7]{55}$ ]] || die "deploying $name did not return a contract id"
  echo "$id"
}

expect() { # expect <description> <actual> <regex>
  if [[ "$2" =~ $3 ]]; then
    log "smoke ok: $1 -> $2"
  else
    die "smoke test failed: $1 returned '$2', expected /$3/"
  fi
}

# One line per deployed contract: "<key> <contract id> <wasm name>".
# (Plain text rather than an associative array keeps bash 3.2 working.)
RECORD="$OUT_DIR/.deployed"
: >"$RECORD"
record() { echo "$1 $2 $3" >>"$RECORD"; }

deploy_oracle() { # deploy_oracle <label> -> prints the oracle's contract id
  local id
  id="$(deploy mocks)"
  invoke "$id" yes init_oracle --admin "$ADMIN" >/dev/null
  log "mock oracle ($1): $id"
  record "oracle_$1" "$id" mocks
  echo "$id"
}

for name in $DEPLOY_CONTRACTS; do
  case "$name" in
    escrow)
      id="$(deploy escrow)"
      log "escrow: $id"
      invoke "$id" yes init_escrow --admin "$ADMIN" >/dev/null
      expect "escrow.get_platform_fee" "$(invoke "$id" no get_platform_fee)" '^"?-?[0-9]+"?$'
      expect "escrow.get_total_escrow_volume" "$(invoke "$id" no get_total_escrow_volume)" '^"?0"?$'
      ;;
    hub)
      primary="${PRIMARY_ORACLE_ID:-}"
      secondary="${SECONDARY_ORACLE_ID:-$primary}"
      if [ -z "$primary" ]; then
        is_test_network || die "set PRIMARY_ORACLE_ID (and SECONDARY_ORACLE_ID) to deploy the hub on $NETWORK"
        primary="$(deploy_oracle primary)"
        secondary="$(deploy_oracle secondary)"
      fi
      id="$(deploy hub)"
      log "hub: $id"
      invoke "$id" yes init_hub --admin "$ADMIN" --primary_oracle "$primary" \
        --secondary_oracle "$secondary" --max_staleness "$ORACLE_MAX_STALENESS" >/dev/null
      expect "hub.is_paused" "$(invoke "$id" no is_paused)" '^false$'
      expect "hub.version" "$(invoke "$id" no version)" '^[0-9]+$'
      expect "hub.get_oracle_config.primary_oracle" "$(invoke "$id" no get_oracle_config)" "\"primary_oracle\":\"$primary\""
      ;;
  esac
  record "$name" "$id" "$name"
done

# --- Record --------------------------------------------------------------------

sort "$RECORD" -o "$RECORD"
{
  while read -r key id _; do
    echo "$(echo "$key" | tr '[:lower:]' '[:upper:]')_CONTRACT_ID=$id"
  done <"$RECORD"
  echo "STELLAR_NETWORK=$NETWORK"
  echo "CONTRACTS_ADMIN=$ADMIN"
} >"$OUT_DIR/contracts.env"

entries=""
while read -r key id wasm_name; do
  wasm="$WASM_DIR/$wasm_name.wasm"
  entries+="$key $id $(sha256 "$wasm") $(wc -c <"$wasm" | tr -d ' ')"$'\n'
done <"$RECORD"
rm -f "$RECORD"
NETWORK="$NETWORK" ADMIN="$ADMIN" COMMIT="$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || echo unknown)" \
  ENTRIES="$entries" python3 - >"$OUT_DIR/contracts.json" <<'PY'
import json, os, datetime
contracts = {}
for line in os.environ["ENTRIES"].strip().splitlines():
    name, cid, sha, size = line.split()
    contracts[name] = {"contract_id": cid, "wasm_sha256": sha, "wasm_bytes": int(size)}
print(json.dumps({
    "network": os.environ["NETWORK"],
    "commit": os.environ["COMMIT"],
    "deployed_at": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "admin": os.environ["ADMIN"],
    "contracts": contracts,
}, indent=2, sort_keys=True))
PY

log "deployment recorded in $OUT_DIR/contracts.json and $OUT_DIR/contracts.env"
cat "$OUT_DIR/contracts.env"
