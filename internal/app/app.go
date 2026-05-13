package app

import (
	"context"
	"log"
	"time"

	"cmdb-devops/internal/auth"
	"cmdb-devops/internal/cloud"
	"cmdb-devops/internal/config"
	"cmdb-devops/internal/httpapi"
	"cmdb-devops/internal/jobs"
	"cmdb-devops/internal/model"
	"cmdb-devops/internal/service"
	"cmdb-devops/internal/store"
	"cmdb-devops/internal/telegram"
)

type App struct {
	Config config.Config
	Store  *store.Store
	Jobs   *jobs.Manager
	HTTP   *httpapi.Server
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	st, err := store.Connect(ctx, cfg.MongoURI, cfg.MongoAdminDB)
	if err != nil {
		return nil, err
	}
	if err := seedAdmin(ctx, st, cfg); err != nil {
		return nil, err
	}
	registry := cloud.NewRegistry(cloud.NewMockProvider("aws"), cloud.NewMockProvider("aliyun"))
	query := &service.QueryService{Store: st}
	jm := &jobs.Manager{Store: st, Registry: registry, Config: cfg}
	tg := &telegram.Service{Store: st, Query: query, Config: cfg}
	httpSrv := &httpapi.Server{Cfg: cfg, Store: st, Jobs: jm, Query: query, Telegram: tg}
	return &App{Config: cfg, Store: st, Jobs: jm, HTTP: httpSrv}, nil
}

func seedAdmin(ctx context.Context, st *store.Store, cfg config.Config) error {
	existing, err := st.FindUserByUsername(ctx, cfg.DefaultAdminUser)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}
	hash, err := auth.HashPassword(cfg.DefaultAdminPassword)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	u := model.User{ID: "user_admin", Username: cfg.DefaultAdminUser, PasswordHash: hash, Role: model.RoleAdmin, Enabled: true, CreatedAt: now, UpdatedAt: now}
	log.Printf("seed default admin user %s", cfg.DefaultAdminUser)
	return st.UpsertUser(ctx, u)
}
