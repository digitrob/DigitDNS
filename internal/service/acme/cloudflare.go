package acme

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// Cloudflare is the DNS-01 provider for zones hosted on Cloudflare. It uses a
// scoped API token (Zone:Zone:Read + Zone:DNS:Edit). The token is never logged.
type Cloudflare struct {
	Token string
	// Zone is the DNS zone name, e.g. "example.com". Empty makes the provider
	// discover it from the certificate name by matching the longest Cloudflare
	// zone suffix.
	Zone string
	HTTP *http.Client

	mu      sync.Mutex
	records map[string]cloudflareRecord
}

type cloudflareRecord struct {
	ZoneID   string
	RecordID string
}

// NewCloudflare builds a provider with a bounded HTTP client. A nil or empty
// token is refused by Present rather than silently trying an anonymous call.
func NewCloudflare(token, zone string) *Cloudflare {
	return &Cloudflare{
		Token:   strings.TrimSpace(token),
		Zone:    strings.TrimSpace(zone),
		HTTP:    &http.Client{Timeout: 20 * time.Second},
		records: map[string]cloudflareRecord{},
	}
}

type cfResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result json.RawMessage `json:"result"`
}

type cfZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cfRecord struct {
	ID string `json:"id"`
}

// Present creates the TXT record and waits until public resolvers see it.
func (c *Cloudflare) Present(ctx context.Context, fqdn, value string) error {
	if c.Token == "" {
		return fmt.Errorf("a Cloudflare API token is required")
	}
	zoneID, err := c.zoneID(ctx, fqdn)
	if err != nil {
		return err
	}
	body := map[string]any{
		"type":    "TXT",
		"name":    fqdn,
		"content": value,
		"ttl":     120,
	}
	var out cfRecord
	if err := c.do(ctx, http.MethodPost, "/zones/"+zoneID+"/dns_records", body, &out); err != nil {
		return fmt.Errorf("create TXT %s: %w", fqdn, err)
	}
	key := fqdn + "\x00" + value
	c.mu.Lock()
	c.records[key] = cloudflareRecord{ZoneID: zoneID, RecordID: out.ID}
	c.mu.Unlock()

	if err := waitForTXT(ctx, fqdn, value, 2*time.Minute); err != nil {
		// A failed propagation is not a reason to leave a stale record behind.
		_ = c.CleanUp(context.Background(), fqdn, value)
		return err
	}
	return nil
}

// CleanUp removes the TXT record. It is idempotent: an unknown pair is a no-op.
func (c *Cloudflare) CleanUp(ctx context.Context, fqdn, value string) error {
	key := fqdn + "\x00" + value
	c.mu.Lock()
	rec, ok := c.records[key]
	if ok {
		delete(c.records, key)
	}
	c.mu.Unlock()
	if !ok {
		return nil
	}
	// Cloudflare's delete endpoint needs the zone id; the record id alone is
	// not enough. A 404 means someone already removed it.
	err := c.do(ctx, http.MethodDelete, "/zones/"+rec.ZoneID+"/dns_records/"+rec.RecordID, nil, nil)
	if err != nil && !strings.Contains(err.Error(), "404") {
		return fmt.Errorf("delete TXT %s: %w", fqdn, err)
	}
	return nil
}

// zoneID resolves the Cloudflare zone for a challenge name. With an explicit
// Zone it is one lookup; otherwise the account's zones are scanned for the
// longest suffix match, which handles co.uk-style public suffixes correctly.
func (c *Cloudflare) zoneID(ctx context.Context, fqdn string) (string, error) {
	if c.Zone != "" {
		var zones []cfZone
		if err := c.do(ctx, http.MethodGet, "/zones?name="+url.QueryEscape(c.Zone), nil, &zones); err != nil {
			return "", fmt.Errorf("look up zone %s: %w", c.Zone, err)
		}
		if len(zones) == 0 {
			return "", fmt.Errorf("Cloudflare zone %s was not found for this token", c.Zone)
		}
		return zones[0].ID, nil
	}

	var zones []cfZone
	if err := c.do(ctx, http.MethodGet, "/zones?per_page=50", nil, &zones); err != nil {
		return "", fmt.Errorf("list Cloudflare zones: %w", err)
	}
	name := strings.TrimPrefix(fqdn, "_acme-challenge.")
	best := ""
	bestID := ""
	for _, z := range zones {
		if name == z.Name || strings.HasSuffix(name, "."+z.Name) {
			if len(z.Name) > len(best) {
				best = z.Name
				bestID = z.ID
			}
		}
	}
	if bestID == "" {
		return "", fmt.Errorf("no Cloudflare zone matches %s; enter the zone explicitly", name)
	}
	return bestID, nil
}

func (c *Cloudflare) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.cloudflare.com/client/v4"+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "DigitDNS-ACME/1")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	var envelope cfResponse
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("Cloudflare returned HTTP %d with an unreadable body", res.StatusCode)
	}
	if !envelope.Success || res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := fmt.Sprintf("Cloudflare returned HTTP %d", res.StatusCode)
		if len(envelope.Errors) > 0 {
			msg += ": " + envelope.Errors[0].Message
		}
		return fmt.Errorf("%s", msg)
	}
	if out != nil && len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, out); err != nil {
			return err
		}
	}
	return nil
}

// waitForTXT polls two public resolvers until both return the expected value.
// Cloudflare's API acknowledges a record before every resolver has it; asking
// the CA to validate too early is the usual DNS-01 failure.
func waitForTXT(ctx context.Context, fqdn, value string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	resolvers := []string{"1.1.1.1:53", "8.8.8.8:53"}
	client := &dns.Client{Net: "udp", Timeout: 4 * time.Second}
	for {
		seen := 0
		for _, resolver := range resolvers {
			msg := new(dns.Msg)
			msg.SetQuestion(dns.Fqdn(fqdn), dns.TypeTXT)
			res, _, err := client.ExchangeContext(ctx, msg, resolver)
			if err == nil {
				for _, rr := range res.Answer {
					if txt, ok := rr.(*dns.TXT); ok && strings.Join(txt.Txt, "") == value {
						seen++
						break
					}
				}
			}
		}
		if seen == len(resolvers) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the TXT record for %s did not appear on public resolvers within %s", fqdn, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}
