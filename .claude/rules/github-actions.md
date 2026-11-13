---
paths:
  - ".github/**"
---

# GitHub Actions

- Pin every action to a full commit SHA with the version in a comment, for example `actions/checkout@<sha> # v7.0.1`. Use only releases that are at least two days old.
- Dependabot updates actions and dependencies with a two-day cooldown.
- Prefer a maintained action or a `just` recipe over shell in `run:` steps. Keep shell for logic that is specific to Comanda.
- A new check goes into `pr-checks.yml` behind a paths filter and into the `needs` and `allowed-skips` lists of the `all-checks` job.
- `main.yml` promotes to production exactly the images its own run built and put on staging, after one approval on the `production` environment.
