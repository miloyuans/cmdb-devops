package service

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"cmdb-devops/internal/model"
	"cmdb-devops/internal/store"
)

type QueryService struct{ Store *store.Store }

type IPQuery struct {
	Raw     string `json:"raw"`
	IsCIDR  bool   `json:"is_cidr"`
	Version int    `json:"version"`
	IPType  string `json:"ip_type"`
	Address string `json:"address,omitempty"`
	Prefix  string `json:"prefix,omitempty"`
}

type IPQueryResult struct {
	Query    IPQuery         `json:"query"`
	CacheHit bool            `json:"cache_hit"`
	Matches  []model.IPIndex `json:"matches"`
	Warnings []string        `json:"warnings"`
}

func ParseIPQuery(input string) (IPQuery, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return IPQuery{}, fmt.Errorf("empty query")
	}
	if strings.Contains(input, "/") {
		p, err := netip.ParsePrefix(input)
		if err != nil {
			return IPQuery{}, err
		}
		v := 4
		if p.Addr().Is6() {
			v = 6
		}
		ipType := "public"
		if p.Addr().IsPrivate() {
			ipType = "private"
		}
		return IPQuery{Raw: input, IsCIDR: true, Version: v, IPType: ipType, Prefix: p.String()}, nil
	}
	addr, err := netip.ParseAddr(input)
	if err != nil {
		return IPQuery{}, err
	}
	v := 4
	if addr.Is6() {
		v = 6
	}
	ipType := "public"
	if addr.IsPrivate() {
		ipType = "private"
	}
	return IPQuery{Raw: input, IsCIDR: false, Version: v, IPType: ipType, Address: addr.String()}, nil
}

func (s *QueryService) SearchIP(ctx context.Context, input string) (*IPQueryResult, error) {
	q, err := ParseIPQuery(input)
	if err != nil {
		return nil, err
	}
	accounts, err := s.Store.ListAccounts(ctx, true)
	if err != nil {
		return nil, err
	}
	matches := make([]model.IPIndex, 0)
	warnings := []string{}
	for _, acc := range accounts {
		if q.IsCIDR {
			part, err := s.searchCIDR(ctx, acc, q.Prefix)
			if err != nil {
				warnings = append(warnings, acc.Alias+": "+err.Error())
				continue
			}
			matches = append(matches, part...)
		} else {
			part, err := s.Store.SearchIP(ctx, acc, q.Address)
			if err != nil {
				warnings = append(warnings, acc.Alias+": "+err.Error())
				continue
			}
			matches = append(matches, part...)
		}
	}
	matches = dedupeIPIndex(matches)
	return &IPQueryResult{Query: q, CacheHit: len(matches) > 0, Matches: matches, Warnings: warnings}, nil
}

func (s *QueryService) searchCIDR(ctx context.Context, acc model.CloudAccount, prefix string) ([]model.IPIndex, error) {
	// Simple production-safe first version: scan only account ip_index, not cloud APIs. For very large fleets,
	// extend this by storing numeric IPv4/IPv6 ranges in Mongo and using range indexes.
	cur, err := s.Store.AccountDB(acc).Collection("ip_index").Find(ctx, map[string]any{})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var all []model.IPIndex
	if err := cur.All(ctx, &all); err != nil {
		return nil, err
	}
	p, err := netip.ParsePrefix(prefix)
	if err != nil {
		return nil, err
	}
	out := make([]model.IPIndex, 0)
	for _, item := range all {
		addr, err := netip.ParseAddr(item.IP)
		if err == nil && p.Contains(addr) {
			out = append(out, item)
		}
	}
	return out, nil
}

func dedupeIPIndex(in []model.IPIndex) []model.IPIndex {
	seen := map[string]bool{}
	out := make([]model.IPIndex, 0, len(in))
	for _, item := range in {
		key := item.Provider + "|" + item.AccountID + "|" + item.Region + "|" + item.IP + "|" + item.ResourceID
		if !seen[key] {
			seen[key] = true
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].AccountAlias+out[i].Region+out[i].ResourceID < out[j].AccountAlias+out[j].Region+out[j].ResourceID
	})
	return out
}

