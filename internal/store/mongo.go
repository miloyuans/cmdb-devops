package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cmdb-devops/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Store struct {
	Client  *mongo.Client
	AdminDB string
}

func Connect(ctx context.Context, uri, adminDB string) (*Store, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	s := &Store{Client: client, AdminDB: adminDB}
	return s, s.EnsureIndexes(ctx)
}

func (s *Store) Admin() *mongo.Database { return s.Client.Database(s.AdminDB) }
func (s *Store) AccountDB(account model.CloudAccount) *mongo.Database {
	return s.Client.Database(account.DBName())
}
func (s *Store) DB(name string) *mongo.Database { return s.Client.Database(name) }

func (s *Store) EnsureIndexes(ctx context.Context) error {
	admin := s.Admin()
	indexSpecs := map[string][]mongo.IndexModel{
		"users": {
			{Keys: bson.D{{Key: "username", Value: 1}}, Options: options.Index().SetUnique(true)},
		},
		"cloud_accounts": {
			{Keys: bson.D{{Key: "provider", Value: 1}, {Key: "alias", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "enabled", Value: 1}}},
		},
		"jobs": {
			{Keys: bson.D{{Key: "job_type", Value: 1}, {Key: "account_id", Value: 1}, {Key: "status", Value: 1}}},
			{Keys: bson.D{{Key: "lock_until", Value: 1}}},
		},
		"access_key_global_index": {
			{Keys: bson.D{{Key: "access_key_id_hash", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "provider", Value: 1}, {Key: "account_alias", Value: 1}}},
		},
		"telegram_sessions": {
			{Keys: bson.D{{Key: "chat_id", Value: 1}, {Key: "telegram_user_id", Value: 1}, {Key: "state", Value: 1}}},
			{Keys: bson.D{{Key: "expire_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		},
		"telegram_chats": {
			{Keys: bson.D{{Key: "chat_id", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "enabled", Value: 1}}},
		},
		"telegram_bots": {
			{Keys: bson.D{{Key: "name", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "enabled", Value: 1}, {Key: "is_default", Value: 1}}},
		},
		"telegram_users": {
			{Keys: bson.D{{Key: "telegram_user_id", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "enabled", Value: 1}}},
		},
		"audit_logs": {
			{Keys: bson.D{{Key: "created_at", Value: -1}}},
			{Keys: bson.D{{Key: "actor", Value: 1}, {Key: "action", Value: 1}}},
		},
		"system_settings": {
			{Keys: bson.D{{Key: "updated_at", Value: -1}}},
		},
	}
	for col, indexes := range indexSpecs {
		if _, err := admin.Collection(col).Indexes().CreateMany(ctx, indexes); err != nil {
			return fmt.Errorf("create indexes for %s: %w", col, err)
		}
	}
	return nil
}

func (s *Store) EnsureAccountIndexes(ctx context.Context, dbName string) error {
	db := s.DB(dbName)
	indexSpecs := map[string][]mongo.IndexModel{
		"resources": {
			{Keys: bson.D{{Key: "resource_id", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "vpc_id", Value: 1}}},
			{Keys: bson.D{{Key: "subnet_id", Value: 1}}},
		},
		"ip_index": {
			{Keys: bson.D{{Key: "ip", Value: 1}}},
			{Keys: bson.D{{Key: "ip_version", Value: 1}, {Key: "ip_type", Value: 1}}},
			{Keys: bson.D{{Key: "resource_id", Value: 1}}},
			{Keys: bson.D{{Key: "vpc_id", Value: 1}}},
			{Keys: bson.D{{Key: "subnet_id", Value: 1}}},
		},
		"security_group_rules": {
			{Keys: bson.D{{Key: "security_group_id", Value: 1}, {Key: "direction", Value: 1}}},
			{Keys: bson.D{{Key: "peer", Value: 1}}},
		},
		"resource_edges": {
			{Keys: bson.D{{Key: "from_id", Value: 1}}},
			{Keys: bson.D{{Key: "to_id", Value: 1}}},
		},
		"access_keys": {
			{Keys: bson.D{{Key: "access_key_id_hash", Value: 1}}, Options: options.Index().SetUnique(true)},
			{Keys: bson.D{{Key: "owner_user_name", Value: 1}}},
		},
		"iam_users": {
			{Keys: bson.D{{Key: "user_name", Value: 1}}},
		},
	}
	for col, indexes := range indexSpecs {
		if _, err := db.Collection(col).Indexes().CreateMany(ctx, indexes); err != nil {
			return fmt.Errorf("create indexes for %s.%s: %w", dbName, col, err)
		}
	}
	return nil
}

func (s *Store) UpsertUser(ctx context.Context, user model.User) error {
	_, err := s.Admin().Collection("users").UpdateByID(ctx, user.ID, bson.M{"$set": user}, options.Update().SetUpsert(true))
	return err
}

func (s *Store) FindUserByUsername(ctx context.Context, username string) (*model.User, error) {
	var u model.User
	err := s.Admin().Collection("users").FindOne(ctx, bson.M{"username": username}).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &u, err
}

func (s *Store) ListUsers(ctx context.Context) ([]model.User, error) {
	cur, err := s.Admin().Collection("users").Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "username", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []model.User
	return out, cur.All(ctx, &out)
}

func (s *Store) UpsertAccount(ctx context.Context, acc model.CloudAccount) error {
	if err := s.EnsureAccountIndexes(ctx, acc.DBName()); err != nil {
		return err
	}
	_, err := s.Admin().Collection("cloud_accounts").UpdateByID(ctx, acc.ID, bson.M{"$set": acc}, options.Update().SetUpsert(true))
	return err
}

func (s *Store) GetAccount(ctx context.Context, id string) (*model.CloudAccount, error) {
	var acc model.CloudAccount
	err := s.Admin().Collection("cloud_accounts").FindOne(ctx, bson.M{"_id": id}).Decode(&acc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &acc, err
}

func (s *Store) ListAccounts(ctx context.Context, enabledOnly bool) ([]model.CloudAccount, error) {
	filter := bson.M{}
	if enabledOnly {
		filter["enabled"] = true
	}
	cur, err := s.Admin().Collection("cloud_accounts").Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "provider", Value: 1}, {Key: "alias", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []model.CloudAccount
	return out, cur.All(ctx, &out)
}

func (s *Store) UpdateAccountRegions(ctx context.Context, accountID string, detected []model.RegionInfo, effective []string) error {
	_, err := s.Admin().Collection("cloud_accounts").UpdateByID(ctx, accountID, bson.M{"$set": bson.M{
		"detected_regions":  detected,
		"effective_regions": effective,
		"updated_at":        time.Now().UTC(),
	}})
	return err
}

func (s *Store) MarkAccountSync(ctx context.Context, accountID, status string) error {
	now := time.Now().UTC()
	_, err := s.Admin().Collection("cloud_accounts").UpdateByID(ctx, accountID, bson.M{"$set": bson.M{
		"last_sync_at":     now,
		"last_sync_status": status,
		"updated_at":       now,
	}})
	return err
}

func (s *Store) TryStartJob(ctx context.Context, job model.Job) (bool, error) {
	col := s.Admin().Collection("jobs")
	filter := bson.M{
		"job_type":   job.JobType,
		"account_id": job.AccountID,
		"status":     "running",
		"lock_until": bson.M{"$gt": time.Now().UTC()},
	}
	count, err := col.CountDocuments(ctx, filter)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	_, err = col.InsertOne(ctx, job)
	return err == nil, err
}

func (s *Store) FinishJob(ctx context.Context, jobID, status, message string) error {
	now := time.Now().UTC()
	_, err := s.Admin().Collection("jobs").UpdateByID(ctx, jobID, bson.M{"$set": bson.M{
		"status":      status,
		"message":     message,
		"finished_at": now,
	}})
	return err
}

func (s *Store) ListJobs(ctx context.Context, limit int64) ([]model.Job, error) {
	cur, err := s.Admin().Collection("jobs").Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "started_at", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []model.Job
	return out, cur.All(ctx, &out)
}

func (s *Store) ReplaceAccountInventory(ctx context.Context, acc model.CloudAccount, resources []model.CloudResource, ips []model.IPIndex, rules []model.SecurityGroupRule, edges []model.ResourceEdge) error {
	db := s.AccountDB(acc)
	if err := s.EnsureAccountIndexes(ctx, acc.DBName()); err != nil {
		return err
	}
	if err := replaceAll(ctx, db.Collection("resources"), resources); err != nil {
		return err
	}
	if err := replaceAll(ctx, db.Collection("ip_index"), ips); err != nil {
		return err
	}
	if err := replaceAll(ctx, db.Collection("security_group_rules"), rules); err != nil {
		return err
	}
	if err := replaceAll(ctx, db.Collection("resource_edges"), edges); err != nil {
		return err
	}
	return nil
}

func replaceAll[T any](ctx context.Context, col *mongo.Collection, docs []T) error {
	if _, err := col.DeleteMany(ctx, bson.M{}); err != nil {
		return err
	}
	if len(docs) == 0 {
		return nil
	}
	items := make([]interface{}, 0, len(docs))
	for _, d := range docs {
		items = append(items, d)
	}
	_, err := col.InsertMany(ctx, items)
	return err
}

func (s *Store) SearchIP(ctx context.Context, acc model.CloudAccount, ip string) ([]model.IPIndex, error) {
	cur, err := s.AccountDB(acc).Collection("ip_index").Find(ctx, bson.M{"ip": ip})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []model.IPIndex
	return out, cur.All(ctx, &out)
}

func (s *Store) GetResource(ctx context.Context, acc model.CloudAccount, resourceID string) (*model.CloudResource, error) {
	var r model.CloudResource
	err := s.AccountDB(acc).Collection("resources").FindOne(ctx, bson.M{"resource_id": resourceID}).Decode(&r)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &r, err
}

func (s *Store) FindAccessKeyGlobal(ctx context.Context, hash string) (*model.AccessKeyGlobalIndex, error) {
	var idx model.AccessKeyGlobalIndex
	err := s.Admin().Collection("access_key_global_index").FindOne(ctx, bson.M{"access_key_id_hash": hash}).Decode(&idx)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &idx, err
}

func (s *Store) ReplaceIdentity(ctx context.Context, acc model.CloudAccount, users []model.IAMUser, keys []model.AccessKey) error {
	db := s.AccountDB(acc)
	if err := s.EnsureAccountIndexes(ctx, acc.DBName()); err != nil {
		return err
	}
	if err := replaceAll(ctx, db.Collection("iam_users"), users); err != nil {
		return err
	}
	if err := replaceAll(ctx, db.Collection("access_keys"), keys); err != nil {
		return err
	}
	admin := s.Admin().Collection("access_key_global_index")
	if _, err := admin.DeleteMany(ctx, bson.M{"provider": acc.Provider, "account_id": acc.AccountID}); err != nil {
		return err
	}
	for _, key := range keys {
		idx := model.AccessKeyGlobalIndex{
			ID:                "akidx:" + key.AccessKeyIDHash,
			AccessKeyIDHash:   key.AccessKeyIDHash,
			AccessKeyIDMasked: key.AccessKeyIDMasked,
			Provider:          key.Provider,
			AccountID:         key.AccountID,
			AccountAlias:      key.AccountAlias,
			AccountDB:         acc.DBName(),
			OwnerType:         key.OwnerType,
			OwnerUserID:       key.OwnerUserID,
			OwnerUserName:     key.OwnerUserName,
			Status:            key.Status,
			Enabled:           key.Enabled,
			CreateDate:        key.CreateDate,
			UpdateDate:        key.UpdateDate,
			LastUsedDate:      key.LastUsedDate,
			LastUsedService:   key.LastUsedService,
			LastUsedRegion:    key.LastUsedRegion,
			LastSyncedAt:      key.LastSyncedAt,
		}
		_, err := admin.UpdateOne(ctx, bson.M{"access_key_id_hash": key.AccessKeyIDHash}, bson.M{"$set": idx}, options.Update().SetUpsert(true))
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListAccessKeys(ctx context.Context, provider, accountAlias, status, enabled string) ([]model.AccessKeyGlobalIndex, error) {
	filter := bson.M{}
	if provider != "" {
		filter["provider"] = provider
	}
	if accountAlias != "" {
		filter["account_alias"] = accountAlias
	}
	if status != "" {
		filter["status"] = status
	}
	if enabled != "" {
		filter["enabled"] = enabled == "true" || enabled == "1" || enabled == "yes"
	}
	cur, err := s.Admin().Collection("access_key_global_index").Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "last_used_date", Value: -1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []model.AccessKeyGlobalIndex
	return out, cur.All(ctx, &out)
}

func (s *Store) ListIAMUsers(ctx context.Context, provider, accountAlias string) ([]model.IAMUser, error) {
	accounts, err := s.ListAccounts(ctx, false)
	if err != nil {
		return nil, err
	}
	out := make([]model.IAMUser, 0)
	for _, acc := range accounts {
		if provider != "" && acc.Provider != provider {
			continue
		}
		if accountAlias != "" && acc.Alias != accountAlias {
			continue
		}
		cur, err := s.AccountDB(acc).Collection("iam_users").Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "user_name", Value: 1}}))
		if err != nil {
			continue
		}
		var users []model.IAMUser
		if err := cur.All(ctx, &users); err == nil {
			out = append(out, users...)
		}
		_ = cur.Close(ctx)
	}
	return out, nil
}

func (s *Store) FindIAMUserByNameInDB(ctx context.Context, dbName, userName string) (*model.IAMUser, error) {
	var user model.IAMUser
	err := s.DB(dbName).Collection("iam_users").FindOne(ctx, bson.M{"user_name": userName}).Decode(&user)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &user, err
}

func (s *Store) UpsertTelegramConfig(ctx context.Context, cfg model.TelegramConfig) error {
	_, err := s.Admin().Collection("telegram_config").UpdateByID(ctx, cfg.ID, bson.M{"$set": cfg}, options.Update().SetUpsert(true))
	return err
}

func (s *Store) GetTelegramConfig(ctx context.Context) (*model.TelegramConfig, error) {
	var cfg model.TelegramConfig
	err := s.Admin().Collection("telegram_config").FindOne(ctx, bson.M{"_id": "default"}).Decode(&cfg)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &cfg, err
}

func (s *Store) ListTelegramChats(ctx context.Context) ([]model.TelegramChat, error) {
	cur, err := s.Admin().Collection("telegram_chats").Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "chat_title", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []model.TelegramChat
	return out, cur.All(ctx, &out)
}

func (s *Store) UpsertTelegramChat(ctx context.Context, chat model.TelegramChat) error {
	_, err := s.Admin().Collection("telegram_chats").UpdateByID(ctx, chat.ID, bson.M{"$set": chat}, options.Update().SetUpsert(true))
	return err
}

func (s *Store) UpsertTelegramSession(ctx context.Context, session model.TelegramSession) error {
	_, err := s.Admin().Collection("telegram_sessions").UpdateByID(ctx, session.ID, bson.M{"$set": session}, options.Update().SetUpsert(true))
	return err
}

func (s *Store) FindTelegramSession(ctx context.Context, chatID int64, userID int64) (*model.TelegramSession, error) {
	var ses model.TelegramSession
	err := s.Admin().Collection("telegram_sessions").FindOne(ctx, bson.M{"chat_id": chatID, "telegram_user_id": userID, "state": bson.M{"$ne": "completed"}, "expire_at": bson.M{"$gt": time.Now().UTC()}}).Decode(&ses)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &ses, err
}

func (s *Store) Close(ctx context.Context) error {
	if s == nil || s.Client == nil {
		return nil
	}
	return s.Client.Disconnect(ctx)
}

func (s *Store) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	var u model.User
	err := s.Admin().Collection("users").FindOne(ctx, bson.M{"_id": id}).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &u, err
}

func (s *Store) DeleteUser(ctx context.Context, id string) error {
	_, err := s.Admin().Collection("users").DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (s *Store) InsertAudit(ctx context.Context, log model.AuditLog) error {
	_, err := s.Admin().Collection("audit_logs").InsertOne(ctx, log)
	return err
}

func (s *Store) ListAudit(ctx context.Context, limit int64) ([]model.AuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	cur, err := s.Admin().Collection("audit_logs").Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []model.AuditLog
	return out, cur.All(ctx, &out)
}

func (s *Store) GetSettings(ctx context.Context) (*model.SystemSettings, error) {
	var cfg model.SystemSettings
	err := s.Admin().Collection("system_settings").FindOne(ctx, bson.M{"_id": "default"}).Decode(&cfg)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &cfg, err
}

func (s *Store) UpsertSettings(ctx context.Context, cfg model.SystemSettings) error {
	_, err := s.Admin().Collection("system_settings").UpdateByID(ctx, cfg.ID, bson.M{"$set": cfg}, options.Update().SetUpsert(true))
	return err
}

func (s *Store) ListTelegramBots(ctx context.Context) ([]model.TelegramBot, error) {
	cur, err := s.Admin().Collection("telegram_bots").Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "is_default", Value: -1}, {Key: "name", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []model.TelegramBot
	return out, cur.All(ctx, &out)
}

func (s *Store) GetTelegramBot(ctx context.Context, id string) (*model.TelegramBot, error) {
	var bot model.TelegramBot
	err := s.Admin().Collection("telegram_bots").FindOne(ctx, bson.M{"_id": id}).Decode(&bot)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &bot, err
}

func (s *Store) GetDefaultTelegramBot(ctx context.Context) (*model.TelegramBot, error) {
	var bot model.TelegramBot
	err := s.Admin().Collection("telegram_bots").FindOne(ctx, bson.M{"enabled": true, "is_default": true}).Decode(&bot)
	if errors.Is(err, mongo.ErrNoDocuments) {
		err = s.Admin().Collection("telegram_bots").FindOne(ctx, bson.M{"enabled": true}).Decode(&bot)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
	}
	return &bot, err
}

func (s *Store) UpsertTelegramBot(ctx context.Context, bot model.TelegramBot) error {
	if bot.IsDefault {
		_, _ = s.Admin().Collection("telegram_bots").UpdateMany(ctx, bson.M{"_id": bson.M{"$ne": bot.ID}}, bson.M{"$set": bson.M{"is_default": false}})
	}
	_, err := s.Admin().Collection("telegram_bots").UpdateByID(ctx, bot.ID, bson.M{"$set": bot}, options.Update().SetUpsert(true))
	return err
}

func (s *Store) ListTelegramUsers(ctx context.Context) ([]model.TelegramAllowedUser, error) {
	cur, err := s.Admin().Collection("telegram_users").Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "username", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []model.TelegramAllowedUser
	return out, cur.All(ctx, &out)
}

func (s *Store) UpsertTelegramUser(ctx context.Context, user model.TelegramAllowedUser) error {
	_, err := s.Admin().Collection("telegram_users").UpdateByID(ctx, user.ID, bson.M{"$set": user}, options.Update().SetUpsert(true))
	return err
}

func (s *Store) TelegramUserAllowed(ctx context.Context, telegramUserID int64) bool {
	col := s.Admin().Collection("telegram_users")
	count, err := col.CountDocuments(ctx, bson.M{"enabled": true})
	if err != nil || count == 0 {
		return true
	}
	match, err := col.CountDocuments(ctx, bson.M{"telegram_user_id": telegramUserID, "enabled": true})
	return err == nil && match > 0
}

func (s *Store) TelegramChatAllowed(ctx context.Context, chatID int64) bool {
	col := s.Admin().Collection("telegram_chats")
	count, err := col.CountDocuments(ctx, bson.M{"enabled": true})
	if err != nil || count == 0 {
		return true
	}
	match, err := col.CountDocuments(ctx, bson.M{"chat_id": chatID, "enabled": true, "allow_query": true})
	return err == nil && match > 0
}
