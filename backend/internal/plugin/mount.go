package plugin

import (
	"backend/internal/generated/api/generated"

	"github.com/go-chi/chi/v5"
)

func Mount(r chi.Router, plugins []Plugin, wrapper *generated.ServerInterfaceWrapper, mw Middlewares, svc Service) {
	for _, p := range plugins {
		mount := p.Routes(wrapper, mw)
		r.Route("/plugins/"+p.Key(), func(pr chi.Router) {
			pr.Use(svc.RequirePlugin(p.Key()))
			mount(pr)
		})
	}
}
