# Load testing

k6 load tests that drive **one dedicated tenant** — `load-test.staging.comanda.leys.ch`
— to find where the staging node saturates and to surface slow paths in Sentry.

Traffic goes through the tenant's **public web host and its `/api` proxy**, not the
backend ingress directly. That is deliberate: the backend resolves the tenant from
`X-Forwarded-Host`, which Traefik overwrites on the public `api.staging…` host, so a
tenant can only be reached through its own web host. It also means a run stresses the
full real path — Next.js proxy → API → Postgres — exactly what a customer hits.

Everything is pinned to `load-test.staging.comanda.leys.ch`:

- The k6 scripts **refuse to run** against any other host.
- The seed/stock/cleanup SQL resolve the tenant from that host's verified
  `tenant_domain` row and scope every write to that tenant's `tenant_id`. No other
  tenant is ever seeded or cleaned.

> ⚠️ **Shared cluster.** Staging comanda + iglesio share the same K3s node. A `stress`
> run to saturation will degrade latency for neighbouring staging tenants **while it
> runs** (data stays isolated; the node's CPU/DB do not). Run stress off-hours and tell
> anyone using staging.

## What you need

- **k6** (`brew install k6`)
- A **privileged** psql connection to the staging DB (the table owner, so the seed can
  write across tenant tables with an explicit `tenant_id`). Open your tunnel and export
  it as `DATABASE_URL`.
- A **Cloudflare Access service token** — staging sits behind CF Access, so every HTTP
  request needs `CF-Access-Client-Id` / `CF-Access-Client-Secret` or it 302s to the CF
  login page. Export both before running (the DB seed goes through the tunnel and does
  **not** need them):

  ```bash
  export CF_ACCESS_CLIENT_ID='….access'
  export CF_ACCESS_CLIENT_SECRET='…'
  ```

  k6 reads them from the environment automatically (both the API and browser tests).
- The `load-test` tenant must already exist with its domain **verified** and **Payrexx
  not configured** (so TWINT falls through to the dev mark-paid path instead of hitting
  the real gateway). If the tenant has no active simple products, the seed creates a
  10-product test catalog with stock; if it already has a real catalog, it's left alone.

## Seed (one-time, via the privileged tunnel)

```bash
export DATABASE_URL='postgres://…privileged…@localhost:…/…'   # your tunnel

cd backend
just loadtest-seed         # users, sessions, POS + station devices — all in load-test
just loadtest-stock        # +50k inventory per simple product so a run can't run dry
```

The seed is idempotent and self-contained — it looks the tenant up by host, auto-provisions
two approved POS devices and one station device (with every active simple product assigned),
and aborts if no verified `load-test` domain exists. It creates:

- 20 customer users + sessions (`loadtest_customer_session_001…020`)
- 3 admin users + sessions + `member` rows (role `staff`)
- 2 POS device bindings (`loadtest_pos_token_001…002`) → 2 approved POS devices
- 3 STATION device bindings (`loadtest_station_token_001…003`) → 1 approved station
- (if the tenant is empty) a `Load Test` category + 10 simple products with stock

Tokens are the plaintext Bearer values in `tokens.json`; the seed stores their sha256
hashes (device tokens) and exact strings (sessions). Never use them outside loadtest.

## Run

Both scripts default `BASE_URL` / `WEB_URL` to the load-test tenant — no env needed.

```bash
cd backend

# API test — 1 VU/scenario, 30s. Proves the seed + proxy path end-to-end.
just loadtest smoke

# Dinner-rush shape (~12 min, ramps to ~10 req/s combined).
just loadtest baseline

# Find the knee — customer ramps 10→200 req/s in plateaus, station 2→20, admin steady.
just loadtest stress
```

`stress` is **not** aborted on threshold failure: the point is to see the full
saturation curve. The per-scenario thresholds (below) go red at the knee — that red
plateau is your answer for "when does it break."

Direct invocation, if you want raw k6 flags:

```bash
k6 run -e PROFILE=stress loadtest/k6.js
```

## Browser test — real Chromium + Core Web Vitals

`browser.js` drives the anonymous customer flow (`/` → add 2 products → `/checkout` →
click "Mit TWINT bezahlen") in real Chromium and captures LCP / FCP / INP / CLS / TTFB
per navigation. Each VU is one Chromium tab (~150–250 MB); `baseline` = 5 tabs, `stress`
ramps to 12.

```bash
just loadtest-browser smoke      # 1 VU, 3 iterations
just loadtest-browser baseline   # 5 tabs, 10 min

# API + browser together — the realistic combined picture. Writes JSON summaries
# to loadtest/results/ for later diffing.
just loadtest-full baseline
```

The storefront must be **open** (`system_enabled = true`; the seed sets it). If it shows
"Aktuell geschlossen" the browser test fails fast. Products with variants pop a picker
the script can't complete — keep the load-test catalog on simple products.

## Scenarios (`k6.js`)

| Scenario | Auth                 | Flow                                                   | Endpoints (under `/api`)                                       |
| -------- | -------------------- | ------------------------------------------------------ | -------------------------------------------------------------- |
| customer | session (Bearer)     | list products → create order → cash pay → poll status  | `/v1/products`, `/v1/orders`, `/v1/orders/{id}/payment`        |
| admin    | admin session        | parallel fetch (today's orders + products)             | `/v1/orders?date_from=…`, `/v1/products`                       |
| station  | POS + station device | POS creates+pays an order, station redeems it          | `/v1/orders`, `/v1/orders/{id}/payment`, `/v1/stations/redeem` |

Payment uses `cash` (not TWINT) to keep the Payrexx sandbox out of the latency picture.

## Reading the results

k6 prints a summary and fails the run if any threshold is breached:

- `http_req_failed` rate > 2%
- customer p95 > 1500 ms · station p95 > 1000 ms · admin p95 > 2000 ms

Use the per-scenario tags (`http_req_duration{scenario:customer}`) and the plateau where
error rate/p95 climb to identify the breaking point.

### Sentry analysis (the "find inefficiencies" half)

While a run is in flight and just after, check the `comanda-api` and `comanda-web` Sentry
projects (env tag `staging`):

- **Performance → slow transactions** during the run window — the endpoints with the
  worst p95 under load are your optimisation targets (N+1 queries, missing indexes,
  per-request work that should be cached).
- **Issues** spiking during the run — 500s, pool exhaustion, context-cancel storms,
  `backend-proxy-cold-start` (proxy couldn't reach the API = the API fell over).
- Correlate the k6 knee timestamp with the Sentry throughput/latency graphs.

## Cleanup

```bash
cd backend && just loadtest-cleanup
```

Removes all loadtest users, sessions, members, POS/station devices, inventory bumps, and
every order generated during runs (customer orders by contact email, POS/station orders
by their loadtest device) — scoped to the load-test tenant only.

## Tuning

- `profiles.stress.*.stages` in `k6.js` — raise the top `target` if the node shrugs off
  200 req/s, or add plateaus for finer resolution around the knee.
- `profiles.baseline.stages` — multiply durations to simulate a full evening.
- `tokens.json` + the `1..20` / `1..3` loops in `seed.sql` — grow the session pool if you
  saturate it at high VU counts.
