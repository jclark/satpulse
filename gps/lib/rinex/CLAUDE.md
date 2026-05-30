# RINEX observation library

## diffobs

`diffobs` is a semantic differ for RINEX observation files. Run it as `go run ./gps/lib/rinex/diffobs a.obs[.gz] b.obs[.gz]`. It emits JSONL diff records to stdout with per-field tolerances (`-pr-tol`, `-cp-tol`, `-do-tol`, `-cn0-tol`); duplicate and out-of-order observations go to stderr; exit is 1 on diffs, 2 on errors.
