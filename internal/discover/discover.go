// Package discover finds Samsung TVs on the local network via mDNS and SSDP.
package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/koron/go-ssdp"
)

// TV is a Samsung smart TV discovered on the LAN.
type TV struct {
	IP             string
	Name           string
	Model          string
	FrameTVSupport bool
	Hostname       string   // mDNS hostname when known (e.g. Samsung.local.)
	Sources        []string // "mdns", "ssdp", "scan"
}

// Options configures network discovery.
type Options struct {
	Timeout time.Duration
	// MDNS enables DNS-SD browsing (_samsungmsf._tcp, _samsungctl._tcp).
	MDNS bool
	// SSDP enables UPnP SSDP scanning (more reliable on Samsung TVs).
	SSDP bool
	// SubnetScan probes port 8001 on the local subnet when multicast discovery finds nothing.
	SubnetScan bool
}

// DefaultOptions returns sensible discovery defaults.
func DefaultOptions() Options {
	return Options{
		Timeout:    8 * time.Second,
		MDNS:       true,
		SSDP:       true,
		SubnetScan: true,
	}
}

// Find scans the LAN for Samsung TVs and probes each candidate IP.
func Find(ctx context.Context, opts Options) ([]TV, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultOptions().Timeout
	}

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	byIP := make(map[string]*TV)
	var mu sync.Mutex

	var wg sync.WaitGroup

	if opts.MDNS {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mergeIPs(ctx, &mu, byIP, browseMDNS(ctx), "mdns")
		}()
	}

	if opts.SSDP {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mergeIPs(ctx, &mu, byIP, searchSSDP(ctx), "ssdp")
		}()
	}

	if opts.SubnetScan {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mergeIPs(ctx, &mu, byIP, scanSubnets(ctx), "scan")
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}

	if len(byIP) == 0 {
		return nil, fmt.Errorf("no Samsung TVs found on the LAN (try discover with -v, or set --host)")
	}

	probeCtx, probeCancel := context.WithTimeout(ctx, 5*time.Second)
	defer probeCancel()

	out := make([]TV, 0, len(byIP))
	for ip, tv := range byIP {
		if info, err := probeDevice(probeCtx, ip); err == nil {
			tv.Name = info.Name
			tv.Model = info.ModelName
			tv.FrameTVSupport = info.FrameTVSupport == "true"
			if info.IP != "" {
				tv.IP = info.IP
			}
		}
		out = append(out, *tv)
	}

	sortTVs(out)
	return out, nil
}

// PickFrameTV returns the best TV for frame-it: a Frame TV if possible, else the only TV.
func PickFrameTV(tvs []TV) (TV, error) {
	if len(tvs) == 0 {
		return TV{}, fmt.Errorf("no TVs to pick from")
	}
	if len(tvs) == 1 {
		return tvs[0], nil
	}

	frame := make([]TV, 0, len(tvs))
	for _, tv := range tvs {
		if tv.FrameTVSupport {
			frame = append(frame, tv)
		}
	}
	if len(frame) == 1 {
		return frame[0], nil
	}
	if len(frame) > 1 {
		return TV{}, fmt.Errorf("multiple Frame TVs found — use discover and pass --host")
	}

	return TV{}, fmt.Errorf("multiple Samsung TVs found — use discover and pass --host")
}

func mergeIPs(ctx context.Context, mu *sync.Mutex, byIP map[string]*TV, ips map[string]string, source string) {
	mu.Lock()
	defer mu.Unlock()

	for ip, hostname := range ips {
		if ctx.Err() != nil {
			return
		}
		tv, ok := byIP[ip]
		if !ok {
			tv = &TV{IP: ip, Sources: []string{source}}
			byIP[ip] = tv
		} else {
			tv.Sources = appendUnique(tv.Sources, source)
		}
		if hostname != "" && tv.Hostname == "" {
			tv.Hostname = hostname
		}
	}
}

