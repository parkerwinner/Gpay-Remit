#![cfg(all(test, feature = "prop"))]

//! Property tests for escrow release conditions (`verify_conditions`).
//!
//! # Shrinking (#376)
//!
//! Inputs come from the small types below, each with an `Arbitrary`
//! implementation whose strategy shrinks toward the *simplest* failing case:
//!
//! * [`Kind`] shrinks toward `Timestamp`, the first and simplest condition.
//! * [`Cond`] shrinks `required` toward `false` and `threshold` toward `0`.
//! * [`Clock`] is generated as a base time plus a signed offset to the
//!   expiration, not as two unrelated `u64`s. A failure therefore shrinks to
//!   the boundary that matters (`ledger_time == expiration`, offset ±1)
//!   instead of two 20-digit numbers.
//! * [`Approvals`] shrinks both counts toward `0`.
//! * [`Scenario`] combines them and shrinks its condition list by dropping
//!   conditions, so a counterexample keeps only the conditions it needs.
//!
//! `strategies_shrink_to_a_minimal_counterexample` checks this on a property
//! that is deliberately false.
//!
//! # Reproducing a failure
//!
//! * The case count honours `PROPTEST_CASES` (CI sets it); without it the
//!   suite runs [`DEFAULT_CASES`].
//! * `PROPTEST_RNG_SEED=<u64>` makes a run deterministic. CI prints the seed
//!   it used, so `PROPTEST_RNG_SEED=<seed> cargo test --features prop --test
//!   prop_conditions` replays a CI failure locally.
//! * Every failure's minimal case is also saved to
//!   `tests/prop_conditions.proptest-regressions`, which proptest replays
//!   first on every later run. CI uploads it when the job fails; commit it to
//!   pin the regression.

use gpay_remit_contracts::payment_escrow::{
    Asset, ConditionOperator, ConditionType, PaymentEscrowContract, PaymentEscrowContractClient,
};
use proptest::prelude::*;
use proptest::test_runner::{RngAlgorithm, TestError, TestRng, TestRunner};
use soroban_sdk::{
    testutils::{Address as _, Ledger},
    Address, Env, String,
};

/// Cases per property when `PROPTEST_CASES` is not set.
const DEFAULT_CASES: u32 = 1000;

/// Shared configuration. `ProptestConfig::with_cases` would ignore
/// `PROPTEST_CASES`, so the default is applied only when the variable is
/// unset; `PROPTEST_RNG_SEED` and failure persistence come from
/// `ProptestConfig::default()`.
fn config() -> ProptestConfig {
    let mut config = ProptestConfig::default();
    if std::env::var_os("PROPTEST_CASES").is_none() {
        config.cases = DEFAULT_CASES;
    }
    // Contract calls are slow; bound shrinking so a failure still finishes.
    config.max_shrink_iters = 4096;
    config
}

// --- Key types and their shrinking strategies ---

/// A release-condition type. Variants are listed simplest first, which is
/// the order a union strategy shrinks toward.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Kind {
    Timestamp,
    KYCVerified,
    Approval,
    MultiSignature,
    OraclePrice,
}

impl Kind {
    fn contract(self) -> ConditionType {
        match self {
            Kind::Timestamp => ConditionType::Timestamp,
            Kind::KYCVerified => ConditionType::KYCVerified,
            Kind::Approval => ConditionType::Approval,
            Kind::MultiSignature => ConditionType::MultiSignature,
            Kind::OraclePrice => ConditionType::OraclePrice,
        }
    }
}

impl Arbitrary for Kind {
    type Parameters = ();
    type Strategy = BoxedStrategy<Self>;

    fn arbitrary_with(_: ()) -> Self::Strategy {
        prop_oneof![
            Just(Kind::Timestamp),
            Just(Kind::KYCVerified),
            Just(Kind::Approval),
            Just(Kind::MultiSignature),
            Just(Kind::OraclePrice),
        ]
        .boxed()
    }
}

/// Combination operator. Shrinks toward `And`.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
struct Op(ConditionOperator);

impl Arbitrary for Op {
    type Parameters = ();
    type Strategy = BoxedStrategy<Self>;

    fn arbitrary_with(_: ()) -> Self::Strategy {
        prop_oneof![
            Just(Op(ConditionOperator::And)),
            Just(Op(ConditionOperator::Or))
        ]
        .boxed()
    }
}

/// One release condition. `threshold` only matters for `OraclePrice`; the
/// range includes negatives because the contract accepts them.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
struct Cond {
    kind: Kind,
    required: bool,
    threshold: i128,
}

impl Arbitrary for Cond {
    type Parameters = ();
    type Strategy = BoxedStrategy<Self>;

