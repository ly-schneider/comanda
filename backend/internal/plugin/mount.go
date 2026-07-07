package plugin

import (
	"backend/internal/generated/api/generated"

	"github.com/go-chi/chi/v5"
)

// Mount wires each plugin's routes under /plugins/<key>, gated by RequirePlugin
// so a disabled plugin's whole subtree 404s. Shared by the HTTP router and its
// tests so both exercise the identical mounting path.
func Mount(r chi.Router, plugins []Plugin, wrapper *generated.ServerInterfaceWrapper, mw Middlewares, svc Service) {
	for _, p := range plugins {
		mount := p.Routes(wrapper, mw)
		r.Route("/plugins/"+p.Key(), func(pr chi.Router) {
			pr.Use(svc.RequirePlugin(p.Key()))
			mount(pr)
		})
	}
}
