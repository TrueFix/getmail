package email

import (
	"fmt"
	"log"
	"net"
	"strings"
)

// IpRange holds IPv4 and IPv6 CIDR ranges.
type SPFRecord struct {
	Domain string
	ipv4   *map[string]struct{}
	ipv6   *map[string]struct{}
}

func NewSPFRecord(domain string) *SPFRecord {
	return &SPFRecord{
		Domain: domain,
		ipv4:   &map[string]struct{}{},
		ipv6:   &map[string]struct{}{},
	}
}

func (r *SPFRecord) Add(ipv4 []string, ipv6 []string) {
	for _, ip := range ipv4 {
		(*r.ipv4)[ip] = struct{}{}
	}
	for _, ip := range ipv6 {
		(*r.ipv6)[ip] = struct{}{}
	}
}

func (r *SPFRecord) CheckIfContains(ip net.IP) bool {
	for cidr := range *r.ipv4 {
		if _, ipnet, err := net.ParseCIDR(cidr); err == nil && ipnet.Contains(ip) {
			return true
		}
	}
	for cidr := range *r.ipv6 {
		if _, ipnet, err := net.ParseCIDR(cidr); err == nil && ipnet.Contains(ip) {
			return true
		}
	}
	return false
}

func (r *SPFRecord) CheckSPF(domain string, ip net.IP) (bool, error) {
	err := r.fetchSPFNetworks(domain, make(map[string]struct{}))
	if err != nil {
		return false, fmt.Errorf("error fetching SPF records: %w", err)
	}
	return r.CheckIfContains(ip), nil
}

// fetchSPFNetworks recursively fetches and resolves SPF records for a domain.
func (r *SPFRecord) fetchSPFNetworks(domain string, visited map[string]struct{}) error {
	if _, seen := visited[domain]; seen {
		return nil // Avoid recursion
	}
	visited[domain] = struct{}{}

	txtRecords, err := net.LookupTXT(domain)
	if err != nil {
		return fmt.Errorf("TXT lookup failed for %s: %w", domain, err)
	}

	var allIPv4, allIPv6 []string
	for _, record := range txtRecords {
		if !strings.HasPrefix(record, "v=spf1") {
			continue
		}

		ipv4, ipv6, includes, redirects := r.parseSPFRecord(record)
		allIPv4 = append(allIPv4, ipv4...)
		allIPv6 = append(allIPv6, ipv6...)

		for _, inc := range includes {
			err := r.fetchSPFNetworks(inc, visited)
			if err != nil {
				log.Printf("Include failed for %s: %v", inc, err)
				continue
			}
		}

		for _, red := range redirects {
			err := r.fetchSPFNetworks(red, visited)
			if err != nil {
				log.Printf("Redirect failed for %s: %v", red, err)
				continue
			}
		}
	}
	r.Add(allIPv4, allIPv6)

	return nil

}

// parseSPFRecord extracts IPs, includes, and redirects from an SPF record.
func (r *SPFRecord) parseSPFRecord(spf string) (ipv4 []string, ipv6 []string, includes []string, redirects []string) {
	spf = r.normalizeSPF(spf)
	parts := strings.Fields(spf)

	for _, part := range parts {
		switch {
		case strings.HasPrefix(part, "ip4:"):
			ip := strings.TrimPrefix(part, "ip4:")
			ipv4 = append(ipv4, ip)
		case strings.HasPrefix(part, "ip6:"):
			ip := strings.TrimPrefix(part, "ip6:")
			ipv6 = append(ipv6, ip)
		case strings.HasPrefix(part, "include:"):
			domain := strings.TrimPrefix(part, "include:")
			includes = append(includes, domain)
		case strings.HasPrefix(part, "redirect="):
			domain := strings.TrimPrefix(part, "redirect=")
			redirects = append(redirects, domain)
		}
	}
	return
}

// normalizeSPF removes the 'v=spf1' prefix and any terminal 'all' mechanism.
func (r *SPFRecord) normalizeSPF(spf string) string {
	spf = strings.TrimPrefix(spf, "v=spf1")
	spf = strings.TrimSpace(spf)
	spf = strings.TrimSuffix(spf, "~all")
	spf = strings.TrimSuffix(spf, "-all")
	spf = strings.TrimSuffix(spf, "+all")
	spf = strings.TrimSuffix(spf, "?all")
	return strings.TrimSpace(spf)
}
