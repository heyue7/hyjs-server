package utils

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"

	"resty.dev/v3"
)

type LB struct {
	LBServers      []string
	InitialServers []string
	LastSelected   string
	LoadBalancer   resty.LoadBalancer
	serversVersion uint64
	mu             *sync.Mutex
	healthStop     chan struct{}
	healthRunning  bool
	retryBaseURL   string
	retryLeft      int
	retryUntil     time.Time
}

type LBOption func(*LB)

func (lb *LB) BuildLoadBalancerForKey(key string) resty.LoadBalancer {
	if lb == nil || lb.LoadBalancer == nil {
		return nil
	}

	switch lb.LoadBalancer.(type) {
	case *hashLB:
		return newHashLB(lb, key)
	default:
		return lb.LoadBalancer
	}
}

func NewLB(servers []string, opts ...LBOption) *LB {
	normalized, err := normalizeBaseURLs(servers)
	if err != nil || len(normalized) == 0 {
		normalized = copyStrings(servers)
	}
	lb := &LB{
		LBServers:      normalized,
		InitialServers: copyStrings(normalized),
		serversVersion: 1,
		mu:             &sync.Mutex{},
		healthStop:     make(chan struct{}),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(lb)
		}
	}
	if lb.LoadBalancer == nil {
		LBRandom()(lb)
	}
	lb.startHealthCheck()
	return lb
}

func LBRandom() LBOption {
	return func(lb *LB) {
		if lb == nil {
			return
		}
		lb.LoadBalancer = newRandomLB(lb)
	}
}

func LBRoundRobin() LBOption {
	return func(lb *LB) {
		if lb == nil {
			return
		}
		lb.LoadBalancer = newRoundRobinLB(lb)
	}
}

func LBHash(key string) LBOption {
	return func(lb *LB) {
		if lb == nil {
			return
		}
		lb.LoadBalancer = newHashLB(lb, key)
	}
}

func (lb *LB) startHealthCheck() {
	lb.mu.Lock()
	if lb.healthRunning {
		lb.mu.Unlock()
		return
	}
	lb.healthRunning = true
	stopCh := lb.healthStop
	lb.mu.Unlock()

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				lb.refreshServers()
			case <-stopCh:
				return
			}
		}
	}()
}

func (lb *LB) StopHealthCheck() {
	lb.mu.Lock()
	if !lb.healthRunning {
		lb.mu.Unlock()
		return
	}
	lb.healthRunning = false
	close(lb.healthStop)
	lb.healthStop = make(chan struct{})
	lb.mu.Unlock()
}

func (lb *LB) refreshServers() {
	current := lb.snapshotServers()
	initial := lb.snapshotInitialServers()

	results := lb.probeServers(append(current, initial...))

	aliveSet := make(map[string]struct{}, len(results))
	for srv, ok := range results {
		if ok {
			aliveSet[srv] = struct{}{}
		}
	}

	newList := make([]string, 0, len(aliveSet))
	initialSet := make(map[string]struct{}, len(initial))
	for _, srv := range initial {
		initialSet[srv] = struct{}{}
		if _, ok := aliveSet[srv]; ok {
			newList = append(newList, srv)
		}
	}

	for _, srv := range current {
		if _, ok := aliveSet[srv]; !ok {
			continue
		}
		if _, ok := initialSet[srv]; ok {
			continue
		}
		newList = append(newList, srv)
	}

	lb.updateServers(newList)
}

func (lb *LB) snapshotServers() []string {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return copyStrings(lb.LBServers)
}

func (lb *LB) snapshotInitialServers() []string {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return copyStrings(lb.InitialServers)
}

func (lb *LB) probeServers(servers []string) map[string]bool {
	result := make(map[string]bool, len(servers))
	seen := make(map[string]struct{}, len(servers))
	for _, srv := range servers {
		if _, ok := seen[srv]; ok {
			continue
		}
		seen[srv] = struct{}{}
		result[srv] = probeServer(srv)
	}
	return result
}

