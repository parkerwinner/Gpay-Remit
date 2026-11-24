#!/usr/bin/env bash
# Build, deploy, initialise and smoke-test the Gpay-Remit Soroban contracts.
#
# Usage:
#   ./scripts/deploy_contracts.sh                 # testnet, ephemeral funded deployer
#   STELLAR_DEPLOYER_SECRET=S... ./scripts/deploy_contracts.sh
#
# Environment:
#   NETWORK                  stellar CLI network name (default: testnet)
#   SOROBAN_RPC_URL          Soroban RPC endpoint (optional; used for deployment metadata)
#   SOROBAN_NETWORK_PASSPHRASE
#                            network passphrase (optional; defaults to Stellar CLI network)
#   ADMIN_SECRET             admin/deployer secret key (preferred in this script)
#   STELLAR_DEPLOYER_SECRET  deployer secret key. Optional on testnet: when
#                            unset, a throwaway key is generated and funded by
#                            friendbot (contracts are then admin-less in
#                            practice once the run ends). Required elsewhere.
#   CONTRACTS                space-separated subset of:
#                            mock-oracle mock-kyc mock-aml escrow hub
#                            (default: all, in dependency order)
#   OUT_DIR                  where addresses/logs are written (default: deployments)
#   SKIP_OVERSIZE            1 = skip (and record) contracts larger than the
#                            network's contract size limit instead of failing
#   MAX_STALENESS_SECONDS    oracle staleness passed to init_hub (default: 3600)
#
# Output: $OUT_DIR/<network>.json with each contract's id, wasm hash, size and
# status ("deployed" or "skipped_oversize"), plus the deployer public key.
# Secrets are never written to disk outside a temporary CLI config directory
# that is deleted on exit, and never passed on a command line.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONTRACTS_DIR="$ROOT_DIR/contracts"
NETWORK="${NETWORK:-testnet}"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/deployments}"
CONTRACTS="${CONTRACTS:-mock-oracle mock-kyc mock-aml escrow hub}"
SKIP_OVERSIZE="${SKIP_OVERSIZE:-0}"
MAX_STALENESS_SECONDS="${MAX_STALENESS_SECONDS:-3600}"
DEFAULT_MAX_WASM_BYTES=131072

