// k6 API load test, driven through the tenant's /api proxy (see README).
// Env: BASE_URL (defaults to the load-test tenant), PROFILE = smoke | baseline | stress
// Requires the tenant seeded via seed.sql and Payrexx unconfigured.

import http from "k6/http";
import { check, group, sleep } from "k6";
import {
  randomIntBetween,
  uuidv4,
} from "https://jslib.k6.io/k6-utils/1.4.0/index.js";

// Hard-pinned so a run can never hit another tenant on the shared platform.
const TARGET_HOST = "load-test.staging.comanda.leys.ch";

const BASE_URL = (__ENV.BASE_URL || `https://${TARGET_HOST}`).replace(/\/$/, "");
const HOST = BASE_URL.replace(/^https?:\/\//, "")
  .split("/")[0]
  .toLowerCase();
if (HOST !== TARGET_HOST) {
  throw new Error(
    `Refusing to run: BASE_URL host '${HOST}' is not the dedicated load-test tenant '${TARGET_HOST}'.`,
  );
}

// Tenant API traffic goes through the web app's /api proxy, which forwards to
// the backend with the tenant's X-Forwarded-Host. Bearer auth skips CSRF there.
const API = `${BASE_URL}/api`;

// Staging is behind Cloudflare Access; the service token bypasses the login.
const cfHeaders =
  __ENV.CF_ACCESS_CLIENT_ID && __ENV.CF_ACCESS_CLIENT_SECRET
    ? {
        "CF-Access-Client-Id": __ENV.CF_ACCESS_CLIENT_ID,
        "CF-Access-Client-Secret": __ENV.CF_ACCESS_CLIENT_SECRET,
      }
    : {};

const PROFILE = __ENV.PROFILE || "smoke";

const tokens = JSON.parse(open("./tokens.json"));

// `baseline` mirrors the dinner-rush shape compressed to ~12 minutes.
const profiles = {
  smoke: {
    customer: {
      executor: "constant-vus",
      exec: "customer",
      vus: 1,
      duration: "30s",
      tags: { scenario: "customer" },
    },
    admin: {
      executor: "constant-vus",
      exec: "admin",
      vus: 1,
      duration: "30s",
      tags: { scenario: "admin" },
    },
    station: {
      executor: "constant-vus",
      exec: "station",
      vus: 1,
      duration: "30s",
      tags: { scenario: "station" },
    },
  },

  baseline: {
    customer: {
      executor: "ramping-arrival-rate",
      exec: "customer",
      startRate: 1,
      timeUnit: "1s",
      preAllocatedVUs: 30,
      maxVUs: 80,
      stages: [
        { target: 2, duration: "1m" }, // warmup — avoids measuring cold-start
        { target: 7, duration: "2m" },
        { target: 7, duration: "5m" }, // sustained dinner peak
        { target: 3, duration: "2m" },
        { target: 0, duration: "1m" },
      ],
      tags: { scenario: "customer" },
    },
    admin: {
      executor: "constant-arrival-rate",
      exec: "admin",
      rate: 1,
      timeUnit: "10s",
      preAllocatedVUs: 2,
      maxVUs: 5,
      duration: "11m",
      tags: { scenario: "admin" },
    },
    station: {
      executor: "ramping-arrival-rate",
      exec: "station",
      startRate: 1,
      timeUnit: "1s",
      preAllocatedVUs: 10,
      maxVUs: 30,
      stages: [
        { target: 1, duration: "1m" },
        { target: 3, duration: "2m" },
        { target: 3, duration: "5m" },
        { target: 1, duration: "2m" },
        { target: 0, duration: "1m" },
      ],
      tags: { scenario: "station" },
    },
  },

  // Find the knee: arrival-rate plateaus climb until error rate / p95 blow up.
  // Not aborted on threshold breach, so the full saturation curve is captured.
  stress: {
    customer: {
      executor: "ramping-arrival-rate",
      exec: "customer",
      startRate: 5,
      timeUnit: "1s",
      preAllocatedVUs: 80,
      maxVUs: 500,
      stages: [
        { target: 10, duration: "1m" }, // warmup
        { target: 25, duration: "2m" },
        { target: 50, duration: "2m" },
        { target: 100, duration: "2m" },
        { target: 150, duration: "2m" },
        { target: 200, duration: "2m" }, // push past the knee
        { target: 0, duration: "1m" },
      ],
      tags: { scenario: "customer" },
    },
    station: {
      executor: "ramping-arrival-rate",
      exec: "station",
      startRate: 1,
      timeUnit: "1s",
      preAllocatedVUs: 20,
      maxVUs: 120,
      stages: [
        { target: 2, duration: "1m" },
        { target: 5, duration: "2m" },
        { target: 10, duration: "2m" },
        { target: 15, duration: "2m" },
        { target: 15, duration: "2m" },
        { target: 20, duration: "2m" },
        { target: 0, duration: "1m" },
      ],
      tags: { scenario: "station" },
    },
    admin: {
      executor: "constant-arrival-rate",
      exec: "admin",
      rate: 1,
      timeUnit: "3s",
      preAllocatedVUs: 3,
      maxVUs: 10,
      duration: "12m",
      tags: { scenario: "admin" },
    },
  },
};

if (!profiles[PROFILE]) {
  throw new Error(
    `Unknown PROFILE '${PROFILE}'. Valid: ${Object.keys(profiles).join(", ")}`,
  );
}

export const options = {
  scenarios: profiles[PROFILE],
  thresholds: {
    http_req_failed: ["rate<0.02"],
    "http_req_duration{scenario:customer}": ["p(95)<1500"],
    "http_req_duration{scenario:station}": ["p(95)<1000"],
    "http_req_duration{scenario:admin}": ["p(95)<2000"],
  },
  maxRedirects: 0,
};

export function setup() {
  const res = http.get(`${API}/v1/products`, { headers: cfHeaders });
  if (res.status !== 200) {
    throw new Error(
      `setup: GET /v1/products returned ${res.status} (${res.body && res.body.slice(0, 200)})`,
    );
  }
  const body = res.json();
  const simple = (body.items || []).filter(
    (p) => p.type === "simple" && p.priceCents > 0,
  );
  if (simple.length === 0) {
    throw new Error(
      "setup: no `simple` products in catalog — seed staging with products first",
    );
  }
  console.log(`setup: ${simple.length} simple products available for ordering`);
  return { products: simple.map((p) => ({ id: p.id, price: p.priceCents })) };
}

function pick(arr) {
  return arr[Math.floor(Math.random() * arr.length)];
}

function bearer(token) {
  return {
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
      ...cfHeaders,
    },
  };
}

