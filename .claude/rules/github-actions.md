---
paths:
  - ".github/**"
---

# GitHub Actions

- Pin every action to a full commit SHA with the version in a comment, for example `actions/checkout@<sha> # v7.0.1`. Use only releases that are at least two days old.
- Dependabot updates actions and dependencies with a two-day cooldown.
- A new check goes into `pr-checks.yml` behind a paths filter and into the `needs` list of the `all-checks` job.
- `main.yml` must stay safe to approve at any time: the production job promotes everything staging runs.
