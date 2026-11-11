# Comanda

A multi-tenant ordering, payment and point-of-sale platform for food stands and events. It covers the full flow from the customer ordering on their phone, through payment and kitchen pickup, to inventory, refunds and the sales export for the books. Each organisation runs as its own tenant with its own branding, domain, payment accounts and staff.

## Architecture

```
            customers · staff · devices
                         │
              ┌──────────▼──────────┐
              │ Traefik + cert-mgr  │  tenant subdomains + BYO custom domains
              └────┬───────────┬────┘
                   │           │
        ┌──────────▼───┐   ┌───▼─────────────┐      ┌──────────────────┐
        │  Next.js 16  │──►│   Go backend    │◄─────│   Android POS    │
        │  React 19    │   │   Chi · Ent     │      │  Kotlin/Compose  │
        │  Better Auth │   │   Uber FX       │      │  SumUp · printer │
        └──────────────┘   └───┬─────────┬───┘      └──────────────────┘
                               │         │
                   ┌───────────▼──┐   ┌──▼─────────────────────────────┐
                   │  PostgreSQL  │   │ Payrexx · SumUp · Plunk · R2   │
                   │  (RLS, per   │   │ (per-tenant credentials,       │
                   │   tenant)    │   │  AES-256-GCM encrypted)        │
                   └──────────────┘   └────────────────────────────────┘
```

### Multi-tenancy

- **Host-based tenant resolution.** The backend maps the request host (a platform subdomain or a verified custom domain) to a tenant. The web app forwards the host on every server-side call.
- **Row-Level Security is the isolation boundary.** Every tenant table carries `tenant_id` and an RLS policy. Request traffic runs as an RLS-scoped database role inside one transaction per request with `app.tenant_id` set; migrations run as a separate privileged role. Policy-coverage and cross-tenant tests enforce this.
- **Bring your own everything.** Each tenant configures its own branding (logo, colours, font), Payrexx account, SumUp merchant, email sender and custom domains. Payment secrets are stored encrypted (AES-256-GCM), and Payrexx webhooks are routed per tenant.
- **Custom domains.** A tenant adds a domain, points a CNAME at the platform, and the backend verifies it and provisions the Ingress and TLS certificate (HTTP-01) automatically.

## Interfaces

| Interface       | Path              | Purpose                                                                 |
| --------------- | ----------------- | ----------------------------------------------------------------------- |
| Storefront      | `/`               | Customer menu, cart, checkout (TWINT via Payrexx), orders and pickup QR |
| Admin           | `/admin`          | Products, bundles, categories, inventory, orders, refunds, sales export |
| POS             | `/pos`            | Cashier interface for cash, card (SumUp) and TWINT payments             |
| Station         | `/station`        | Pickup station: scans pickup QR codes and hands out orders              |
| Account hub     | `/account`, `/t/` | Tenant owners: branding, domains, Payrexx, SumUp, invites               |
| Platform admin  | `/platform-admin` | Tenant provisioning and QA                                              |

The Android app is a native POS for tablets with a SumUp card reader and a thermal receipt printer. It can be built as a per-tenant branded APK so several tenants can share one device.

### Plugins

Optional features ship in-tree as plugins that are enabled per tenant (disabled plugins return 404):

- **staff-perks**: free meals and drinks for staff and volunteers
- **artist-voucher**: offline comp vouchers for artists and single-use outreach vouchers
- **club100**: member benefits synced from an external membership system
- **stationqr**: enroll pickup stations by scanning a QR code

## Tech Stack

| Component          | Stack                                                                          |
| ------------------ | ------------------------------------------------------------------------------ |
| **Backend**        | Go 1.26, Chi, Ent, Atlas migrations, Uber FX, PostgreSQL with RLS, oapi-codegen |
| **Web App**        | Next.js 16, React 19, TypeScript, Tailwind CSS 4, Radix UI, Better Auth        |
| **Android**        | Kotlin, Jetpack Compose, SumUp SDK, ZXing, ESC/POS thermal printing            |
| **Auth**           | Better Auth with organizations (email OTP + Google OAuth), JWKS validation      |
| **Payments**       | Payrexx (TWINT and more), SumUp (card terminal), cash, deposits and refunds    |
| **Storage & mail** | S3-compatible object storage (Cloudflare R2, MinIO locally), Plunk             |
| **Infrastructure** | Hetzner K3s, ArgoCD gitops, Scaleway PostgreSQL + PgBouncer, Traefik, Doppler  |
| **Observability**  | Sentry, OpenTelemetry, Zap                                                     |

## Repository Structure

```
backend/      Go HTTP API: tenancy + RLS, orders, payments, inventory, plugins
web/          Next.js app: storefront, admin, POS, station, account hub
android/      Android POS: SumUp payments, thermal printing, per-tenant builds
docs/         Documentation site (Fumadocs): admin handbook, developer guide
```

## Quick Start

### Prerequisites

- Go 1.26+
- Node.js 22+ with pnpm
- Docker with Docker Compose
- [just](https://just.systems/)

### Run locally

```bash
# Terminal 1: backend
cd backend
cp .env.example .env
just up                                 # Postgres, MinIO, Mailpit
just bootstrap-local you@example.com    # migrations, demo catalog, admin user
just dev                                # API on http://localhost:8080

# Terminal 2: web app
cd web
pnpm install
pnpm dev                                # app on http://localhost:3000
```

Sign in at http://localhost:3000/login with the email you bootstrapped. The one-time code lands in Mailpit at http://localhost:8025.

### Tests

```bash
cd backend && just test               # unit tests
cd backend && just test-integration   # RLS + integration tests (needs Postgres)
cd web && pnpm typecheck && pnpm lint && pnpm test
cd web && pnpm e2e:headless           # Playwright
```

## Releases and Deployment

Releases are driven by [Conventional Commits](https://www.conventionalcommits.org/):

1. A push to `main` runs CI. On success, `semantic-release` computes the next version from the commits since the last tag (`feat` → minor, `fix` → patch, `feat!` → major) and creates the tag and GitHub release.
2. Images are built and pushed to GHCR, tagged with the commit SHA and the version.
3. Staging deploys automatically. Production is promoted by pinning the same image SHA in the gitops repository, which ArgoCD syncs to the cluster. A rollback means pinning an earlier SHA.

Android builds are released separately through GitHub Releases, and the app updates itself from there.

## Documentation

- [`backend/README.md`](backend/README.md): API architecture, development setup, testing
- [`web/README.md`](web/README.md): frontend architecture, routes, components
- [`android/README.md`](android/README.md): Android POS, build variants, hardware integration
- [`docs/README.md`](docs/README.md): documentation site

## Contributing and Security

See [CONTRIBUTING.md](CONTRIBUTING.md). Please report vulnerabilities privately as described in [SECURITY.md](SECURITY.md).

## License

Comanda is source-available under the [Business Source License 1.1](LICENSE), not an OSI open source license.

- **Free** for individuals, non-profits, schools, and organizations with fewer than 50 people and less than USD 5M annual revenue, including self-hosting it for your own organization and events in production.
- **Not allowed** without a commercial license: offering Comanda to others as a hosted or managed service, or embedding it in a product you sell.
- **Larger organizations** need a commercial license for production use. Contact levyn.schneider@leys.ch.
- Each version converts to the GNU AGPL v3.0 or later four years after it is published.

Reading, modifying and running Comanda for development, testing and evaluation is always allowed.
