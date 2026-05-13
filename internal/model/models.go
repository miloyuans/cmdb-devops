package model

import "time"

type Role string

const (
	RoleViewer Role = "viewer"
	RoleAdmin  Role = "admin"
)

type User struct {
	ID           string    `bson:"_id" json:"id"`
	Username     string    `bson:"username" json:"username"`
	PasswordHash string    `bson:"password_hash" json:"-"`
	Role         Role      `bson:"role" json:"role"`
	Enabled      bool      `bson:"enabled" json:"enabled"`
	TelegramID   int64     `bson:"telegram_user_id,omitempty" json:"telegram_user_id,omitempty"`
	CreatedAt    time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time `bson:"updated_at" json:"updated_at"`
}

type Credential struct {
	AccessKeyID        string `bson:"access_key_id" json:"access_key_id"`
	AccessKeySecretEnc string `bson:"access_key_secret_enc,omitempty" json:"-"`
	SecretProvided     bool   `bson:"secret_provided" json:"secret_provided"`
}

type RegionInfo struct {
	Region        string    `bson:"region" json:"region"`
	HasResource   bool      `bson:"has_resource" json:"has_resource"`
	Confidence    string    `bson:"confidence" json:"confidence"`
	Source        string    `bson:"source" json:"source"`
	Services      []string  `bson:"services" json:"services"`
	LastCheckedAt time.Time `bson:"last_checked_at" json:"last_checked_at"`
}

type CloudAccount struct {
	ID               string       `bson:"_id" json:"id"`
	Provider         string       `bson:"provider" json:"provider"`
	Alias            string       `bson:"alias" json:"alias"`
	AccountID        string       `bson:"account_id" json:"account_id"`
	Credential       Credential   `bson:"credential" json:"credential"`
	Enabled          bool         `bson:"enabled" json:"enabled"`
	RegionMode       string       `bson:"region_mode" json:"region_mode"` // auto/manual
	SelectedRegions  []string     `bson:"selected_regions" json:"selected_regions"`
	DetectedRegions  []RegionInfo `bson:"detected_regions" json:"detected_regions"`
	EffectiveRegions []string     `bson:"effective_regions" json:"effective_regions"`
	LastSyncAt       *time.Time   `bson:"last_sync_at,omitempty" json:"last_sync_at,omitempty"`
	LastSyncStatus   string       `bson:"last_sync_status" json:"last_sync_status"`
	CreatedAt        time.Time    `bson:"created_at" json:"created_at"`
	UpdatedAt        time.Time    `bson:"updated_at" json:"updated_at"`
}

func (a CloudAccount) DBName() string {
	return "cmdb_" + a.Provider + "_" + safeDB(a.Alias)
}

func safeDB(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			out = append(out, r)
		} else {
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "unknown"
	}
	return string(out)
}

type CloudResource struct {
	ID                  string         `bson:"_id" json:"id"`
	Provider            string         `bson:"provider" json:"provider"`
	AccountID           string         `bson:"account_id" json:"account_id"`
	AccountAlias        string         `bson:"account_alias" json:"account_alias"`
	Region              string         `bson:"region" json:"region"`
	ResourceType        string         `bson:"resource_type" json:"resource_type"`
	ResourceID          string         `bson:"resource_id" json:"resource_id"`
	ResourceName        string         `bson:"resource_name" json:"resource_name"`
	State               string         `bson:"state" json:"state"`
	VpcID               string         `bson:"vpc_id" json:"vpc_id"`
	SubnetID            string         `bson:"subnet_id" json:"subnet_id"`
	PrivateIPs          []string       `bson:"private_ips" json:"private_ips"`
	PublicIPs           []string       `bson:"public_ips" json:"public_ips"`
	SecurityGroupIDs    []string       `bson:"security_group_ids" json:"security_group_ids"`
	NetworkInterfaceIDs []string       `bson:"network_interface_ids" json:"network_interface_ids"`
	RouteTableIDs       []string       `bson:"route_table_ids" json:"route_table_ids"`
	NatGatewayIDs       []string       `bson:"nat_gateway_ids" json:"nat_gateway_ids"`
	LoadBalancerIDs     []string       `bson:"load_balancer_ids" json:"load_balancer_ids"`
	Raw                 map[string]any `bson:"raw,omitempty" json:"raw,omitempty"`
	UpdatedAt           time.Time      `bson:"updated_at" json:"updated_at"`
}

