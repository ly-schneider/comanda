# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Code Style

- **No unnecessary comments.** Do not add comments to code unless they are truly essential and non-obvious. The code should speak for itself. Only add a comment when the logic is genuinely surprising or when required context cannot be expressed through naming alone.

## Repository Overview

This is a monorepo for the Bless2n Food System - a comprehensive food ordering platform with public ordering, admin dashboard, POS terminal, and IoT device integration. The system is designed for food stations with inventory tracking, payment processing (Payrexx/TWINT), and multi-platform support.

## Project Structure

```
bless2n-food-system/
├── bfs-backend/          # Go HTTP backend (Clean Architecture)
├── bfs-web-app/          # Next.js 15 web application (React 19)
├── bfs-android-app/      # Android mobile application
├── bfs-cloud/            # Azure Terraform infrastructure
├── bfs-docs/             # Documentation site (Fumadocs/Next.js)
└── docker-compose.yml    # Local development services
```

## Backend (Go)

**Location:** `bfs-backend/`

### Architecture

The backend follows **Clean Architecture** with clear layer separation:
- **Schema Layer**: Ent database schema definitions (`internal/schema/`)
- **Repository Layer**: Data access with PostgreSQL via pgx/Ent (`internal/repository/`)
- **Service Layer**: Business logic (`internal/service/`)
- **API Layer**: HTTP handlers (`internal/api/`)
- **HTTP Layer**: Chi router, middleware (`internal/http/`, `internal/middleware/`)
- **Auth Layer**: Better Auth JWKS validation, RBAC, device auth (`internal/auth/`)

Dependency injection is managed by Uber FX (`internal/app/`).

### Key Technologies
- Go 1.25+
- PostgreSQL with pgx and Ent ORM
- Chi router for HTTP routing
- Better Auth session validation (JWKS from Next.js app)
- Payrexx payment processing (TWINT for Switzerland)
- OIDC via Better Auth (Google OAuth handled in Next.js app)
- Atlas for database migrations
- Plunk for transactional emails
- Uber Zap logging
- OpenTelemetry for observability
- Air for live reload

### Development Commands

```bash
# Backend directory: bfs-backend/

# Setup environment
cp .env.example .env
# Edit .env if needed (defaults work for local Docker PostgreSQL)

# Start supporting services (PostgreSQL + Mailpit + Azurite)
just up

# Run database migrations
just migrate

# Seed development data (optional, idempotent)
just seed

# Run backend with live reload (recommended for development)
just dev

# Other common commands
just lint              # Run linters
just lint --fix        # Auto-fix linting issues
just fmt               # Format code
just tidy              # Tidy go.mod
just test              # Run unit tests
just test-integration  # Run integration tests (requires PostgreSQL)
just clean             # Clean build artifacts
```

### Service URLs (when running)
- Backend API: http://localhost:8080
- PostgreSQL: localhost:5432
- Mailpit (email testing): http://localhost:8025

### Testing Strategy
Tests are placed in the `test/` directory:
- `test/integration/` - Integration tests (requires PostgreSQL)

Run tests: `just test` or `go test -v -race ./internal/...`

## Web App (Next.js)

**Location:** `bfs-web-app/`

### Architecture
- Next.js 15 with App Router
- React 19 with Server Components
- TypeScript with strict mode
- Tailwind CSS 4 for styling
- Radix UI components
- React Hook Form + Zod validation
- Better Auth for authentication (sessions, Google OAuth)

### Key Features
- Public ordering interface (`app/(site)/`)
- Admin dashboard (`app/admin/`)
- POS terminal interface (`app/pos/`)
- Station/device interface (`app/station/`)
- Authentication routes (`app/(auth)/`, `app/api/auth/`)
- Device management (`app/admin/devices/`)

### Development Commands

```bash
# Web app directory: bfs-web-app/

# Install dependencies (uses pnpm)
pnpm install

# Run development server
pnpm dev
# Opens at http://127.0.0.1:3000

# Build for production
pnpm build

# Type checking
pnpm typecheck

# Linting
pnpm lint
pnpm lint:fix

# Formatting
pnpm prettier
pnpm prettier:fix

# Testing
pnpm test                # Vitest unit tests
pnpm test:watch          # Watch mode
pnpm test:coverage       # Coverage report
pnpm e2e:headless        # Playwright E2E tests
pnpm e2e:ui              # Playwright UI mode

# Storybook
pnpm storybook           # Start Storybook on port 6006
pnpm build-storybook     # Build static Storybook
pnpm test-storybook      # Test Storybook components

# Bundle analysis
pnpm analyze
```