if [ -f "$CONTRACTS_DIR/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  . "$CONTRACTS_DIR/.env"
  set +a
fi

STELLAR_DEPLOYER_SECRET="${STELLAR_DEPLOYER_SECRET:-${ADMIN_SECRET:-}}"

log() { printf '\n==> %s\n' "$*"; }
warn() {
  printf 'WARNING: %s\n' "$*" >&2
  [ -n "${GITHUB_ACTIONS:-}" ] && printf '::warning::%s\n' "$*"
  return 0
}
die() {
  printf 'ERROR: %s\n' "$*" >&2
  [ -n "${GITHUB_ACTIONS:-}" ] && printf '::error::%s\n' "$*"
  exit 1
}

command -v stellar >/dev/null || die "stellar CLI not found (https://developers.stellar.org/docs/tools/cli)"
command -v jq >/dev/null || die "jq not found"

# Isolated CLI config: identities created here disappear on exit.
STELLAR_CONFIG_DIR="$(mktemp -d)"
export STELLAR_CONFIG_DIR
trap 'rm -rf "$STELLAR_CONFIG_DIR"' EXIT
CFG=(--config-dir "$STELLAR_CONFIG_DIR")

mkdir -p "$OUT_DIR"
WASM_DIR="$OUT_DIR/wasm"
mkdir -p "$WASM_DIR"
RESULT_FILE="$OUT_DIR/$NETWORK.json"

# ── Deployer identity ─────────────────────────────────────────────────────────
log "Preparing deployer identity on $NETWORK"
if [ -n "${STELLAR_DEPLOYER_SECRET:-}" ]; then
  STELLAR_SECRET_KEY="$STELLAR_DEPLOYER_SECRET" stellar keys add deployer --secret-key "${CFG[@]}" </dev/null >/dev/null
elif [ "$NETWORK" = "testnet" ]; then
  warn "STELLAR_DEPLOYER_SECRET not set: using an ephemeral friendbot-funded deployer. Contracts will not be administrable after this run."
  stellar keys generate deployer --network "$NETWORK" --fund "${CFG[@]}" >/dev/null
else
  die "STELLAR_DEPLOYER_SECRET is required for network '$NETWORK'"
fi
DEPLOYER="$(stellar keys address deployer "${CFG[@]}")"
echo "Deployer: $DEPLOYER"

# ── Network contract size limit ───────────────────────────────────────────────
MAX_WASM_BYTES="$(stellar network settings --network "$NETWORK" "${CFG[@]}" 2>/dev/null \
  | jq -r '[.. | objects | .contract_max_size_bytes? // empty] | first // empty' || true)"
if ! [[ "$MAX_WASM_BYTES" =~ ^[0-9]+$ ]]; then
  MAX_WASM_BYTES=$DEFAULT_MAX_WASM_BYTES
  warn "could not read contract_max_size_bytes from $NETWORK; assuming $MAX_WASM_BYTES"
fi
echo "Contract size limit: $MAX_WASM_BYTES bytes"

# ── Build ─────────────────────────────────────────────────────────────────────
# Each contract is its own Wasm module (see the features in contracts/Cargo.toml).
build_contract() {
  local feature="$1" build_dir
  build_dir="$(mktemp -d)"
  stellar contract build --manifest-path "$CONTRACTS_DIR/Cargo.toml" --locked \
    --profile release-wasm --features "$feature" --optimize --out-dir "$build_dir" "${CFG[@]}" >&2
  local wasm
  wasm="$(ls "$build_dir"/*.optimized.wasm 2>/dev/null | head -1)"
  [ -n "$wasm" ] || wasm="$(ls "$build_dir"/*.wasm | head -1)"
  mv "$wasm" "$WASM_DIR/$feature.wasm"
  rm -rf "$build_dir"
  printf '%s' "$WASM_DIR/$feature.wasm"
}

# ── Deploy helpers ────────────────────────────────────────────────────────────
RESULTS='{}'
record() { # feature status [id] [hash] [size]
  RESULTS="$(jq -c --arg k "$1" --arg s "$2" --arg id "${3:-}" --arg h "${4:-}" --arg sz "${5:-0}" \
    '.[$k] = {status: $s, contract_id: ($id | select(. != "") // null), wasm_hash: ($h | select(. != "") // null), wasm_bytes: ($sz | tonumber)}' <<<"$RESULTS")"
}

invoke() { # contract_id fn args... (sends a transaction)
  local id="$1"; shift
  stellar contract invoke --id "$id" --source-account deployer --network "$NETWORK" "${CFG[@]}" -- "$@"
}

view() { # contract_id fn args... (simulation only, no transaction)
  local id="$1"; shift
  stellar contract invoke --id "$id" --source-account deployer --network "$NETWORK" --send=no "${CFG[@]}" -- "$@"
}

# Contract id of a deployed instance, or "" (bash 3.2 compatible: no
# associative arrays; ids live in $RESULTS).
id_of() { jq -r --arg k "$1" '.[$k].contract_id // empty' <<<"$RESULTS"; }

deploy_feature() { # feature [instances...]
  local feature="$1" wasm size hash
  log "Building $feature"
  wasm="$(build_contract "$feature")"
  size="$(wc -c <"$wasm" | tr -d ' ')"
  echo "$feature.wasm: $size bytes"
  if [ "$size" -gt "$MAX_WASM_BYTES" ]; then
    local msg="$feature.wasm is $size bytes, over the $NETWORK contract size limit of $MAX_WASM_BYTES bytes"
    if [ "$SKIP_OVERSIZE" = "1" ]; then
      warn "$msg; skipping"
      record "$feature" skipped_oversize "" "" "$size"
      return 0
    fi
    die "$msg (set SKIP_OVERSIZE=1 to skip it)"
  fi

  log "Uploading $feature"
  hash="$(stellar contract upload --wasm "$wasm" --source-account deployer --network "$NETWORK" "${CFG[@]}")"
  shift
  local names=("$@")
  [ ${#names[@]} -gt 0 ] || names=("$feature")
  for name in "${names[@]}"; do
    log "Deploying $name"
    local id
    id="$(stellar contract deploy --wasm-hash "$hash" --source-account deployer --network "$NETWORK" "${CFG[@]}")"
    echo "$name: $id"
    record "$name" deployed "$id" "$hash" "$size"
  done
}

want() { [[ " $CONTRACTS " == *" $1 "* ]]; }

# ── Deploy + initialise (dependency order) ────────────────────────────────────
if want mock-oracle; then
  deploy_feature mock-oracle oracle-primary oracle-secondary
  invoke "$(id_of oracle-primary)" init_oracle --admin "$DEPLOYER" >/dev/null
  invoke "$(id_of oracle-secondary)" init_oracle --admin "$DEPLOYER" >/dev/null
fi
if want mock-kyc; then
  deploy_feature mock-kyc
  if [ -n "$(id_of mock-kyc)" ]; then invoke "$(id_of mock-kyc)" init_kyc --admin "$DEPLOYER" >/dev/null; fi
fi
if want mock-aml; then
  deploy_feature mock-aml
  if [ -n "$(id_of mock-aml)" ]; then invoke "$(id_of mock-aml)" initialize --admin "$DEPLOYER" >/dev/null; fi
fi
if want escrow; then
  deploy_feature escrow
  if [ -n "$(id_of escrow)" ]; then invoke "$(id_of escrow)" init_escrow --admin "$DEPLOYER" >/dev/null; fi
fi
if want hub; then
  [ -n "$(id_of oracle-primary)" ] || die "hub needs mock-oracle in CONTRACTS (primary/secondary oracle addresses)"
  deploy_feature hub
  if [ -n "$(id_of hub)" ]; then
    invoke "$(id_of hub)" init_hub --admin "$DEPLOYER" \
      --primary_oracle "$(id_of oracle-primary)" --secondary_oracle "$(id_of oracle-secondary)" \
      --max_staleness "$MAX_STALENESS_SECONDS" >/dev/null
  fi
fi

# ── Smoke test: read back state from every deployed contract ──────────────────
log "Smoke testing deployed contracts"
FAILED=0
expect() { # description expected actual
  if [ "$2" = "$3" ]; then
    echo "  ok   $1 = $3"
  else
    echo "  FAIL $1: expected $2, got $3" >&2
    FAILED=1
  fi
}
if [ -n "$(id_of hub)" ]; then
  expect "hub.is_paused" "false" "$(view "$(id_of hub)" is_paused)"
  version="$(view "$(id_of hub)" version)"
  [[ "$version" =~ ^[0-9]+$ ]] && echo "  ok   hub.version = $version" || { echo "  FAIL hub.version: $version" >&2; FAILED=1; }
  oracle_cfg="$(view "$(id_of hub)" get_oracle_config)"
  if [[ "$oracle_cfg" == *"$(id_of oracle-primary)"* && "$oracle_cfg" == *"$(id_of oracle-secondary)"* ]]; then
    echo "  ok   hub.get_oracle_config references the deployed oracles"
  else
    echo "  FAIL hub.get_oracle_config: $oracle_cfg" >&2
    FAILED=1
  fi
fi
if [ -n "$(id_of mock-kyc)" ]; then
  expect "mock-kyc.is_kyc(deployer)" "0" "$(view "$(id_of mock-kyc)" is_kyc --account "$DEPLOYER")"
fi
if [ -n "$(id_of mock-aml)" ]; then
  expect "mock-aml.screen(deployer, deployer, 0)" "0" \
    "$(view "$(id_of mock-aml)" screen --sender "$DEPLOYER" --recipient "$DEPLOYER" --_amount 0)"
fi
if [ -n "$(id_of escrow)" ]; then
  fee="$(view "$(id_of escrow)" get_platform_fee)"
  [[ "$fee" =~ ^\"?-?[0-9]+\"?$ ]] && echo "  ok   escrow.get_platform_fee = $fee" || { echo "  FAIL escrow.get_platform_fee: $fee" >&2; FAILED=1; }
fi

jq -n --arg network "$NETWORK" --arg deployer "$DEPLOYER" --argjson limit "$MAX_WASM_BYTES" \
  --arg commit "$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null || echo unknown)" \
  --arg at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --argjson contracts "$RESULTS" \
  '{network: $network, deployed_at: $at, commit: $commit, deployer: $deployer, contract_max_size_bytes: $limit, contracts: $contracts}' \
  >"$RESULT_FILE"

log "Wrote $RESULT_FILE"
cat "$RESULT_FILE"

[ "$FAILED" = "0" ] || die "smoke test failed"
log "Deployment and smoke test passed"