function postJSON(url, token, body, extraHeaders, name) {
  const opts = bearer(token);
  if (extraHeaders) Object.assign(opts.headers, extraHeaders);
  // Name tag groups per-order URLs; without it each order id is its own series.
  if (name) opts.tags = { name };
  return http.post(url, JSON.stringify(body), opts);
}

const PAYMENT_TAG = "v1/orders/{id}/payment";

function buildCartItems(products) {
  const n = randomIntBetween(1, 3);
  const picked = [];
  for (let i = 0; i < n; i++) {
    picked.push({
      productId: pick(products).id,
      quantity: randomIntBetween(1, 2),
    });
  }
  return picked;
}

export function customer(data) {
  const sessionToken = pick(tokens.customer_sessions);

  group("customer/list-products", () => {
    const r = http.get(`${API}/v1/products`, bearer(sessionToken));
    check(r, { "products 200": (res) => res.status === 200 });
  });

  let orderId;
  group("customer/create-order", () => {
    const r = postJSON(
      `${API}/v1/orders`,
      sessionToken,
      {
        items: buildCartItems(data.products),
        contactEmail: `loadtest+vu${__VU}@loadtest.local`,
      },
      { "Idempotency-Key": uuidv4() },
    );
    const ok = check(r, {
      "create 2xx": (res) => res.status === 200 || res.status === 201,
    });
    if (ok) orderId = r.json("id");
  });

  if (!orderId) return;

  // Cash instead of TWINT — avoids Payrexx sandbox flakiness masking backend perf.
  group("customer/pay-cash", () => {
    const r = postJSON(
      `${API}/v1/orders/${orderId}/payment`,
      sessionToken,
      { method: "cash" },
      { "Idempotency-Key": uuidv4() },
      PAYMENT_TAG,
    );
    check(r, { "pay 2xx": (res) => res.status === 200 || res.status === 201 });
  });

  group("customer/poll-payment", () => {
    const opts = bearer(sessionToken);
    opts.tags = { name: PAYMENT_TAG };
    const r = http.get(`${API}/v1/orders/${orderId}/payment`, opts);
    check(r, { "poll 2xx": (res) => res.status === 200 });
  });

  sleep(randomIntBetween(1, 3));
}

export function admin(data) {
  const sessionToken = pick(tokens.admin_sessions);
  const today = new Date();
  const from = new Date(today);
  from.setHours(0, 0, 0, 0);
  const to = new Date(today);
  to.setHours(24, 0, 0, 0);
  const qs = `date_from=${encodeURIComponent(from.toISOString())}&date_to=${encodeURIComponent(to.toISOString())}`;

  // Mirrors the parallel fetch in web/app/admin/page.tsx.
  group("admin/dashboard-fetch", () => {
    const responses = http.batch([
      ["GET", `${API}/v1/orders?${qs}`, null, bearer(sessionToken)],
      ["GET", `${API}/v1/products`, null, bearer(sessionToken)],
    ]);
    check(responses[0], { "orders 200": (r) => r.status === 200 });
    check(responses[1], { "products 200": (r) => r.status === 200 });
  });

  sleep(randomIntBetween(3, 8));
}

export function station(data) {
  const posToken = pick(tokens.pos_tokens);
  const stationToken = pick(tokens.station_tokens);

  let orderId;
  group("station/pos-create-order", () => {
    const r = postJSON(
      `${API}/v1/orders`,
      posToken,
      { items: buildCartItems(data.products) },
      { "Idempotency-Key": uuidv4() },
    );
    if (
      check(r, {
        "pos create 2xx": (res) => res.status === 200 || res.status === 201,
      })
    ) {
      orderId = r.json("id");
    }
  });

  if (!orderId) return;

  group("station/pos-pay-cash", () => {
    const r = postJSON(
      `${API}/v1/orders/${orderId}/payment`,
      posToken,
      { method: "cash" },
      { "Idempotency-Key": uuidv4() },
      PAYMENT_TAG,
    );
    check(r, {
      "pos pay 2xx": (res) => res.status === 200 || res.status === 201,
    });
  });

  group("station/redeem", () => {
    const opts = bearer(stationToken);
    opts.headers["Idempotency-Key"] = uuidv4();
    // Redeem is rate-limited (5 req/10s per station), so 429 is expected here.
    opts.responseCallback = http.expectedStatuses(200, 201, 429);
    const r = http.post(
      `${API}/v1/stations/redeem`,
      JSON.stringify({ orderId }),
      opts,
    );
    check(r, {
      "redeem ok or rate-limited": (res) =>
        res.status === 200 || res.status === 201 || res.status === 429,
    });
  });
}