### Testing
- **Unit Tests**: Vitest with React Testing Library
- **E2E Tests**: Playwright
- **Component Tests**: Storybook + test-runner

## Cloud Infrastructure (Terraform)

**Location:** `bfs-cloud/`

### Architecture
Azure-based infrastructure with environment isolation:
- **Container Apps**: Auto-scaling web applications (0-20 replicas)
- **PostgreSQL**: NeonDB for data persistence
- **Container Registry**: GHCR for image storage
- **Key Vault**: Secret management
- **VNets**: Network isolation per environment
- **Log Analytics**: Observability and diagnostics

### Deployment

```bash
# Deploy staging
cd bfs-cloud/envs/staging
terraform init
terraform apply

# Deploy production
cd bfs-cloud/envs/production
terraform init
terraform apply
```

### Key Features
- **Scale-to-Zero**: Apps scale down to 0 when idle
- **Auto-Scaling Rules**: HTTP requests, CPU, memory-based
- **Environment Isolation**: Separate VNets, resource groups
- **Three-Phase Deployment**: ACR -> Image Build -> Container Apps

### CI/CD Integration
GitHub Actions uses a **semantic-release, immutable tag release pipeline** with reusable workflows and composite actions. Versioning is driven by [Conventional Commits](https://www.conventionalcommits.org/).

**Composite Actions** (`.github/actions/`):
- `setup-node-pnpm` — Node 22 + pnpm 9 + `pnpm install --frozen-lockfile`
- `setup-go-env` — Go 1.25 with module cache
- `generate-backend` — Ent + OpenAPI code generation
- `docker-build-push` — BuildX + GHCR login + build & push with registry caching

**CI** (`ci.yml` — runs on PRs and pushes to `main`):
- Detects changed projects via path filtering (`dorny/paths-filter`)
- Delegates to reusable workflows: `ci-web.yml`, `ci-backend.yml`
- Inline jobs for docs (lint + build) and Terraform (fmt + validate)
- `ci-gate` (named `CI Success`) is the single branch-protection required check
- Does **not** trigger deploys directly — `deploy.yml` listens for successful CI runs via `workflow_run`

**Deploy Staging** (`deploy.yml` — triggered by successful CI on `main`, or manual dispatch with `force=true`):
- Runs `semantic-release` first (config in `.releaserc.json`): analyses commits since last tag, creates `vX.Y.Z` git tag + GitHub release
- Only builds/deploys if semantic-release produced a new version (or `force=true` was passed)
- Builds Docker images via `docker-build-push` composite with `X.Y.Z` + `staging` tags
- Runs Atlas migrations, deploys via Terraform Cloud, creates Sentry staging release

**Deploy Prod** (`deploy-prod.yml` — manual `workflow_dispatch` only):
- Input: `version` (e.g. `v1.3.2`)
- Validates the image exists in GHCR, re-tags it as `production` via `crane tag` (no rebuild, same digest)
- Runs Atlas migrations against prod, deploys via Terraform Cloud prod workspace, creates Sentry prod release
- Rollback = dispatch with a previous `vX.Y.Z` tag

**Key Features:**
- Conventional Commits → automatic semver (no manual VERSION file)
- Immutable version tags: image tag = git tag (no environment suffixes)
- Production guarantees byte-for-byte same image as staging
- Intelligent path filtering to skip unchanged components
- Advanced Docker layer caching via registry cache
- PRs only run CI (no builds) to save resources

## Docker Compose Development

The backend includes a `docker-compose.yml` for local development:

```bash
# Start default services (PostgreSQL + Mailpit + Azurite)
just up

# Start with dev extras (+ pgAdmin)
just up --profile dev

# Stop services
just down

# Stop and remove volumes (data loss!)
just down --volumes
```

**Services:**
- `postgres`: PostgreSQL 18 (port 5432)
- `azurite`: Azure Blob Storage emulator (port 10000)
- `mailpit`: Email testing UI (port 8025 UI, port 1025 SMTP)

## Authentication & Authorization

### Better Auth (Next.js App)
Authentication is handled by Better Auth in the Next.js web app:
- Session-based authentication with secure cookies
- Google OAuth via Better Auth social providers
- Email/password authentication
- JWKS endpoint for backend token verification

### Backend Auth Validation
The Go backend validates requests using:
- JWKS discovery from the Better Auth URL (`BETTER_AUTH_URL/api/auth/jwks`)
- Session middleware that verifies Bearer tokens against JWKS
- Role-Based Access Control (RBAC) middleware (`internal/auth/rbac.go`)
- Device authentication for IoT/station devices (`internal/auth/device_middleware.go`)

### Role-Based Access Control
- Admin role: Full system access
- Customer role: Limited to ordering and account management

## Payment Processing

### Payrexx Integration
- Payrexx API for payment gateway creation
- TWINT payment method (Switzerland-specific)
- Currency: CHF
- Webhook handling for payment status updates
- HMAC signature verification for API requests and webhooks

### Configuration
- `PAYREXX_INSTANCE`: Payrexx instance name
- `PAYREXX_API_SECRET`: API secret for request signing
- `PAYREXX_WEBHOOK_SECRET`: Webhook verification secret

## Branch Strategy

- **Main branch**: `main` (trunk-based development)
- Feature branches are created from `main` and merged back via PR
- **Conventional Commits** drive versioning: `feat:` → minor, `fix:` → patch, `feat!:` / `BREAKING CHANGE:` → major
- Every push to `main` runs CI; on success `semantic-release` tags the next version and staging auto-deploys
- Production releases are triggered by manually dispatching the `Deploy Prod` workflow with a `vX.Y.Z` tag
- One version everywhere — git tag, image tag, Terraform var, `APP_VERSION` env (no VERSION file)
- Production promotes the exact staging image (no rebuild, same digest)
- Rollback: dispatch `Deploy Prod` with a previous `vX.Y.Z` tag

## Common Development Workflow

1. **Backend changes:**
   ```bash
   cd bfs-backend
   just up              # Start PostgreSQL + Mailpit + Azurite
   just migrate         # Run database migrations
   just dev             # Run backend with live reload
   # Make changes, Air auto-reloads
   just test            # Run unit tests
   ```

2. **Web app changes:**
   ```bash
   cd bfs-web-app
   pnpm dev          # Start Next.js dev server
   # Make changes, hot reload active
   pnpm test         # Run tests
   pnpm typecheck    # Verify types
   ```

3. **Full stack testing:**
   ```bash
   # Terminal 1: Start backend
   cd bfs-backend && just up && just migrate && just dev

   # Terminal 2: Start web app
   cd bfs-web-app && pnpm dev

   # Access frontend at http://localhost:3000
   # Access backend at http://localhost:8080
   ```

4. **Infrastructure changes:**
   ```bash
   cd bfs-cloud/envs/staging
   terraform plan    # Review changes
   terraform apply   # Apply changes
   ```

## Important Notes

- **Environment Variables**: Always copy `.env.example` to `.env` and configure
- **PostgreSQL**: Backend requires PostgreSQL running (via `just up`)
- **Migrations**: Run `just migrate` after starting PostgreSQL for the first time or after pulling new migration files
- **Package Manager**: Web app uses pnpm (specified in `package.json`)
- **Node Version**: Requires Node.js 20.0.0+
- **Go Version**: Requires Go 1.25+
- **Better Auth**: Authentication is managed in the Next.js app; the backend only validates tokens via JWKS

## Configuration Management

### Backend (.env)
Key variables in `bfs-backend/.env`:
- `DATABASE_URL`: PostgreSQL connection string
- `BETTER_AUTH_URL`: Next.js app URL for JWKS discovery
- `PAYREXX_INSTANCE` / `PAYREXX_API_SECRET` / `PAYREXX_WEBHOOK_SECRET`: Payment processing
- `PUBLIC_BASE_URL`: Frontend URL (for redirects)
- `PLUNK_API_KEY`: Transactional email service
- `SECURITY_TRUSTED_ORIGINS`: CORS origins

### Web App (.env.local)
Key variables in `bfs-web-app/.env.local`:
- `NEXT_PUBLIC_API_BASE_URL`: Backend API URL (client-side)
- `BACKEND_INTERNAL_URL`: Backend URL (server-side)
- `BETTER_AUTH_SECRET`: Session signing secret
- `DATABASE_URL`: PostgreSQL connection for Better Auth
- `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET`: OAuth (managed by Better Auth)

## Monitoring & Observability

### Local Development
- **Mailpit**: View sent emails at http://localhost:8025
- **Structured Logging**: Uber Zap in backend (JSON format)
- **Jaeger UI**: Distributed tracing at http://localhost:16686 (requires `--profile observability`)

### Production (Azure)
- Log Analytics workspace for both environments
- Diagnostic settings on all resources
- Metric-based alerts (configurable per environment)
- Application Insights integration via OpenTelemetry