type IPIndex struct {
	ID               string    `bson:"_id" json:"id"`
	Provider         string    `bson:"provider" json:"provider"`
	AccountID        string    `bson:"account_id" json:"account_id"`
	AccountAlias     string    `bson:"account_alias" json:"account_alias"`
	Region           string    `bson:"region" json:"region"`
	IP               string    `bson:"ip" json:"ip"`
	IPVersion        int       `bson:"ip_version" json:"ip_version"`
	IPType           string    `bson:"ip_type" json:"ip_type"`
	ResourceType     string    `bson:"resource_type" json:"resource_type"`
	ResourceID       string    `bson:"resource_id" json:"resource_id"`
	ResourceName     string    `bson:"resource_name" json:"resource_name"`
	State            string    `bson:"state" json:"state"`
	VpcID            string    `bson:"vpc_id" json:"vpc_id"`
	SubnetID         string    `bson:"subnet_id" json:"subnet_id"`
	SecurityGroupIDs []string  `bson:"security_group_ids" json:"security_group_ids"`
	RouteTableIDs    []string  `bson:"route_table_ids" json:"route_table_ids"`
	NatGatewayIDs    []string  `bson:"nat_gateway_ids" json:"nat_gateway_ids"`
	LoadBalancerIDs  []string  `bson:"load_balancer_ids" json:"load_balancer_ids"`
	UpdatedAt        time.Time `bson:"updated_at" json:"updated_at"`
}

type SecurityGroupRule struct {
	ID              string    `bson:"_id" json:"id"`
	Provider        string    `bson:"provider" json:"provider"`
	AccountID       string    `bson:"account_id" json:"account_id"`
	AccountAlias    string    `bson:"account_alias" json:"account_alias"`
	Region          string    `bson:"region" json:"region"`
	SecurityGroupID string    `bson:"security_group_id" json:"security_group_id"`
	Direction       string    `bson:"direction" json:"direction"` // ingress/egress
	Effect          string    `bson:"effect" json:"effect"`       // allow/deny
	Protocol        string    `bson:"protocol" json:"protocol"`
	FromPort        int       `bson:"from_port" json:"from_port"`
	ToPort          int       `bson:"to_port" json:"to_port"`
	PeerType        string    `bson:"peer_type" json:"peer_type"` // cidr/security_group
	Peer            string    `bson:"peer" json:"peer"`
	Priority        int       `bson:"priority,omitempty" json:"priority,omitempty"`
	Description     string    `bson:"description,omitempty" json:"description,omitempty"`
	UpdatedAt       time.Time `bson:"updated_at" json:"updated_at"`
}

type ResourceEdge struct {
	ID           string    `bson:"_id" json:"id"`
	Provider     string    `bson:"provider" json:"provider"`
	AccountID    string    `bson:"account_id" json:"account_id"`
	AccountAlias string    `bson:"account_alias" json:"account_alias"`
	Region       string    `bson:"region" json:"region"`
	FromType     string    `bson:"from_type" json:"from_type"`
	FromID       string    `bson:"from_id" json:"from_id"`
	ToType       string    `bson:"to_type" json:"to_type"`
	ToID         string    `bson:"to_id" json:"to_id"`
	Relation     string    `bson:"relation" json:"relation"`
	UpdatedAt    time.Time `bson:"updated_at" json:"updated_at"`
}