func appendUnique(list []string, v string) []string {
	for _, s := range list {
		if s == v {
			return list
		}
	}
	return append(list, v)
}

var mdnsServiceTypes = []string{
	"_samsungmsf._tcp", // Samsung Smart View
	"_samsungctl._tcp", // Samsung remote control
}

var ssdpSearchTargets = []string{
	ssdp.All,
	"urn:dial-multiscreen-org:service:dial:1",
	"urn:samsung.com:device:RemoteControlReceiver:1",
}

func browseMDNS(ctx context.Context) map[string]string {
	out := make(map[string]string)

	var resolverOpts []zeroconf.ClientOption
	if ifaces, err := lanInterfaces(); err == nil && len(ifaces) > 0 {
		resolverOpts = append(resolverOpts, zeroconf.SelectIfaces(ifaces))
	}

	resolver, err := zeroconf.NewResolver(resolverOpts...)
	if err != nil {
		return out
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, service := range mdnsServiceTypes {
		wg.Add(1)
		go func(service string) {
			defer wg.Done()

			entries := make(chan *zeroconf.ServiceEntry)
			go func() {
				_ = resolver.Browse(ctx, service, "local.", entries)
			}()

			for entry := range entries {
				if ctx.Err() != nil {
					return
				}
				host := strings.TrimSuffix(entry.HostName, ".")
				for _, ip := range entry.AddrIPv4 {
					mu.Lock()
					if _, ok := out[ip.String()]; !ok {
						out[ip.String()] = host
					}
					mu.Unlock()
				}
			}
		}(service)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}

	return out
}

func searchSSDP(ctx context.Context) map[string]string {
	out := make(map[string]string)

	ifaces, err := lanInterfaces()
	if err != nil || len(ifaces) == 0 {
		return out
	}

	waitSec := ssdpWaitSeconds(ctx)
	type searchResult struct {
		services []ssdp.Service
	}

	prev := ssdp.Interfaces
	ssdp.Interfaces = ifaces
	defer func() { ssdp.Interfaces = prev }()

	results := make(chan searchResult, len(ssdpSearchTargets))
	for _, target := range ssdpSearchTargets {
		go func(target string) {
			services, err := ssdp.Search(target, waitSec, "")
			if err != nil {
				results <- searchResult{}
				return
			}
			results <- searchResult{services: services}
		}(target)
	}

	for range ssdpSearchTargets {
		select {
		case <-ctx.Done():
			return out
		case result := <-results:
			for _, svc := range result.services {
				if !isSamsungSSDP(svc) {
					continue
				}
				ip := hostFromLocation(svc.Location)
				if ip == "" {
					continue
				}
				out[ip] = ""
			}
		}
	}

	return out
}

func isSamsungSSDP(svc ssdp.Service) bool {
	for _, field := range []string{svc.Type, svc.Server, svc.USN, svc.Location} {
		if strings.Contains(strings.ToLower(field), "samsung") {
			return true
		}
	}
	return false
}

func ssdpWaitSeconds(ctx context.Context) int {
	waitSec := 3
	if deadline, ok := ctx.Deadline(); ok {
		if sec := int(time.Until(deadline) / time.Second); sec >= 1 && sec < waitSec {
			waitSec = sec
		}
	}
	return waitSec
}

func lanInterfaces() ([]net.Interface, error) {
	ifis, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var out []net.Interface
	for _, ifi := range ifis {
		if ifi.Flags&net.FlagUp == 0 ||
			ifi.Flags&net.FlagMulticast == 0 ||
			ifi.Flags&net.FlagBroadcast == 0 ||
			ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := ipv4FromAddr(addr)
			if ip != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
				out = append(out, ifi)
				break
			}
		}
	}
	return out, nil
}

