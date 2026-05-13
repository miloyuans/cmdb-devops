package cloud

import (
	"context"
	"fmt"

	"cmdb-devops/internal/model"
)

type InventorySnapshot struct {
	Resources []model.CloudResource
	IPIndex   []model.IPIndex
	Rules     []model.SecurityGroupRule
	Edges     []model.ResourceEdge
}

type IdentitySnapshot struct {
	Users []model.IAMUser
	Keys  []model.AccessKey
}

type Provider interface {
	Name() string
	ValidateAccount(ctx context.Context, account model.CloudAccount, secret string) error
	DiscoverRegions(ctx context.Context, account model.CloudAccount, secret string) ([]model.RegionInfo, error)
	CollectInventory(ctx context.Context, account model.CloudAccount, secret string, regions []string) (*InventorySnapshot, error)
	CollectIdentity(ctx context.Context, account model.CloudAccount, secret string) (*IdentitySnapshot, error)
}

type Registry struct{ providers map[string]Provider }

func NewRegistry(providers ...Provider) *Registry {
	m := map[string]Provider{}
	for _, p := range providers {
		m[p.Name()] = p
	}
	return &Registry{providers: m}
}

func (r *Registry) Get(name string) (Provider, error) {
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %s not registered", name)
	}
	return p, nil
}

func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.providers))
	for k := range r.providers {
		out = append(out, k)
	}
	return out
}