type IAMUser struct {
	ID               string          `bson:"_id" json:"id"`
	Provider         string          `bson:"provider" json:"provider"`
	AccountID        string          `bson:"account_id" json:"account_id"`
	AccountAlias     string          `bson:"account_alias" json:"account_alias"`
	UserID           string          `bson:"user_id" json:"user_id"`
	UserName         string          `bson:"user_name" json:"user_name"`
	DisplayName      string          `bson:"display_name" json:"display_name"`
	ARN              string          `bson:"arn" json:"arn"`
	UserType         string          `bson:"user_type" json:"user_type"`
	Enabled          bool            `bson:"enabled" json:"enabled"`
	CreateDate       time.Time       `bson:"create_date" json:"create_date"`
	UpdateDate       *time.Time      `bson:"update_date,omitempty" json:"update_date,omitempty"`
	Groups           []string        `bson:"groups" json:"groups"`
	AttachedPolicies []PolicySummary `bson:"attached_policies" json:"attached_policies"`
	InlinePolicies   []string        `bson:"inline_policies" json:"inline_policies"`
	LastSyncedAt     time.Time       `bson:"last_synced_at" json:"last_synced_at"`
}

type PolicySummary struct {
	PolicyName string     `bson:"policy_name" json:"policy_name"`
	PolicyARN  string     `bson:"policy_arn,omitempty" json:"policy_arn,omitempty"`
	PolicyType string     `bson:"policy_type" json:"policy_type"`
	AttachDate *time.Time `bson:"attach_date,omitempty" json:"attach_date,omitempty"`
}

type AccessKey struct {
	ID                string     `bson:"_id" json:"id"`
	Provider          string     `bson:"provider" json:"provider"`
	AccountID         string     `bson:"account_id" json:"account_id"`
	AccountAlias      string     `bson:"account_alias" json:"account_alias"`
	AccessKeyID       string     `bson:"access_key_id" json:"access_key_id"`
	AccessKeyIDHash   string     `bson:"access_key_id_hash" json:"access_key_id_hash"`
	AccessKeyIDMasked string     `bson:"access_key_id_masked" json:"access_key_id_masked"`
	OwnerType         string     `bson:"owner_type" json:"owner_type"`
	OwnerUserID       string     `bson:"owner_user_id" json:"owner_user_id"`
	OwnerUserName     string     `bson:"owner_user_name" json:"owner_user_name"`
	Status            string     `bson:"status" json:"status"`
	Enabled           bool       `bson:"enabled" json:"enabled"`
	CreateDate        time.Time  `bson:"create_date" json:"create_date"`
	UpdateDate        *time.Time `bson:"update_date,omitempty" json:"update_date,omitempty"`
	LastUsedDate      *time.Time `bson:"last_used_date,omitempty" json:"last_used_date,omitempty"`
	LastUsedService   string     `bson:"last_used_service,omitempty" json:"last_used_service,omitempty"`
	LastUsedRegion    string     `bson:"last_used_region,omitempty" json:"last_used_region,omitempty"`
	RiskLevel         string     `bson:"risk_level" json:"risk_level"`
	RiskReasons       []string   `bson:"risk_reasons" json:"risk_reasons"`
	LastSyncedAt      time.Time  `bson:"last_synced_at" json:"last_synced_at"`
}

type AccessKeyGlobalIndex struct {
	ID                string     `bson:"_id" json:"id"`
	AccessKeyIDHash   string     `bson:"access_key_id_hash" json:"access_key_id_hash"`
	AccessKeyIDMasked string     `bson:"access_key_id_masked" json:"access_key_id_masked"`
	Provider          string     `bson:"provider" json:"provider"`
	AccountID         string     `bson:"account_id" json:"account_id"`
	AccountAlias      string     `bson:"account_alias" json:"account_alias"`
	AccountDB         string     `bson:"account_db" json:"account_db"`
	OwnerType         string     `bson:"owner_type" json:"owner_type"`
	OwnerUserID       string     `bson:"owner_user_id" json:"owner_user_id"`
	OwnerUserName     string     `bson:"owner_user_name" json:"owner_user_name"`
	Status            string     `bson:"status" json:"status"`
	Enabled           bool       `bson:"enabled" json:"enabled"`
	CreateDate        time.Time  `bson:"create_date" json:"create_date"`
	UpdateDate        *time.Time `bson:"update_date,omitempty" json:"update_date,omitempty"`
	LastUsedDate      *time.Time `bson:"last_used_date,omitempty" json:"last_used_date,omitempty"`
	LastUsedService   string     `bson:"last_used_service,omitempty" json:"last_used_service,omitempty"`
	LastUsedRegion    string     `bson:"last_used_region,omitempty" json:"last_used_region,omitempty"`
	LastSyncedAt      time.Time  `bson:"last_synced_at" json:"last_synced_at"`
}

