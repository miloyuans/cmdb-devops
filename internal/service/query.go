package service

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"sync"
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

	var mu sync.Mutex
	matches := make([]model.IPIndex, 0)
	warnings := make([]string, 0)
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup

	for _, account := range accounts {
		acc := account
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			qctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			var part []model.IPIndex
			var qerr error
			if q.IsCIDR {
				part, qerr = s.searchCIDR(qctx, acc, q.Prefix)
			} else {
				part, qerr = s.Store.SearchIP(qctx, acc, q.Address)
			}

			mu.Lock()
			defer mu.Unlock()
			if qerr != nil {
				warnings = append(warnings, acc.Alias+": "+qerr.Error())
				return
			}
			matches = append(matches, part...)
		}()
	}
	wg.Wait()

	matches = dedupeIPIndex(matches)
	return &IPQueryResult{Query: q, CacheHit: len(matches) > 0, Matches: matches, Warnings: warnings}, nil
}

func (s *QueryService) searchCIDR(ctx context.Context, acc model.CloudAccount, prefix string) ([]model.IPIndex, error) {
	// First production-safe version: scan only account ip_index from Mongo, never cloud APIs.
	// For huge fleets, add numeric ip_start/ip_end fields and range indexes.
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
	Source              []model.IPIndex `json:"source"`
	Target              []model.IPIndex `json:"target"`
	NetworkStatus       string          `json:"network_status"`
	SourceEgressStatus  string          `json:"source_egress_status"`
	TargetIngressStatus string          `json:"target_ingress_status"`
	SGStatus            string          `json:"security_group_status"`
	Result              string          `json:"result"`
	Reason              string          `json:"reason"`
	CheckedAt           time.Time       `json:"checked_at"`
}

func (s *QueryService) AnalyzeConnectivity(ctx context.Context, req ConnectivityRequest) (*ConnectivityResult, error) {
	if req.Protocol == "" {
		req.Protocol = "tcp"
	}
	req.Protocol = strings.ToLower(req.Protocol)
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
		res.SourceEgressStatus = "unknown"
		res.TargetIngressStatus = "unknown"
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
		res.SourceEgressStatus = "not_evaluated"
		res.TargetIngressStatus = "not_evaluated"
		res.Reason = "resources are not in same account/region/VPC; route-peering/TGW/CEN analysis needs corresponding route resources"
		return res, nil
	}

	egressAllowed := s.anyRuleAllows(ctx, a, b, a.SecurityGroupIDs, "egress", req.Protocol, req.Port)
	ingressAllowed := s.anyRuleAllows(ctx, a, b, b.SecurityGroupIDs, "ingress", req.Protocol, req.Port)

	if egressAllowed {
		res.SourceEgressStatus = "allowed"
	} else {
		res.SourceEgressStatus = "denied_or_not_found"
	}
	if ingressAllowed {
		res.TargetIngressStatus = "allowed"
	} else {
		res.TargetIngressStatus = "denied_or_not_found"
	}

	switch {
	case egressAllowed && ingressAllowed:
		res.SGStatus = "source_egress_and_target_ingress_allowed"
		res.Result = "allowed"
		res.Reason = "same VPC, source egress allows the target, and target ingress allows the source"
	case !egressAllowed && !ingressAllowed:
		res.SGStatus = "source_egress_and_target_ingress_denied"
		res.Result = "blocked"
		res.Reason = "network path appears reachable, but both source egress and target ingress are not allowed by cached security group rules"
	case !egressAllowed:
		res.SGStatus = "source_egress_denied"
		res.Result = "blocked"
		res.Reason = "network path appears reachable, but source security group egress does not allow the requested traffic"
	default:
		res.SGStatus = "target_ingress_denied"
		res.Result = "blocked"
		res.Reason = "network path appears reachable, but target security group ingress does not allow the requested traffic"
	}
	return res, nil
}

func (s *QueryService) anyRuleAllows(ctx context.Context, src, tgt model.IPIndex, securityGroups []string, direction, proto string, port int) bool {
	for _, sg := range securityGroups {
		if s.rulesAllow(ctx, src, tgt, sg, direction, proto, port) {
			return true
		}
	}
	return false
}

func (s *QueryService) rulesAllow(ctx context.Context, src, tgt model.IPIndex, sg, direction, proto string, port int) bool {
	acc := model.CloudAccount{Provider: tgt.Provider, AccountID: tgt.AccountID, Alias: tgt.AccountAlias}
	if direction == "egress" {
		acc = model.CloudAccount{Provider: src.Provider, AccountID: src.AccountID, Alias: src.AccountAlias}
	}
	cur, err := s.Store.AccountDB(acc).Collection("security_group_rules").Find(ctx, map[string]any{"security_group_id": sg, "direction": direction})
	if err != nil {
		return false
	}
	defer cur.Close(ctx)
	var rules []model.SecurityGroupRule
	if err := cur.All(ctx, &rules); err != nil {
		return false
	}

	peerAddr := src.IP
	peerSGs := src.SecurityGroupIDs
	if direction == "egress" {
		peerAddr = tgt.IP
		peerSGs = tgt.SecurityGroupIDs
	}
	addr, _ := netip.ParseAddr(peerAddr)

	for _, r := range rules {
		if strings.EqualFold(r.Effect, "deny") {
			continue
		}
		ruleProto := strings.ToLower(r.Protocol)
		if ruleProto != "all" && ruleProto != "-1" && ruleProto != proto {
			continue
		}
		if r.FromPort >= 0 && !(port >= r.FromPort && port <= r.ToPort) {
			continue
		}
		switch r.PeerType {
		case "cidr":
			prefix, err := netip.ParsePrefix(r.Peer)
			if err == nil && prefix.Contains(addr) {
				return true
			}
		case "security_group":
			for _, psg := range peerSGs {
				if psg == r.Peer {
					return true
				}
			}
		}
	}
	return false
}