func probeServer(server string) bool {
	urlStr := normalizeProbeURL(server)
	ctx, cancel := context.WithTimeout(context.TODO(), 800*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", urlStr)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "200"
}

func normalizeProbeURL(server string) string {
	if !strings.HasPrefix(server, "http://") && !strings.HasPrefix(server, "https://") {
		server = "http://" + server
	}
	u, err := url.Parse(server)
	if err != nil {
		return server
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/health"
	}
	return u.String()
}

func copyStrings(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (lb *LB) updateServers(servers []string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	if equalStrings(lb.LBServers, servers) {
		return
	}
	lb.LBServers = servers
	lb.serversVersion++
}

func (lb *LB) currentServersVersion() uint64 {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return lb.serversVersion
}

func normalizeBaseURLs(baseURLs []string) ([]string, error) {
	normalized := make([]string, 0, len(baseURLs))
	for _, raw := range baseURLs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "://") {
			raw = "http://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil {
			return nil, err
		}
		u.Path = ""
		u.RawQuery = ""
		normalized = append(normalized, strings.TrimRight(u.String(), "/"))
	}
	return normalized, nil
}

type roundRobinLB struct {
	parent *LB
	mu     sync.Mutex
	idx    int
}

func newRoundRobinLB(parent *LB) *roundRobinLB {
	return &roundRobinLB{parent: parent}
}

func (lb *roundRobinLB) NextWithContext(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	return lb.parent.nextWithRetry(func(servers []string) (string, error) {
		lb.mu.Lock()
		defer lb.mu.Unlock()
		baseURL := servers[lb.idx%len(servers)]
		lb.idx = (lb.idx + 1) % len(servers)
		return baseURL, nil
	})
}

func (lb *roundRobinLB) Feedback(f *resty.RequestFeedback) { lb.parent.handleFeedback(f) }
func (lb *roundRobinLB) Close() error                      { return nil }

type randomLB struct {
	parent *LB
	mu     sync.Mutex
	rand   *rand.Rand
}

func newRandomLB(parent *LB) *randomLB {
	return &randomLB{
		parent: parent,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (lb *randomLB) NextWithContext(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	return lb.parent.nextWithRetry(func(servers []string) (string, error) {
		lb.mu.Lock()
		defer lb.mu.Unlock()
		return servers[lb.rand.Intn(len(servers))], nil
	})
}

func (lb *randomLB) Feedback(f *resty.RequestFeedback) { lb.parent.handleFeedback(f) }
func (lb *randomLB) Close() error                      { return nil }

type hashLB struct {
	parent *LB
	key    string
}

func newHashLB(parent *LB, key string) *hashLB {
	return &hashLB{parent: parent, key: key}
}

func (lb *hashLB) NextWithContext(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	return lb.parent.nextWithRetry(func(servers []string) (string, error) {
		if lb.key == "" {
			return servers[0], nil
		}
		idx := int(hashString(lb.key) % uint32(len(servers)))
		return servers[idx], nil
	})
}

func (lb *hashLB) Feedback(f *resty.RequestFeedback) { lb.parent.handleFeedback(f) }
func (lb *hashLB) Close() error                      { return nil }

func hashString(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

func (lb *LB) setLastSelected(server string) {
	lb.mu.Lock()
	lb.LastSelected = server
	lb.mu.Unlock()
}

func (lb *LB) nextWithRetry(pick func([]string) (string, error)) (string, error) {
	if baseURL, ok := lb.consumeRetry(); ok {
		lb.setLastSelected(baseURL)
		return baseURL, nil
	}

	servers := lb.snapshotServers()
	if len(servers) == 0 {
		return "", fmt.Errorf("load balancer: no base urls")
	}

	baseURL, err := pick(servers)
	if err != nil {
		return "", err
	}
	lb.setLastSelected(baseURL)
	return baseURL, nil
}

func (lb *LB) consumeRetry() (string, bool) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	if lb.retryLeft <= 0 || lb.retryBaseURL == "" {
		return "", false
	}
	if time.Now().After(lb.retryUntil) {
		lb.retryLeft = 0
		lb.retryBaseURL = ""
		return "", false
	}
	if !containsString(lb.LBServers, lb.retryBaseURL) {
		lb.retryLeft = 0
		lb.retryBaseURL = ""
		return "", false
	}
	lb.retryLeft--
	return lb.retryBaseURL, true
}

func (lb *LB) handleFeedback(f *resty.RequestFeedback) {
	if f == nil {
		return
	}
	if f.Success {
		lb.clearRetry()
		return
	}
	if f.BaseURL == "" {
		return
	}

	lb.mu.Lock()
	lb.retryBaseURL = f.BaseURL
	lb.retryLeft = 3
	lb.retryUntil = time.Now().Add(2 * time.Second)
	lb.mu.Unlock()
}

func (lb *LB) clearRetry() {
	lb.mu.Lock()
	lb.retryLeft = 0
	lb.retryBaseURL = ""
	lb.mu.Unlock()
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
