package jobs

import (
	"context"
	"fmt"
	"log"
	"time"

	"cmdb-devops/internal/cloud"
	"cmdb-devops/internal/config"
	"cmdb-devops/internal/model"
	"cmdb-devops/internal/security"
	"cmdb-devops/internal/store"
)

type Manager struct {
	Store    *store.Store
	Registry *cloud.Registry
	Config   config.Config
}

func (m *Manager) Start(ctx context.Context) {
	go m.loop(ctx, "inventory_sync", m.Config.InventoryInterval, func(ctx context.Context) { m.RunInventoryAll(ctx, "schedule", "system") })
	go m.loop(ctx, "region_discovery", m.Config.RegionCheckInterval, func(ctx context.Context) { m.RunRegionDiscoveryAll(ctx, "schedule", "system") })
	go m.loop(ctx, "identity_sync", m.Config.IdentityInterval, func(ctx context.Context) { m.RunIdentityAll(ctx, "schedule", "system") })
}

func (m *Manager) loop(ctx context.Context, name string, interval time.Duration, fn func(context.Context)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	log.Printf("job loop %s started, interval=%s", name, interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fn(ctx)
		}
	}
}

func (m *Manager) TriggerAccountJob(ctx context.Context, jobType, accountID, actor string) (bool, string, error) {
	jobID := fmt.Sprintf("job_%s_%s_%d", jobType, accountID, time.Now().UnixNano())
	job := model.Job{ID: jobID, JobType: jobType, AccountID: accountID, Status: "running", TriggerType: "manual", StartedAt: time.Now().UTC(), LockUntil: time.Now().UTC().Add(30 * time.Minute), CreatedBy: actor}
	ok, err := m.Store.TryStartJob(ctx, job)
	if err != nil || !ok {
		return ok, jobID, err
	}
	go m.runAccountJob(context.Background(), job)
	return true, jobID, nil
}

func (m *Manager) RequestMissRefresh(ctx context.Context, jobType, provider string) {
	jobID := fmt.Sprintf("job_%s_%s_%d", jobType, provider, time.Now().UnixNano())
	job := model.Job{ID: jobID, JobType: jobType, Provider: provider, Status: "running", TriggerType: "miss", StartedAt: time.Now().UTC(), LockUntil: time.Now().UTC().Add(5 * time.Minute), CreatedBy: "query_miss"}
	ok, err := m.Store.TryStartJob(ctx, job)
	if err != nil {
		log.Printf("miss refresh lock error: %v", err)
		return
	}
	if !ok {
		return
	}
	go func() {
		defer func() { _ = m.Store.FinishJob(context.Background(), jobID, "success", "miss refresh completed") }()
		if jobType == "identity_sync" {
			m.RunIdentityAll(context.Background(), "miss", "query_miss")
		} else {
			m.RunInventoryAll(context.Background(), "miss", "query_miss")
		}
	}()
}

func (m *Manager) runAccountJob(ctx context.Context, job model.Job) {
	var err error
	switch job.JobType {
	case "region_discovery":
		err = m.RunRegionDiscovery(ctx, job.AccountID)
	case "inventory_sync":
		err = m.RunInventory(ctx, job.AccountID)
	case "identity_sync":
		err = m.RunIdentity(ctx, job.AccountID)
	default:
		err = fmt.Errorf("unknown job type %s", job.JobType)
	}
	if err != nil {
		log.Printf("job %s failed: %v", job.ID, err)
		_ = m.Store.FinishJob(ctx, job.ID, "failed", err.Error())
		return
	}
	_ = m.Store.FinishJob(ctx, job.ID, "success", "ok")
}

func (m *Manager) RunRegionDiscoveryAll(ctx context.Context, trigger, actor string) {
	accounts, err := m.Store.ListAccounts(ctx, true)
	if err != nil {
		log.Printf("list accounts for region discovery: %v", err)
		return
	}
	for _, acc := range accounts {
		if err := m.RunRegionDiscovery(ctx, acc.ID); err != nil {
			log.Printf("region discovery %s: %v", acc.Alias, err)
		}
	}
}