type ConnectivityRequest struct {
	SourceIP string `json:"source_ip"`
	TargetIP string `json:"target_ip"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
}

type ConnectivityResult struct {
	Source        []model.IPIndex `json:"source"`
	Target        []model.IPIndex `json:"target"`
	NetworkStatus string          `json:"network_status"`
	SGStatus      string          `json:"security_group_status"`
	Result        string          `json:"result"`
	Reason        string          `json:"reason"`
	CheckedAt     time.Time       `json:"checked_at"`
}

func (s *QueryService) AnalyzeConnectivity(ctx context.Context, req ConnectivityRequest) (*ConnectivityResult, error) {
	if req.Protocol == "" {
		req.Protocol = "tcp"
	}
	if req.Port == 0 {
		req.Port = 443
	}
	src, err := s.SearchIP(ctx, req.SourceIP)
	if err != nil {
		return nil, err
	}
	tgt, err := s.SearchIP(ctx, req.TargetIP)
	if err != nil {
		return nil, err
	}
	res := &ConnectivityResult{Source: src.Matches, Target: tgt.Matches, CheckedAt: time.Now().UTC()}
	if len(src.Matches) == 0 || len(tgt.Matches) == 0 {
		res.Result = "unknown"
		res.NetworkStatus = "unknown"
		res.SGStatus = "unknown"
		res.Reason = "source or target not found in cached CMDB"
		return res, nil
	}
	a, b := src.Matches[0], tgt.Matches[0]
	if a.AccountID == b.AccountID && a.Region == b.Region && a.VpcID == b.VpcID {
		res.NetworkStatus = "reachable_same_vpc"
	} else {
		res.NetworkStatus = "unknown_cross_network"
		res.Result = "unknown"
		res.SGStatus = "not_evaluated"
		res.Reason = "resources are not in same account/region/VPC; route-peering/TGW/CEN analysis needs corresponding route resources"
		return res, nil
	}
	allowed := false
	for _, sg := range b.SecurityGroupIDs {
		if s.targetIngressAllows(ctx, a, b, sg, req.Protocol, req.Port) {
			allowed = true
			break
		}
	}
	if allowed {
		res.SGStatus = "target_ingress_allowed"
		res.Result = "allowed"
		res.Reason = "same VPC and target security group has matching ingress rule"
	} else {
		res.SGStatus = "target_ingress_denied"
		res.Result = "blocked"
		res.Reason = "network path appears reachable, but target security group does not allow the requested traffic"
	}
	return res, nil
}

func (s *QueryService) targetIngressAllows(ctx context.Context, src, tgt model.IPIndex, sg, proto string, port int) bool {
	acc := model.CloudAccount{Provider: tgt.Provider, AccountID: tgt.AccountID, Alias: tgt.AccountAlias}
	cur, err := s.Store.AccountDB(acc).Collection("security_group_rules").Find(ctx, map[string]any{"security_group_id": sg, "direction": "ingress"})
	if err != nil {
		return false
	}
	defer cur.Close(ctx)
	var rules []model.SecurityGroupRule
	if err := cur.All(ctx, &rules); err != nil {
		return false
	}
	srcAddr, _ := netip.ParseAddr(src.IP)
	for _, r := range rules {
		if r.Effect == "deny" {
			continue
		}
		if r.Protocol != "all" && r.Protocol != proto {
			continue
		}
		if r.FromPort >= 0 && !(port >= r.FromPort && port <= r.ToPort) {
			continue
		}
		if r.PeerType == "cidr" {
			prefix, err := netip.ParsePrefix(r.Peer)
			if err == nil && prefix.Contains(srcAddr) {
				return true
			}
		}
		if r.PeerType == "security_group" {
			for _, ssg := range src.SecurityGroupIDs {
				if ssg == r.Peer {
					return true
				}
			}
		}
	}
	return false
}
