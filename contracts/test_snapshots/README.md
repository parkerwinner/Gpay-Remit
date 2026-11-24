# Contract regression snapshots

The JSON files in this directory are Proptest regression cases. `cargo test --all-features` replays them against the current contract tests. The manifest pins both the source/test revision and each regression case so changes prompt an explicit review and update.

After changing contract logic or intentionally adding regression cases, run:

```sh
UPDATE_SNAPSHOTS=1 python scripts/check_contract_snapshots.py
```

The command runs the complete locked contract test suite first. It updates the manifest only when that run succeeds. Review snapshot and manifest changes together before committing. CI runs the contract suite and validates the manifest on every pull request.
