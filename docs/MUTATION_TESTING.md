# Mutation Testing Guide

> Issue [#421](https://github.com/parkerwinner/Gpay-Remit/issues/421) — Mutation Testing for Backend Reliability

---

## 1. Overview

Code coverage metrics only measure which lines of code were executed during a test run; they do not verify whether the assertions are strong enough to detect bugs.

**Mutation testing** systematically introduces small semantic faults (mutants) into the codebase:
- Inverting boolean conditions (`if x == y` -> `if x != y`)
- Swapping arithmetic operators (`+` -> `-`, `*` -> `/`)
- Removing statements or returning default values early

If the existing test suite fails when a mutant is introduced, the mutant is considered **killed** (good). If the test suite still passes despite the altered logic, the mutant **survives** (test gap).

The **Mutation Score** is calculated as:
$$\text{Mutation Score} = \left(\frac{\text{Killed Mutants}}{\text{Total Mutants}}\right) \times 100$$

Gpay-Remit enforces a target score of **>80%** across core backend domain packages.

---

## 2. Running Mutation Tests Locally

Run via `make` from the `backend/` directory:
```bash
cd backend
make mutation-test
```

Or invoke the runner script directly with custom targets:
```bash
cd backend
./scripts/run_mutation_tests.sh ./services/...
```

To adjust the target score threshold:
```bash
TARGET_SCORE=85 ./scripts/run_mutation_tests.sh ./validators/...
```

---

## 3. Continuous Integration & Tracking

Mutation testing is integrated into `.github/workflows/mutation-testing.yml` and `.github/workflows/ci.yml`:
- Runs automated mutation tests on changes to backend packages.
- Uploads `mutation_report.txt` and `mutation_score.log` as build artifacts to track score trends over time.
- Fails the build if the overall mutation score falls below the 80% threshold.