type TelegramConfig struct {
	ID              string    `bson:"_id" json:"id"`
	Enabled         bool      `bson:"enabled" json:"enabled"`
	Mode            string    `bson:"mode" json:"mode"` // webhook/polling
	BotName         string    `bson:"bot_name" json:"bot_name"`
	BotTokenEnc     string    `bson:"bot_token_enc,omitempty" json:"-"`
	BotTokenEnv     string    `bson:"bot_token_env,omitempty" json:"bot_token_env,omitempty"`
	WebhookURL      string    `bson:"webhook_url" json:"webhook_url"`
	ParseMode       string    `bson:"parse_mode" json:"parse_mode"`
	RateLimitPerSec int       `bson:"rate_limit_per_second" json:"rate_limit_per_second"`
	MaxWorkers      int       `bson:"max_workers" json:"max_workers"`
	Version         int64     `bson:"version" json:"version"`
	UpdatedBy       string    `bson:"updated_by" json:"updated_by"`
	UpdatedAt       time.Time `bson:"updated_at" json:"updated_at"`
}

type TelegramChat struct {
	ID                string    `bson:"_id" json:"id"`
	ChatID            int64     `bson:"chat_id" json:"chat_id"`
	ChatTitle         string    `bson:"chat_title" json:"chat_title"`
	ChatType          string    `bson:"chat_type" json:"chat_type"`
	Enabled           bool      `bson:"enabled" json:"enabled"`
	AllowQuery        bool      `bson:"allow_query" json:"allow_query"`
	AllowNotification bool      `bson:"allow_notification" json:"allow_notification"`
	DefaultLanguage   string    `bson:"default_language" json:"default_language"`
	MaxResultCount    int       `bson:"max_result_count" json:"max_result_count"`
	CreatedAt         time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt         time.Time `bson:"updated_at" json:"updated_at"`
}

type TelegramSession struct {
	ID             string    `bson:"_id" json:"id"`
	ChatID         int64     `bson:"chat_id" json:"chat_id"`
	TelegramUserID int64     `bson:"telegram_user_id" json:"telegram_user_id"`
	Command        string    `bson:"command" json:"command"`
	State          string    `bson:"state" json:"state"`
	CreatedAt      time.Time `bson:"created_at" json:"created_at"`
	ExpireAt       time.Time `bson:"expire_at" json:"expire_at"`
}

type Job struct {
	ID          string     `bson:"_id" json:"id"`
	JobType     string     `bson:"job_type" json:"job_type"`
	AccountID   string     `bson:"account_id,omitempty" json:"account_id,omitempty"`
	Provider    string     `bson:"provider,omitempty" json:"provider,omitempty"`
	Status      string     `bson:"status" json:"status"`
	TriggerType string     `bson:"trigger_type" json:"trigger_type"`
	Message     string     `bson:"message,omitempty" json:"message,omitempty"`
	StartedAt   time.Time  `bson:"started_at" json:"started_at"`
	FinishedAt  *time.Time `bson:"finished_at,omitempty" json:"finished_at,omitempty"`
	LockUntil   time.Time  `bson:"lock_until" json:"lock_until"`
	CreatedBy   string     `bson:"created_by" json:"created_by"`
}

type AuditLog struct {
	ID        string         `bson:"_id" json:"id"`
	Actor     string         `bson:"actor" json:"actor"`
	Action    string         `bson:"action" json:"action"`
	Target    string         `bson:"target" json:"target"`
	Meta      map[string]any `bson:"meta,omitempty" json:"meta,omitempty"`
	CreatedAt time.Time      `bson:"created_at" json:"created_at"`
}