func (m *Manager) RunInventoryAll(ctx context.Context, trigger, actor string) {
	accounts, err := m.Store.ListAccounts(ctx, true)
	if err != nil {
		log.Printf("list accounts for inventory: %v", err)
		return
	}
	for _, acc := range accounts {
		if err := m.RunInventory(ctx, acc.ID); err != nil {
			log.Printf("inventory %s: %v", acc.Alias, err)
		}
	}
}

func (m *Manager) RunIdentityAll(ctx context.Context, trigger, actor string) {
	accounts, err := m.Store.ListAccounts(ctx, true)
	if err != nil {
		log.Printf("list accounts for identity: %v", err)
		return
	}
	for _, acc := range accounts {
		if err := m.RunIdentity(ctx, acc.ID); err != nil {
			log.Printf("identity %s: %v", acc.Alias, err)
		}
	}
}

func (m *Manager) RunRegionDiscovery(ctx context.Context, accountID string) error {
	acc, err := m.Store.GetAccount(ctx, accountID)
	if err != nil {
		return err
	}
	if acc == nil {
		return fmt.Errorf("account not found")
	}
	p, err := m.Registry.Get(acc.Provider)
	if err != nil {
		return err
	}
	secret, _ := security.Decrypt(acc.Credential.AccessKeySecretEnc, m.Config.EncryptionKey)
	regions, err := p.DiscoverRegions(ctx, *acc, secret)
	if err != nil {
		return err
	}
	effective := acc.SelectedRegions
	if acc.RegionMode != "manual" || len(effective) == 0 {
		effective = []string{}
		for _, r := range regions {
			if r.HasResource {
				effective = append(effective, r.Region)
			}
		}
	}
	return m.Store.UpdateAccountRegions(ctx, acc.ID, regions, effective)
}

func (m *Manager) RunInventory(ctx context.Context, accountID string) error {
	acc, err := m.Store.GetAccount(ctx, accountID)
	if err != nil {
		return err
	}
	if acc == nil {
		return fmt.Errorf("account not found")
	}
	p, err := m.Registry.Get(acc.Provider)
	if err != nil {
		return err
	}
	secret, _ := security.Decrypt(acc.Credential.AccessKeySecretEnc, m.Config.EncryptionKey)
	regions := acc.EffectiveRegions
	if len(regions) == 0 {
		if err := m.RunRegionDiscovery(ctx, acc.ID); err != nil {
			return err
		}
		acc, _ = m.Store.GetAccount(ctx, accountID)
		if acc != nil {
			regions = acc.EffectiveRegions
		}
	}
	snap, err := p.CollectInventory(ctx, *acc, secret, regions)
	if err != nil {
		_ = m.Store.MarkAccountSync(ctx, acc.ID, "failed")
		return err
	}
	if err := m.Store.ReplaceAccountInventory(ctx, *acc, snap.Resources, snap.IPIndex, snap.Rules, snap.Edges); err != nil {
		return err
	}
	return m.Store.MarkAccountSync(ctx, acc.ID, "success")
}

func (m *Manager) RunIdentity(ctx context.Context, accountID string) error {
	acc, err := m.Store.GetAccount(ctx, accountID)
	if err != nil {
		return err
	}
	if acc == nil {
		return fmt.Errorf("account not found")
	}
	p, err := m.Registry.Get(acc.Provider)
	if err != nil {
		return err
	}
	secret, _ := security.Decrypt(acc.Credential.AccessKeySecretEnc, m.Config.EncryptionKey)
	snap, err := p.CollectIdentity(ctx, *acc, secret)
	if err != nil {
		return err
	}
	return m.Store.ReplaceIdentity(ctx, *acc, snap.Users, snap.Keys)
}
