package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"backend/internal/generated/api/generated"
	"backend/internal/plugin"
	"backend/internal/response"
)

// ListPlugins returns the plugins enabled for the host-resolved tenant with
// their non-secret config. Public, same trust level as branding.
// (GET /plugins)
func (h *Handlers) ListPlugins(w http.ResponseWriter, r *http.Request) {
	items, err := h.plugins.ListPublic(r.Context())
	if err != nil {
		writeEntError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, p := range items {
		out = append(out, map[string]any{"key": p.Key, "name": p.Name, "config": p.Config})
	}
	response.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

// ListTenantPlugins returns every registered plugin's enablement state for one
// tenant. Platform-admin only.
// (GET /platform/tenants/{tenantId}/plugins)
func (h *Handlers) ListTenantPlugins(w http.ResponseWriter, r *http.Request, tenantId string) {
	if !h.requirePlatformContext(w, r) {
		return
	}
	items, err := h.plugins.ListForTenant(r.Context(), tenantId)
	if err != nil {
		writeEntError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, p := range items {
		out = append(out, tenantPluginStateResponse(p))
	}
	response.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

// UpdateTenantPlugin enables/disables a plugin for a tenant and sets its config
// and write-only secrets. Platform-admin only; secret values are never returned.
// (PUT /platform/tenants/{tenantId}/plugins/{key})
func (h *Handlers) UpdateTenantPlugin(w http.ResponseWriter, r *http.Request, tenantId string, key string) {
	if !h.requirePlatformContext(w, r) {
		return
	}
	var body generated.TenantPluginUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	in := plugin.UpdateInput{Enabled: body.Enabled}
	if body.Config != nil {
		raw, err := json.Marshal(*body.Config)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "Invalid config")
			return
		}
		in.Config = raw
	}
	if body.Secrets != nil {
		in.Secrets = *body.Secrets
	}

	state, err := h.plugins.SetForTenant(r.Context(), tenantId, key, in)
	if err != nil {
		if errors.Is(err, plugin.ErrUnknownPlugin) {
			writeError(w, http.StatusBadRequest, "unknown_plugin", "No plugin is registered for that key")
			return
		}
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, tenantPluginStateResponse(state))
}

func tenantPluginStateResponse(p plugin.TenantPluginState) map[string]any {
	return map[string]any{
		"key":                 p.Key,
		"name":                p.Name,
		"enabled":             p.Enabled,
		"config":              p.Config,
		"secretsConfiguredAt": p.SecretsConfiguredAt,
	}
}
