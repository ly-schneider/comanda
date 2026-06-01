# Grafana Cloud — Internal Notes (Leys Services)

Gitignored. Do not commit.

## Environment

| Thing                  | Value                                                  |
| ---------------------- | ------------------------------------------------------ |
| Grafana stack URL      | `https://leysservices.grafana.net`                     |
| Terraform Cloud org    | `leys-services`                                        |
| Grafana workspace      | `bfs-grafana`                                          |
| Env workspaces         | `bfs-staging`, `bfs-production`                        |
| Azure tenant ID        | `900be068-d990-4a2f-82f0-1a2faf493ab0`                 |
| Azure subscription ID  | `a6e97570-b80b-4a4d-a27c-081fe6c7e7f3`                 |
| Staging resource group | `bfs-staging-rg`                                       |
| Production RG          | `bfs-production-rg`                                    |
| SP display name        | `sp-grafana-cloud-reader` (already created, rbac only) |
| Sentry org slug        | `leys`                                                 |
| Personal Azure login   | `pos@gpmc.ch` (limited rights — no AAD, no sub RBAC)   |

## Where the secrets live

All five tokens/secrets are stored as Terraform Variables in the `bfs-grafana` TF Cloud workspace (Variables tab, category **Terraform Variable**):

- `grafana_cloud_token` ✅ sensitive — stack service account token, regenerate from `leysservices.grafana.net → Administration → Service accounts → terraform-admin`
- `grafana_azure_client_id` — `appId` of `sp-grafana-cloud-reader`
- `grafana_azure_client_secret` ✅ sensitive — rotate via `az ad app credential reset`
- `azure_tenant_id` — same as above tenant
- `azure_subscription_id` — same as above sub
- `sentry_auth_token` ✅ sensitive — from Sentry → Developer Settings → `grafana-cloud-reader` internal integration

## Ready-to-paste commands

Subscription:

```bash
export SUB=a6e97570-b80b-4a4d-a27c-081fe6c7e7f3
```

SP rotate (when the 1y secret expires):

```bash
az ad app credential reset \
  --id $(az ad app list --display-name sp-grafana-cloud-reader --query '[0].appId' -o tsv) \
  --years 1
# → paste new password into TF Cloud var `grafana_azure_client_secret`, queue apply
```

Sanity-check the SP's role assignments:

```bash
OBJECT_ID=$(az ad sp list --display-name sp-grafana-cloud-reader --query '[0].id' -o tsv)
az role assignment list --assignee "$OBJECT_ID" --all -o table
```

## Known issues carried forward

- The dashboard's latency + status-code panels parse `latency_ms` / `status` fields from `ContainerAppConsoleLogs_CL`. The backend (`bfs-backend/internal/app/server.go`) doesn't emit structured request logs yet — those panels stay empty until a Zap HTTP middleware is added.
- Since `pos@gpmc.ch` has no AAD / no subscription-level RBAC rights, any future SP creation or subscription-scoped role changes need to be done by whoever has `Owner` on the sub (the person who set up `bfs-staging` originally).

## Quick links

- Grafana dashboards: https://leysservices.grafana.net/dashboards
- TF Cloud workspace: https://app.terraform.io/app/leys-services/workspaces/bfs-grafana
- Azure portal (resource groups): https://portal.azure.com/#@900be068-d990-4a2f-82f0-1a2faf493ab0/resource/subscriptions/a6e97570-b80b-4a4d-a27c-081fe6c7e7f3/resourceGroups
- Sentry org: https://leys.sentry.io