    fn arbitrary_with(_: ()) -> Self::Strategy {
        // bool shrinks to false; the range shrinks toward 0.
        (any::<Kind>(), any::<bool>(), -100i128..2000)
            .prop_map(|(kind, required, threshold)| Cond {
                kind,
                required,
                threshold,
            })
            .boxed()
    }
}

/// Ledger time and escrow expiration. Built from a base and a signed offset
/// so shrinking converges on the `ledger_time == expiration` boundary. The
/// base includes the extremes of `u64` to exercise saturation.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
struct Clock {
    ledger_time: u64,
    expiration: u64,
}

impl Clock {
    fn from_offset(base: u64, offset: i64) -> Self {
        let expiration = if offset >= 0 {
            base.saturating_add(offset as u64)
        } else {
            base.saturating_sub(offset.unsigned_abs())
        };
        Clock {
            ledger_time: base,
            expiration,
        }
    }
}

impl Arbitrary for Clock {
    type Parameters = ();
    type Strategy = BoxedStrategy<Self>;

    fn arbitrary_with(_: ()) -> Self::Strategy {
        let base = prop_oneof![
            4 => 0u64..2_000_000_000,
            1 => Just(0u64),
            1 => Just(u64::MAX),
            1 => any::<u64>(),
        ];
        let offset = prop_oneof![
            4 => -1_000_000i64..1_000_000,
            1 => any::<i64>(),
        ];
        (base, offset)
            .prop_map(|(b, o)| Clock::from_offset(b, o))
            .boxed()
    }
}

/// Approvals granted and approvals required; both shrink toward 0.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
struct Approvals {
    current: u32,
    min: u32,
}

impl Arbitrary for Approvals {
    type Parameters = ();
    type Strategy = BoxedStrategy<Self>;

    fn arbitrary_with(_: ()) -> Self::Strategy {
        (0u32..10, 0u32..10)
            .prop_map(|(current, min)| Approvals { current, min })
            .boxed()
    }
}

/// A complete escrow set-up. The condition list shrinks by dropping
/// elements, every scalar shrinks toward its simplest value.
#[derive(Debug, Clone, PartialEq, Eq)]
struct Scenario {
    conds: Vec<Cond>,
    op: Op,
    proof: i128,
    clock: Clock,
    kyc: bool,
    approvals: Approvals,
}

impl Arbitrary for Scenario {
    type Parameters = ();
    type Strategy = BoxedStrategy<Self>;

    fn arbitrary_with(_: ()) -> Self::Strategy {
        (
            prop::collection::vec(any::<Cond>(), 1..10),
            any::<Op>(),
            -100i128..2000,
            any::<Clock>(),
            any::<bool>(),
            any::<Approvals>(),
        )
            .prop_map(|(conds, op, proof, clock, kyc, approvals)| Scenario {
                conds,
                op,
                proof,
                clock,
                kyc,
                approvals,
            })
            .boxed()
    }
}

// --- Model of the contract's rules (the oracle) ---

/// Whether one condition passes, per `verify_conditions`.
fn model_passes(c: &Cond, s: &Scenario) -> bool {
    match c.kind {
        Kind::Timestamp => s.clock.ledger_time >= s.clock.expiration,
        Kind::Approval | Kind::MultiSignature => s.approvals.current >= s.approvals.min,
        Kind::OraclePrice => s.proof > 0 && s.proof >= c.threshold,
        Kind::KYCVerified => s.kyc,
    }
}

/// Whether the whole set passes. AND: no required condition fails and the
/// passed count covers the required count. OR: at least one condition passes.
fn model_all_passed(s: &Scenario) -> bool {
    let passed = s.conds.iter().filter(|c| model_passes(c, s)).count();
    match s.op.0 {
        ConditionOperator::And => {
            let required = s.conds.iter().filter(|c| c.required).count();
            let failed_required = s.conds.iter().any(|c| c.required && !model_passes(c, s));
            !failed_required && (required == 0 || passed >= required)
        }
        ConditionOperator::Or => passed > 0,
    }
}

// --- Contract helpers ---

fn setup_escrow_env(env: &Env) -> (PaymentEscrowContractClient, Address, Asset) {
    let contract_id = env.register_contract(None, PaymentEscrowContract);
    let client = PaymentEscrowContractClient::new(env, &contract_id);
    let admin = Address::generate(env);
    client.init_escrow(&admin);

    let asset = Asset {
        code: String::from_str(env, "USDC"),
        issuer: Address::generate(env),
    };
    client.add_supported_asset(&admin, &asset);

    (client, admin, asset)
}

