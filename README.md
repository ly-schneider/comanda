# Comanda

A full-stack food ordering and event management platform. Handles the complete lifecycle from customer ordering and payment processing to kitchen fulfillment, inventory tracking, and POS terminal operations.

## Architecture

```
                    ┌──────────────────────────────────────┐
                    │      Hetzner K3s on Arm64 SSD        │
                    │    (gitops/ArgoCD, scale-to-zero)    │
                    └──────────┬──────────────┬────────────┘
                               │              │
                    ┌──────────▼───┐    ┌─────▼────────────┐
                    │  Next.js 16  │    │    Go Backend    │
                    │  React 19    │    │    Chi Router    │
                    │  TypeScript  │◄──►│    Clean Arch    │
                    │  Tailwind 4  │    │    Uber FX DI    │
                    └──────┬───────┘    └──────┬───────────┘
                           │                   │
             ┌─────────────┼───────────────────┼───────────┐
             │             │                   │           │
      ┌──────▼─────────┐ ┌─▼──────┐  ┌─────────▼───┐  ┌────▼────────┐
      │ Better Auth    │ │ Sentry │  │ PostgreSQL  │  │   Payrexx   │
      │ (Sessions,     │ │        │  │  (Scaleway) │  │ (TWINT/more)│
      │  Google OAuth) │ │        │  │             │  │             │
      └────────────────┘ └────────┘  └─────────────┘  └─────────────┘
```

**Four distinct interfaces** serve different user roles:

| Interface | Path | Purpose |
|-----------|------|---------|
| Public Ordering | `/` | Customer-facing menu, cart, QR code ordering, TWINT checkout |
| Admin Dashboard | `/admin` | Order management, inventory, analytics, device configuration |
| POS Terminal | `/pos` | Cashier interface for on-site payments via SumUp |
| Station Display | `/station` | Kitchen/preparation queue for food station operators |

## Tech Stack

| Component | Stack |
|-----------|-------|
| **Backend** | Go 1.25, Chi, PostgreSQL, Ent ORM, Atlas Migrations, Uber FX, RLS |
| **Web App** | Next.js 16, React 19, TypeScript, Tailwind CSS 4, Radix UI |
| **Android** | Kotlin, Jetpack Compose, SumUp SDK, ZXing, Thermal Printer |
| **Infrastructure** | Hetzner K3s, ArgoCD gitops, Scaleway DB, Traefik, cert-manager |
| **Auth** | Better Auth (sessions + Google OAuth), JWKS validation, RBAC |
| **Payments** | Payrexx (TWINT + more), SumUp (card terminal) |
| **CI/CD** | GitHub Actions → GHCR, trunk-based development, immutable SHA tags |
| **Observability** | Sentry, OpenTelemetry, Uber Zap, Doppler secrets |

## Repository Structure

```
comanda-backend/      Go HTTP API — Clean Architecture, Payrexx payments, RLS
comanda-web/          Next.js web app — ordering, admin, POS, station interfaces
comanda-android/      Android POS terminal — SumUp payments, thermal printing
comanda-docs/         Documentation site — Fumadocs, admin handbook, dev guide
```

## Quick Start

### Prerequisites

- Go 1.25+
- Node.js 20+ with pnpm
- Docker & Docker Compose
- [Just](https://just.systems/) task runner

### Full Stack Development

```bash
# Terminal 1 — Backend
cd comanda-backend
cp .env.example .env
just up
just migrate
just seed
just dev              # API at http://localhost:8080

# Terminal 2 — Web App
cd comanda-web
pnpm install
pnpm dev              # App at http://localhost:3000
```

## Deployment

The project uses a **semantic-release, immutable-SHA release pipeline** driven by [Conventional Commits](https://www.conventionalcommits.org/):

1. **Push to `main`** runs CI. On success, `semantic-release` analyses commits since the last tag, computes the next semver, creates a git tag (`vX.Y.Z`) + GitHub release, then images are built and pushed to GHCR with immutable SHA tags and semver tags
2. **ArgoCD** watches the gitops repo (`hetzner`) and syncs image references to K3s (deployed in P5)
3. **Rollback** = update the gitops repo to reference a previous image SHA — images are immutable in GHCR

Versions come from commit messages: `feat:` → minor, `fix:` → patch, `feat!:` / `BREAKING CHANGE:` → major. Commits that don't match produce no release.

### Environments

| Environment | Deployment | Scaling |
|-------------|------------|---------|
| **Staging** | On semver tag (automatic) | K3s managed, idle → minimal |
| **Production** | Manual gitops commit (promote SHA tag) | K3s managed, idle → minimal |

Both environments run on **Hetzner K3s with scale-to-zero**.

## Key Engineering Decisions

- **Better Auth over custom auth**: Session-based authentication managed in Next.js, backend validates via JWKS — clean separation of concerns
- **Payrexx for Swiss payments**: Native TWINT support with HMAC-verified webhooks
- **Atlas + Ent for migrations**: Declarative schema in Go, versioned SQL migrations, CI linting
- **Immutable releases**: Production always references the exact same image SHA that ran in staging
- **Clean Architecture in Go**: Strict layer separation with Uber FX dependency injection
- **RLS for tenant isolation**: PostgreSQL Row-Level Security as the security boundary (per P1 design)
- **K3s + ArgoCD on Hetzner**: Cost-effective, gitops-driven, scale-to-zero via K3s cluster autoscaling

## Project Documentation

Detailed READMEs are available in each sub-project:

- [`comanda-backend/README.md`](comanda-backend/README.md) — API architecture, development setup, testing
- [`comanda-web/README.md`](comanda-web/README.md) — Frontend architecture, routes, component system
- [`comanda-android/README.md`](comanda-android/README.md) — Android POS, build variants, hardware integration
- [`comanda-docs/README.md`](comanda-docs/README.md) — Documentation site setup and content structure
## License

This project is source-available. All rights reserved. See [LICENSE](LICENSE) for details.
