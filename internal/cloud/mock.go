package cloud

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"cmdb-devops/internal/model"
	"cmdb-devops/internal/security"
)

type MockProvider struct{ name string }

func NewMockProvider(name string) *MockProvider { return &MockProvider{name: name} }
func (p *MockProvider) Name() string            { return p.name }
func (p *MockProvider) ValidateAccount(ctx context.Context, account model.CloudAccount, secret string) error {
	return nil
}

func (p *MockProvider) DiscoverRegions(ctx context.Context, account model.CloudAccount, secret string) ([]model.RegionInfo, error) {
	now := time.Now().UTC()
	regions := []string{"ap-southeast-1", "us-east-1"}
	if p.name == "aliyun" {
		regions = []string{"cn-hangzhou", "cn-shanghai"}
	}
	return []model.RegionInfo{
		{Region: regions[0], HasResource: true, Confidence: "high", Source: "mock_light_probe", Services: []string{"ecs", "vpc", "security_group", "load_balancer"}, LastCheckedAt: now},
		{Region: regions[1], HasResource: false, Confidence: "medium", Source: "mock_event_history", Services: []string{}, LastCheckedAt: now},
	}, nil
}

func (p *MockProvider) CollectInventory(ctx context.Context, account model.CloudAccount, secret string, regions []string) (*InventorySnapshot, error) {
	now := time.Now().UTC()
	if len(regions) == 0 {
		regions = []string{"ap-southeast-1"}
	}
	var resources []model.CloudResource
	var ips []model.IPIndex
	var rules []model.SecurityGroupRule
	var edges []model.ResourceEdge
	for _, region := range regions {
		if region == "" {
			continue
		}
		prefix := "i"
		if p.name == "aliyun" {
			prefix = "ecs"
		}
		instID := fmt.Sprintf("%s-%s-web-001", prefix, region)
		privateIP := "10.0.1.12"
		if p.name == "aliyun" {
			privateIP = "10.1.1.12"
		}
		res := model.CloudResource{
			ID:       p.name + ":" + account.AccountID + ":" + region + ":" + instID,
			Provider: p.name, AccountID: account.AccountID, AccountAlias: account.Alias, Region: region,
			ResourceType: "compute", ResourceID: instID, ResourceName: account.Alias + "-web-01", State: "running",
			VpcID: "vpc-001", SubnetID: "subnet-a", PrivateIPs: []string{privateIP}, PublicIPs: []string{"54.1.2.3"},
			SecurityGroupIDs: []string{"sg-web"}, NetworkInterfaceIDs: []string{"eni-web-001"},
			RouteTableIDs: []string{"rtb-main"}, NatGatewayIDs: []string{"nat-main"}, LoadBalancerIDs: []string{"alb-web"}, UpdatedAt: now,
			Raw: map[string]any{"mock": true},
		}
		resources = append(resources, res)
		ips = append(ips, buildIPIndex(account, res, privateIP, "private", now), buildIPIndex(account, res, "54.1.2.3", "public", now))
		rules = append(rules,
			model.SecurityGroupRule{ID: p.name + ":" + region + ":sg-web:egress:any", Provider: p.name, AccountID: account.AccountID, AccountAlias: account.Alias, Region: region, SecurityGroupID: "sg-web", Direction: "egress", Effect: "allow", Protocol: "all", FromPort: -1, ToPort: -1, PeerType: "cidr", Peer: "0.0.0.0/0", Description: "mock allow all egress", UpdatedAt: now},
			model.SecurityGroupRule{ID: p.name + ":" + region + ":sg-web:ingress:443", Provider: p.name, AccountID: account.AccountID, AccountAlias: account.Alias, Region: region, SecurityGroupID: "sg-web", Direction: "ingress", Effect: "allow", Protocol: "tcp", FromPort: 443, ToPort: 443, PeerType: "cidr", Peer: "10.0.0.0/16", Description: "mock allow https from vpc", UpdatedAt: now},
		)
		edges = append(edges,
			edge(account, region, "compute", instID, "eni", "eni-web-001", "attached_to", now),
			edge(account, region, "eni", "eni-web-001", "subnet", "subnet-a", "in_subnet", now),
			edge(account, region, "subnet", "subnet-a", "vpc", "vpc-001", "in_vpc", now),
			edge(account, region, "subnet", "subnet-a", "route_table", "rtb-main", "uses_route_table", now),
			edge(account, region, "route_table", "rtb-main", "nat_gateway", "nat-main", "default_egress", now),
			edge(account, region, "load_balancer", "alb-web", "compute", instID, "targets", now),
		)
	}
	return &InventorySnapshot{Resources: resources, IPIndex: ips, Rules: rules, Edges: edges}, nil
}