func lanSearchAddrs() ([]string, error) {
	ifis, err := lanInterfaces()
	if err != nil {
		return nil, err
	}

	var out []string
	for _, ifi := range ifis {
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := ipv4FromAddr(addr)
			if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
				continue
			}
			out = append(out, net.JoinHostPort(ip.String(), "0"))
		}
	}
	if len(out) == 0 {
		return []string{""}, nil
	}
	return out, nil
}

func ipv4FromAddr(addr net.Addr) net.IP {
	switch a := addr.(type) {
	case *net.IPNet:
		return a.IP.To4()
	case *net.IPAddr:
		return a.IP.To4()
	}
	return nil
}

func scanSubnets(ctx context.Context) map[string]string {
	out := make(map[string]string)

	nets, err := localSubnets()
	if err != nil || len(nets) == 0 {
		return out
	}

	type job struct{ ip string }
	jobs := make(chan job, 256)
	var wg sync.WaitGroup
	var mu sync.Mutex

	workers := 64
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if ctx.Err() != nil {
					return
				}
				if isSamsungAPI(ctx, j.ip) {
					mu.Lock()
					out[j.ip] = ""
					mu.Unlock()
				}
			}
		}()
	}

scanLoop:
	for _, network := range nets {
		ip := network.IP.Mask(network.Mask)
		for ip := ip.Mask(network.Mask); network.Contains(ip); incIP(ip) {
			if ctx.Err() != nil {
				break scanLoop
			}
			host := ip.String()
			if ip.Equal(network.IP) {
				continue // skip network address
			}
			select {
			case jobs <- job{ip: host}:
			case <-ctx.Done():
				break scanLoop
			}
		}
	}
	close(jobs)
	wg.Wait()

	return out
}

func localSubnets() ([]*net.IPNet, error) {
	ifis, err := lanInterfaces()
	if err != nil {
		return nil, err
	}

	var out []*net.IPNet
	seen := make(map[string]struct{})

	for _, ifi := range ifis {
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil || ip4.IsLoopback() {
				continue
			}
			ones, bits := ipNet.Mask.Size()
			if ones == 0 || ones >= bits {
				continue
			}
			network := &net.IPNet{
				IP:   ip4.Mask(ipNet.Mask),
				Mask: ipNet.Mask,
			}
			key := network.String()
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, network)
		}
	}
	return out, nil
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func isSamsungAPI(ctx context.Context, ip string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, fmt.Sprintf("http://%s:8001/api/v2/", ip), nil)
	if err != nil {
		return false
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return false
	}

	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "samsung") || strings.Contains(lower, "frametvsupport")
}

func hostFromLocation(location string) string {
	if !strings.HasPrefix(location, "http://") && !strings.HasPrefix(location, "https://") {
		return ""
	}
	host := strings.TrimPrefix(location, "https://")
	host = strings.TrimPrefix(host, "http://")
	if i := strings.Index(host, "/"); i >= 0 {
		host = host[:i]
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

type deviceInfo struct {
	Name           string `json:"name"`
	ModelName      string `json:"modelName"`
	FrameTVSupport string `json:"FrameTVSupport"`
	IP             string `json:"ip"`
}

type deviceEnvelope struct {
	Device deviceInfo `json:"device"`
}

func probeDevice(ctx context.Context, ip string) (deviceInfo, error) {
	url := fmt.Sprintf("http://%s:8001/api/v2/", ip)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return deviceInfo{}, err
	}

	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return deviceInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return deviceInfo{}, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return deviceInfo{}, err
	}

	var envelope deviceEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return deviceInfo{}, err
	}

	return envelope.Device, nil
}

func sortTVs(tvs []TV) {
	// Frame TVs first, then by IP.
	for i := 0; i < len(tvs); i++ {
		for j := i + 1; j < len(tvs); j++ {
			if tvs[j].FrameTVSupport && !tvs[i].FrameTVSupport {
				tvs[i], tvs[j] = tvs[j], tvs[i]
			} else if tvs[i].IP > tvs[j].IP {
				tvs[i], tvs[j] = tvs[j], tvs[i]
			}
		}
	}
}
