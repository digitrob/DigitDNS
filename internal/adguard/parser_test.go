package adguard

import (
	"strings"
	"testing"
)

const sampleConfig = `# ==============================================================================
# GENERAL & FALLBACK RESOLVERS
# ==============================================================================
https://dns.cloudflare.com/dns-query
tls://one.one.one.one
1.1.1.1
# ==============================================================================
# UK & US STREAMING SERVICES (Smart DNS Routing)
# ==============================================================================
[/abcnews.com/adobetm.com/akamaiedge.net/akamaihd.net/akamized.net/bbc.co.uk/bbc.com/*.bbci.co.uk/bidi.net.uk/c4assets.com/channel4.com/channel5.com/cnn.com/conviva.com/espncdn.com/foxnews.com/fwmrm.net/itv.com/nbcnews.com/semfs.engsvc.go.com/]https://norwich-vps.duckdns.org:8443/dns-query
# ==============================================================================
# REGIONAL & LOCAL GATEWAY ROUTING
# ==============================================================================
[/kommersant.ru/nalog.ru/]77.88.8.8
# Disabled / Local Gateway Drafts
#[/claude.ai/lrt.lt/]192.168.2.1:53
# ==============================================================================
# AI ECOSYSTEM: OPENAI & CHATGPT
# ==============================================================================
[/chatgpt.com/oaistatic.com/oaiusercontent.com/openai.com/openaiapi-site.azureedge.net/]https://norwich-vps.duckdns.org:8443/dns-query
# ==============================================================================
# AI ECOSYSTEM: GOOGLE AI, GEMINI & NOTEBOOKLM
# ==============================================================================
[/aisandbox-pa.googleapis.com/aistudio.google.com/aitestkitchen.withgoogle.com/alkalimakersuite-pa.clients6.google.com/assistant-s3-pa.googleapis.com/gemini.google.com/generativelanguage.googleapis.com/jules.google.com/labs.google/notebooklm.google/notebooklm.google.com/o.pki.goog/proactivebackend-pa.googleapis.com/robinfrontend-pa.googleapis.com/stitch.withgoogle.com/webchannel-alkalimakersuite-pa.clients6.google.com/signaler-pa.googleapis.com/android.googleapis.com/play.googleapis.com/clients4.google.com/clients6.google.com/antigravity.google/cloudcode-pa.googleapis.com/flow.google.com/clients6.google.com/signaler-pa.googleapis.com/www.google.com/flow-content.google/notebooklm-pa.googleapis.com/labstailwind.pa.googleapis.com/]https://norwich-vps.duckdns.org:8443/dns-query
# ==============================================================================
# AI ECOSYSTEM: ANTHROPIC & CLAUDE
# ==============================================================================
[/anthropic.com/claude.ai/claude.com/claudemcpclient.com/]https://norwich-vps.duckdns.org:8443/dns-query
# ==============================================================================
# AI ECOSYSTEM: xAI (GROK)
# ==============================================================================
[/grok.com/grok.x.com/x.ai/]https://norwich-vps.duckdns.org:8443/dns-query
# ==============================================================================
# AI ECOSYSTEM: OTHER UTILITIES
# ==============================================================================
[/flowmusic.app/]37.230.192.51 164.5.224.81
[/greateranglia.co.uk/*.thetrainline.com/*.trainline.com/ltglink.lt/lrt.lt/*.etsy.com/*.etsy.co.uk/stacksocial.com/reddit.com/weather.com/truthsocial.com/waveterm.dev/opencode.ai/upwork.com/miro.com/flowmusic.app/platform.reflection.ai/myqnapcloud.io/]quic://norwich-vps.duckdns.org:853
#steampowered.com/steamcommunity.com/steamstatic.com/steamcontent.com/steamusercontent.com/steamserver.net/steamgames.com/steam-chat.com/steamcdn-a.akamaihd.net
`

func byValue(patterns []Pattern, value string) (Pattern, bool) {
	for _, p := range patterns {
		if p.String() == value {
			return p, true
		}
	}
	return Pattern{}, false
}

