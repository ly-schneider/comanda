package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"backend/internal/generated/api/generated"
	"backend/internal/plugin"
	"backend/internal/response"
)

func (h *Handlers) ListPlugins(w http.ResponseWriter, r *http.Request) {
	items, err := h.plugins.ListPublic(r.Context())
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, generated.PublicPluginList{Items: toAPIPublicPlugins(items)})
}

func (h *Handlers) ListTenantPlugins(w http.ResponseWriter, r *http.Request, tenantId string) {
	if !h.requirePlatformContext(w, r) {
		return
	}
	items, err := h.plugins.ListForTenant(r.Context(), tenantId)
	if err != nil {
		writeEntError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, generated.TenantPluginList{Items: toAPITenantPluginStates(items)})
}

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
	response.WriteJSON(w, http.StatusOK, toAPITenantPluginState(state))
}
