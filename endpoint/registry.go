package endpoint

import (
	"strconv"
	"sync"

	"github.com/tokzone/fluxcore/provider"
)

type Registry struct {
	byID        sync.Map
	byProvModel sync.Map
}

var globalRegistry = NewRegistry()

func NewRegistry() *Registry {
	return &Registry{}
}

func GlobalRegistry() *Registry {
	return globalRegistry
}

func provModelKey(providerID uint, model string) string {
	return strconv.FormatUint(uint64(providerID), 10) + ":" + model
}

func (r *Registry) Register(ep *Endpoint) {
	r.byID.Store(ep.ID, ep)
	key := provModelKey(ep.Provider().ID, ep.Model)
	r.byProvModel.Store(key, ep)
}

func (r *Registry) Get(id uint) *Endpoint {
	if v, ok := r.byID.Load(id); ok {
		return v.(*Endpoint)
	}
	return nil
}

func (r *Registry) GetByProviderModel(prov *provider.Provider, model string) *Endpoint {
	key := provModelKey(prov.ID, model)
	if v, ok := r.byProvModel.Load(key); ok {
		return v.(*Endpoint)
	}
	return nil
}

func (r *Registry) GetAll() []*Endpoint {
	var result []*Endpoint
	r.byID.Range(func(key, value interface{}) bool {
		result = append(result, value.(*Endpoint))
		return true
	})
	return result
}

func (r *Registry) Clear() {
	r.byID = sync.Map{}
	r.byProvModel = sync.Map{}
}

func RegisterEndpoint(id uint, prov *provider.Provider, model string) *Endpoint {
	ep, err := NewEndpoint(id, prov, model)
	if err != nil {
		return nil
	}
	globalRegistry.Register(ep)
	return ep
}