fn create_base_escrow(
    env: &Env,
    client: &PaymentEscrowContractClient,
    asset: &Asset,
    expiration: u64,
) -> u64 {
    client.create_escrow(
        &Address::generate(env),
        &Address::generate(env),
        &1000,
        asset,
        &expiration,
        &String::from_str(env, ""),
    )
}

/// Build the escrow described by `s` and return `verify_conditions().all_passed`.
fn run_scenario(s: &Scenario) -> bool {
    let env = Env::default();
    env.mock_all_auths();
    env.ledger()
        .with_mut(|li| li.timestamp = s.clock.ledger_time);

    let (client, admin, asset) = setup_escrow_env(&env);
    let escrow_id = create_base_escrow(&env, &client, &asset, s.clock.expiration);

    if s.kyc {
        client.admin_override_kyc(&admin, &escrow_id);
    }
    client.set_min_approvals(&escrow_id, &admin, &s.approvals.min);
    for _ in 0..s.approvals.current {
        client.add_approval(&escrow_id, &admin);
    }
    client.set_condition_operator(&escrow_id, &admin, &s.op.0);
    for c in &s.conds {
        client.add_condition(
            &escrow_id,
            &admin,
            &c.kind.contract(),
            &c.required,
            &c.threshold,
        );
    }

    client.verify_conditions(&escrow_id, &s.proof).all_passed
}

// --- Properties ---

proptest! {
    #![proptest_config(config())]

    /// Verification is a pure read of the escrow state: running it twice
    /// with the same proof gives the same verdict and the same failures.
    #[test]
    fn test_verify_idempotency(clock in any::<Clock>(), proof in any::<i128>(), op in any::<Op>()) {
        let env = Env::default();
        env.mock_all_auths();
        env.ledger().with_mut(|li| li.timestamp = clock.ledger_time);

        let (client, admin, asset) = setup_escrow_env(&env);
        let escrow_id = create_base_escrow(&env, &client, &asset, clock.expiration);
        client.set_condition_operator(&escrow_id, &admin, &op.0);

        let first = client.verify_conditions(&escrow_id, &proof);
        let second = client.verify_conditions(&escrow_id, &proof);

        prop_assert_eq!(first.all_passed, second.all_passed);
        prop_assert_eq!(first.failed_conditions.len(), second.failed_conditions.len());
    }

    /// A timestamp condition is monotone in time: once it passes at T it
    /// still passes at any T + delta.
    #[test]
    fn test_monotonicity_timestamp(clock in any::<Clock>(), delta in 0u64..1_000_000_000) {
        let env = Env::default();
        env.mock_all_auths();

        let (client, admin, asset) = setup_escrow_env(&env);
        let escrow_id = create_base_escrow(&env, &client, &asset, clock.expiration);
        client.set_min_approvals(&escrow_id, &admin, &0);
        client.add_condition(&escrow_id, &admin, &ConditionType::Timestamp, &true, &0);

        env.ledger().with_mut(|li| li.timestamp = clock.ledger_time);
        let at_t = client.verify_conditions(&escrow_id, &0).all_passed;
        env.ledger().with_mut(|li| li.timestamp = clock.ledger_time.saturating_add(delta));
        let later = client.verify_conditions(&escrow_id, &0).all_passed;

        prop_assert!(!at_t || later, "passed at T but not at T+{}", delta);
    }

    /// Under AND, a set of required oracle-price conditions passes exactly
    /// when the proof meets every threshold.
    #[test]
    fn test_composability_and_operator(
        thresholds in prop::collection::vec(0i128..1000, 1..5),
        proof in 0i128..2000,
    ) {
        let scenario = Scenario {
            conds: thresholds
                .iter()
                .map(|&threshold| Cond { kind: Kind::OraclePrice, required: true, threshold })
                .collect(),
            op: Op(ConditionOperator::And),
            proof,
            clock: Clock { ledger_time: 0, expiration: 0 },
            kyc: false,
            approvals: Approvals { current: 0, min: 0 },
        };
        let expected = proof > 0 && thresholds.iter().all(|&t| proof >= t);
        prop_assert_eq!(run_scenario(&scenario), expected, "proof={} thresholds={:?}", proof, thresholds);
    }

    /// With no conditions, AND is vacuously true and OR is false.
    #[test]
    fn test_empty_conditions(op in any::<Op>()) {
        let env = Env::default();
        env.mock_all_auths();
        let (client, admin, asset) = setup_escrow_env(&env);
        let escrow_id = create_base_escrow(&env, &client, &asset, 0);
        client.set_condition_operator(&escrow_id, &admin, &op.0);

        let result = client.verify_conditions(&escrow_id, &0);
        prop_assert_eq!(result.all_passed, op.0 == ConditionOperator::And);
    }

    /// A required timestamp condition passes exactly when
    /// `ledger_time >= expiration`, including at 0 and u64::MAX.
    #[test]
    fn test_timestamp_extremes(clock in any::<Clock>()) {
        let scenario = Scenario {
            conds: vec![Cond { kind: Kind::Timestamp, required: true, threshold: 0 }],
            op: Op(ConditionOperator::And),
            proof: 0,
            clock,
            kyc: false,
            approvals: Approvals { current: 0, min: 0 },
        };
        prop_assert_eq!(
            run_scenario(&scenario),
            clock.ledger_time >= clock.expiration,
            "ledger_time={} expiration={}", clock.ledger_time, clock.expiration
        );
    }

    /// For any mix of conditions, operator, clock, KYC and approvals, the
    /// contract agrees with the model in `model_all_passed`.
    #[test]
    fn test_mixed_composability(s in any::<Scenario>()) {
        prop_assert_eq!(run_scenario(&s), model_all_passed(&s), "scenario: {:#?}", s);
    }
}