func (p *MockProvider) CollectIdentity(ctx context.Context, account model.CloudAccount, secret string) (*IdentitySnapshot, error) {
	now := time.Now().UTC()
	created := now.AddDate(-1, 0, 0)
	lastUsed := now.Add(-24 * time.Hour)
	ak := "AKIA" + account.AccountID + "MOCK"
	if p.name == "aliyun" {
		ak = "LTAI" + account.AccountID + "MOCK"
	}
	user := model.IAMUser{
		ID: p.name + ":" + account.AccountID + ":user:deploy-user", Provider: p.name, AccountID: account.AccountID, AccountAlias: account.Alias,
		UserID: "user-deploy", UserName: "deploy-user", DisplayName: "deploy-user", ARN: p.name + ":" + account.AccountID + ":user/deploy-user",
		UserType: "iam_user", Enabled: true, CreateDate: created, Groups: []string{"devops"},
		AttachedPolicies: []model.PolicySummary{{PolicyName: "ReadOnlyAccess", PolicyType: "managed"}}, InlinePolicies: []string{"custom-cmdb-read"}, LastSyncedAt: now,
	}
	key := model.AccessKey{
		ID: p.name + ":" + account.AccountID + ":ak:" + security.HashAccessKeyID(ak), Provider: p.name, AccountID: account.AccountID, AccountAlias: account.Alias,
		AccessKeyID: ak, AccessKeyIDHash: security.HashAccessKeyID(ak), AccessKeyIDMasked: security.MaskAccessKeyID(ak), OwnerType: "user", OwnerUserID: user.UserID, OwnerUserName: user.UserName,
		Status: "active", Enabled: true, CreateDate: created, LastUsedDate: &lastUsed, LastUsedService: "ec2", LastUsedRegion: "ap-southeast-1", RiskLevel: "medium", RiskReasons: []string{"AK enabled", "mock key used recently"}, LastSyncedAt: now,
	}
	return &IdentitySnapshot{Users: []model.IAMUser{user}, Keys: []model.AccessKey{key}}, nil
}

func buildIPIndex(account model.CloudAccount, r model.CloudResource, ip, ipType string, now time.Time) model.IPIndex {
	addr, _ := netip.ParseAddr(ip)
	version := 4
	if addr.Is6() {
		version = 6
	}
	return model.IPIndex{
		ID: r.Provider + ":" + account.AccountID + ":" + r.Region + ":" + ip + ":" + r.ResourceID, Provider: r.Provider, AccountID: account.AccountID, AccountAlias: account.Alias, Region: r.Region,
		IP: ip, IPVersion: version, IPType: ipType, ResourceType: r.ResourceType, ResourceID: r.ResourceID, ResourceName: r.ResourceName, State: r.State,
		VpcID: r.VpcID, SubnetID: r.SubnetID, SecurityGroupIDs: r.SecurityGroupIDs, RouteTableIDs: r.RouteTableIDs, NatGatewayIDs: r.NatGatewayIDs, LoadBalancerIDs: r.LoadBalancerIDs, UpdatedAt: now,
	}
}

func edge(account model.CloudAccount, region, fromType, fromID, toType, toID, relation string, now time.Time) model.ResourceEdge {
	return model.ResourceEdge{ID: account.Provider + ":" + account.AccountID + ":" + region + ":" + fromID + ":" + relation + ":" + toID, Provider: account.Provider, AccountID: account.AccountID, AccountAlias: account.Alias, Region: region, FromType: fromType, FromID: fromID, ToType: toType, ToID: toID, Relation: relation, UpdatedAt: now}
}