func TestParseSampleConfig(t *testing.T) {
	result := Parse(sampleConfig, []string{"norwich-vps.duckdns.org", "195.58.134.162"})
	if len(result.Groups) != 8 {
		t.Fatalf("parsed %d groups, want 8", len(result.Groups))
	}
	if len(result.Patterns) == 0 {
		t.Fatal("no patterns parsed")
	}

	wanted := []string{
		"bbc.co.uk", "*.bbci.co.uk", "kommersant.ru", "chatgpt.com",
		"www.google.com", "anthropic.com", "grok.com", "flowmusic.app",
		"*.etsy.com", "lrt.lt",
	}
	for _, value := range wanted {
		if _, ok := byValue(result.Patterns, value); !ok {
			t.Errorf("missing normalized pattern %q", value)
		}
	}

	for _, group := range result.Groups {
		if group.Target == "192.168.2.1:53" {
			t.Error("disabled local draft group was imported")
		}
		if group.Target == "77.88.8.8" && group.TargetIsSelf {
			t.Error("Yandex group was marked as this server")
		}
	}
	if got := result.Groups[1].Label; got != "REGIONAL & LOCAL GATEWAY ROUTING" {
		t.Errorf("group label = %q", got)
	}

	clients6 := 0
	for _, p := range result.Patterns {
		if p.Value == "clients6.google.com" {
			clients6++
		}
	}
	if clients6 != 1 {
		t.Errorf("clients6.google.com imported %d times, want 1", clients6)
	}

	joinedWarnings := strings.Join(result.Warnings, "\n")
	for _, warning := range []string{"www.google.com", "play.googleapis.com", "clients6.google.com"} {
		if !strings.Contains(joinedWarnings, warning) {
			t.Errorf("missing broad-domain warning for %s", warning)
		}
	}
	if result.Checksum == "" {
		t.Error("no source checksum")
	}
}

func TestNormalizeTokenMarksBroadDomains(t *testing.T) {
	broad, ok := NormalizeToken("www.google.com")
	if !ok || !broad.Broad {
		t.Fatalf("www.google.com = %#v, want broad", broad)
	}
	ordinary, ok := NormalizeToken("bbc.co.uk")
	if !ok || ordinary.Broad {
		t.Fatalf("bbc.co.uk = %#v, want ordinary", ordinary)
	}
}

func TestSelfDetectionIgnoresSchemeAndPort(t *testing.T) {
	self := selfHostSet([]string{"NORWICH-VPS.DUCKDNS.ORG."})
	for _, target := range []string{
		"https://norwich-vps.duckdns.org:8443/dns-query",
		"quic://norwich-vps.duckdns.org:853",
	} {
		if !IsSelfTarget(target, self) {
			t.Errorf("%q was not recognized as this server", target)
		}
	}
	if IsSelfTarget("https://example.net/dns-query", self) {
		t.Error("foreign upstream was recognized as this server")
	}
}

func TestCoverageUnderstandsWildcardSemantics(t *testing.T) {
	plain, _ := NormalizeToken("bbci.co.uk")
	wild, _ := NormalizeToken("*.bbci.co.uk")
	child, _ := NormalizeToken("www.bbci.co.uk")
	other, _ := NormalizeToken("bbc.co.uk")

	if !Covers(plain, child) {
		t.Error("plain rule should cover its subdomain")
	}
	if !Covers(plain, wild) {
		t.Error("plain rule should cover a wildcard under it")
	}
	if Covers(wild, plain) {
		t.Error("wildcard must not cover its own parent")
	}
	if !Covers(wild, child) {
		t.Error("wildcard should cover a strict subdomain")
	}
	if Overlaps(plain, wild) != true {
		t.Error("plain and wildcard under it should overlap")
	}
	if Overlaps(plain, other) {
		t.Error("sibling domains should not overlap")
	}
}

func TestGenerateManagedBlock(t *testing.T) {
	a, _ := NormalizeToken("b.example")
	b, _ := NormalizeToken("a.example")
	block := GenerateManagedBlock([]Pattern{a, b}, "https://norwich-vps.duckdns.org:8443/dns-query")
	for _, line := range []string{
		ManagedBlockStart,
		"[/a.example/b.example]https://norwich-vps.duckdns.org:8443/dns-query",
		ManagedBlockEnd,
	} {
		if !strings.Contains(block, line) {
			t.Errorf("generated block missing %q:\n%s", line, block)
		}
	}
}
