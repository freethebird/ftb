# Validate the build with GitHub Actions (no server)

The fastest way to prove the code compiles and the tests pass, with full internet and no VPS. The
workflow `.github/workflows/ci.yml` already does the exact build: Go 1.26, the neo-go v0.123.0
compiler, compile every contract, and run every test. Push the repo and it runs on GitHub's
runners, free for public repos and within the free minutes for private ones.

## Steps

1. Create a GitHub repository (private is fine). Do not initialize it with any files.

2. From the repo folder on your machine, push it. You need `git` and a GitHub account; you do not
   need Go locally, since the build runs on GitHub.

   ```bash
   cd ftb
   git init
   git add .
   git commit -m "FTB: contracts, tooling, audit, web, ops"
   git branch -M main
   git remote add origin git@github.com:YOURNAME/ftb.git   # or the https URL
   git push -u origin main
   ```

3. Open the repository's Actions tab. The push triggers two jobs:
   - `build-test`: resolves modules, installs neo-go, compiles the contracts, runs every test.
     This is the substantive signal.
   - `lint`: gofmt and go vet. Kept separate so a formatting nit never hides the build result.

## What to expect on the first run

`build-test` is the one that answers "does it work." If it is green, the contracts compile and the
whole test suite passes on a real toolchain.

Two first-run things are normal and quick to clear:

- `lint` may fail on gofmt, because the Go here was written by hand, not run through the formatter.
  This is cosmetic and auto-fixable: run `gofmt -w .` once (any machine with Go, including the CI
  logs will name the files), commit, and push. It does not affect whether the code works.
- A test may fail on a neo-go version detail. The test files call it out: the `stackitem` shape for
  void methods, and the `InvokeAndCheck` helper used by the interest-accrual test. These are
  one-line adjustments.

Either way, paste me the failing step's log (or just the step name and the error) and I will give
you the exact change. After the first green run, commit the generated `go.sum` so builds are fully
reproducible.

## Why this instead of a server, for now

Validating the code needs full internet for a few minutes, which is exactly what a CI runner is.
A VPS (1984 or Atlas) is worth it when you need an always-on box, which is the TestNet deployment
phase, not the compile-and-test phase. The VPS runbook (`ops/1984_SETUP.md`) and the one-command
bootstrap (`ops/ftb-env-setup.sh`) are ready for that step when you reach it, and they work on Atlas
too, since both hand you a Debian or Ubuntu VM.