// --- Shrinking checks (no contract calls, so they are fast and exact) ---

fn deterministic_runner() -> TestRunner {
    let config = ProptestConfig {
        cases: 2048,
        failure_persistence: None,
        ..ProptestConfig::default()
    };
    TestRunner::new_with_rng(config, TestRng::deterministic_rng(RngAlgorithm::ChaCha))
}

/// A deliberately false property ("no required OraclePrice condition has a
/// threshold above 500") must shrink to one condition with exactly that
/// threshold and every other field at its simplest value.
#[test]
fn strategies_shrink_to_a_minimal_counterexample() {
    let result = deterministic_runner().run(&any::<Scenario>(), |s| {
        let bad = s
            .conds
            .iter()
            .any(|c| c.kind == Kind::OraclePrice && c.required && c.threshold > 500);
        prop_assert!(!bad);
        Ok(())
    });
    let minimal = match result {
        Err(TestError::Fail(_, minimal)) => minimal,
        other => panic!("expected a counterexample, got {:?}", other),
    };
    assert_eq!(
        minimal.conds,
        vec![Cond {
            kind: Kind::OraclePrice,
            required: true,
            threshold: 501
        }]
    );
    assert_eq!(minimal.op, Op(ConditionOperator::And));
    assert_eq!(minimal.proof, 0);
    assert_eq!(
        minimal.clock,
        Clock {
            ledger_time: 0,
            expiration: 0
        }
    );
    assert!(!minimal.kyc);
    assert_eq!(minimal.approvals, Approvals { current: 0, min: 0 });
}

/// Clock shrinks to the timestamp boundary rather than to arbitrary large
/// values: the smallest clock with `ledger_time < expiration` is (0, 1).
#[test]
fn clock_shrinks_to_the_timestamp_boundary() {
    let result = deterministic_runner().run(&any::<Clock>(), |c| {
        prop_assert!(c.ledger_time >= c.expiration);
        Ok(())
    });
    match result {
        Err(TestError::Fail(_, minimal)) => {
            assert_eq!(
                minimal,
                Clock {
                    ledger_time: 0,
                    expiration: 1
                }
            )
        }
        other => panic!("expected a counterexample, got {:?}", other),
    }
}

/// The model and the contract agree on hand-picked edge cases, so the
/// oracle used by `test_mixed_composability` is itself checked.
#[test]
fn model_matches_contract_on_edge_cases() {
    let base = Scenario {
        conds: vec![],
        op: Op(ConditionOperator::And),
        proof: 0,
        clock: Clock {
            ledger_time: 10,
            expiration: 10,
        },
        kyc: false,
        approvals: Approvals { current: 0, min: 0 },
    };
    let cases = [
        Scenario {
            conds: vec![Cond {
                kind: Kind::Timestamp,
                required: true,
                threshold: 0,
            }],
            ..base.clone()
        },
        Scenario {
            conds: vec![Cond {
                kind: Kind::KYCVerified,
                required: true,
                threshold: 0,
            }],
            ..base.clone()
        },
        Scenario {
            conds: vec![Cond {
                kind: Kind::OraclePrice,
                required: false,
                threshold: -5,
            }],
            op: Op(ConditionOperator::Or),
            ..base.clone()
        },
        Scenario {
            conds: vec![Cond {
                kind: Kind::Approval,
                required: true,
                threshold: 0,
            }],
            approvals: Approvals { current: 1, min: 2 },
            ..base.clone()
        },
    ];
    for s in &cases {
        assert_eq!(run_scenario(s), model_all_passed(s), "{:?}", s);
    }
}
