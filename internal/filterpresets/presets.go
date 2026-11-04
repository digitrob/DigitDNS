// Package filterpresets is the curated catalogue of DNS blocklists shown in the
// Ad Blocking page. It stores source URLs only; list content is fetched by the
// filter service at runtime, never bundled into the binary.
package filterpresets

// Preset is one selectable list. Homepage points at the project page, URL at the
// raw list file, and License is the upstream project's stated licence (or "See
// upstream" where it is not machine-readable).
type Preset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Group       string `json:"group"`
	URL         string `json:"url"`
	Homepage    string `json:"homepage"`
	License     string `json:"license"`
	Description string `json:"description,omitempty"`
	Warning     string `json:"warning,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
	Base        bool   `json:"base,omitempty"`
}

const (
	GroupBase      = "base"
	GroupAds       = "ads"
	GroupSecurity  = "security"
	GroupTelemetry = "telemetry"
	GroupExtras    = "extras"
)

const hageziHome = "https://github.com/hagezi/dns-blocklists"

var presets = []Preset{
	// General ads + trackers. Exactly one should be selected as the base.
	{ID: "hagezi-pro", Name: "HaGeZi Pro", Group: GroupBase, Base: true, Recommended: true,
		URL:      "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/pro.txt",
		Homepage: hageziHome, License: "GPL-3.0", Description: "Balanced multi-purpose blocking; recommended starting point."},
	{ID: "hagezi-light", Name: "HaGeZi Light", Group: GroupBase, Base: true,
		URL:      "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/light.txt",
		Homepage: hageziHome, License: "GPL-3.0", Description: "Smaller, conservative general list."},
	{ID: "hagezi-pro-plus", Name: "HaGeZi Pro+", Group: GroupBase, Base: true,
		URL:      "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/pro.plus.txt",
		Homepage: hageziHome, License: "GPL-3.0", Description: "Stronger than Pro; more false-positive risk."},
	{ID: "hagezi-ultimate", Name: "HaGeZi Ultimate", Group: GroupBase, Base: true,
		URL:      "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/ultimate.txt",
		Homepage: hageziHome, License: "GPL-3.0", Description: "Maximum protection; highest false-positive risk."},
	{ID: "oisd-big", Name: "OISD Big", Group: GroupBase, Base: true,
		URL: "https://big.oisd.nl", Homepage: "https://oisd.nl", License: "See upstream",
		Description: "Large, widely used general ads and tracking list."},
	{ID: "oisd-small", Name: "OISD Small", Group: GroupBase, Base: true,
		URL: "https://small.oisd.nl", Homepage: "https://oisd.nl", License: "See upstream",
		Description: "Smaller OISD selection."},
	{ID: "adguard-dns", Name: "AdGuard DNS filter", Group: GroupBase, Base: true,
		URL:      "https://adguardteam.github.io/AdGuardSDNSFilter/Filters/filter.txt",
		Homepage: "https://github.com/AdguardTeam/AdGuardSDNSFilter", License: "GPL-3.0",
		Description: "AdGuard's DNS-oriented filter."},
	{ID: "1hosts-xtra", Name: "1Hosts Xtra", Group: GroupBase, Base: true,
		URL:      "https://raw.githubusercontent.com/badmojr/1Hosts/master/Xtra/adblock.txt",
		Homepage: "https://github.com/badmojr/1Hosts", License: "MPL-2.0",
		Description: "Aggressive 1Hosts tier (Pro is no longer published upstream)."},
	{ID: "1hosts-lite", Name: "1Hosts Lite", Group: GroupBase, Base: true,
		URL:      "https://raw.githubusercontent.com/badmojr/1Hosts/master/Lite/adblock.txt",
		Homepage: "https://github.com/badmojr/1Hosts", License: "MPL-2.0",
		Description: "Light 1Hosts tier."},
	{ID: "stevenblack", Name: "StevenBlack Unified", Group: GroupBase, Base: true,
		URL:      "https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts",
		Homepage: "https://github.com/StevenBlack/hosts", License: "MIT",
		Description: "Classic hosts-format unified list."},

	// Supplementary ad/tracker lists.
	{ID: "peter-lowe", Name: "Peter Lowe's Ad and tracking server list", Group: GroupAds,
		URL:      "https://pgl.yoyo.org/adservers/serverlist.php?hostformat=adblockplus&showintro=0&mimetype=plaintext",
		Homepage: "https://pgl.yoyo.org/adservers/", License: "See upstream"},
	{ID: "adaway", Name: "AdAway", Group: GroupAds,
		URL: "https://adaway.org/hosts.txt", Homepage: "https://adaway.org", License: "See upstream"},
	{ID: "dan-pollock", Name: "Dan Pollock's hosts", Group: GroupAds,
		URL:      "https://someonewhocares.org/hosts/zero/hosts",
		Homepage: "https://someonewhocares.org/hosts/", License: "See upstream"},
	{ID: "developerdan", Name: "Developer Dan Ads & Tracking Extended", Group: GroupAds,
		URL:      "https://www.github.developerdan.com/hosts/lists/ads-and-tracking-extended.txt",
		Homepage: "https://www.github.developerdan.com/hosts/", License: "See upstream"},
	{ID: "notracking", Name: "NoTracking", Group: GroupAds,
		URL:      "https://raw.githubusercontent.com/notracking/hosts-blocklists/master/adblock/adblock.txt",
		Homepage: "https://github.com/notracking/hosts-blocklists", License: "See upstream"},
	{ID: "disconnect-ads", Name: "Disconnect Ads", Group: GroupAds,
		URL:      "https://s3.amazonaws.com/lists.disconnect.me/simple_ad.txt",
		Homepage: "https://disconnect.me/trackerprotection", License: "GPL-3.0"},
	{ID: "disconnect-tracking", Name: "Disconnect Tracking", Group: GroupAds,
		URL:      "https://s3.amazonaws.com/lists.disconnect.me/simple_tracking.txt",
		Homepage: "https://disconnect.me/trackerprotection", License: "GPL-3.0"},
	{ID: "frogeye-first-party", Name: "Frogeye first-party trackers", Group: GroupAds,
		URL:      "https://hostfiles.frogeye.fr/firstparty-trackers-hosts.txt",
		Homepage: "https://hostfiles.frogeye.fr", License: "See upstream",
		Description: "Hosts for first-party tracker domains; pairs with CNAME-aware inspection."},
	{ID: "easylist", Name: "EasyList", Group: GroupAds,
		URL: "https://easylist.to/easylist/easylist.txt", Homepage: "https://easylist.to",
		License: "See upstream", Warning: "Browser list: most cosmetic/path rules cannot be applied to DNS."},
	{ID: "easyprivacy", Name: "EasyPrivacy", Group: GroupAds,
		URL: "https://easylist.to/easylist/easyprivacy.txt", Homepage: "https://easylist.to",
		License: "See upstream", Warning: "Browser list: most cosmetic/path rules cannot be applied to DNS."},

	// Malware / phishing / scams.
	{ID: "hagezi-tif", Name: "HaGeZi Threat Intelligence Feeds", Group: GroupSecurity,
		URL:      "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/tif.txt",
		Homepage: hageziHome, License: "GPL-3.0", Warning: "Large list (~38 MB); use one security list at a time."},
	{ID: "urlhaus", Name: "URLhaus", Group: GroupSecurity,
		URL:      "https://malware-filter.gitlab.io/malware-filter/urlhaus-filter-agh.txt",
		Homepage: "https://urlhaus.abuse.ch", License: "See upstream"},
	{ID: "phishing-army", Name: "Phishing Army Extended", Group: GroupSecurity,
		URL:      "https://phishing.army/download/phishing-army-adblock.txt",
		Homepage: "https://phishing.army", License: "See upstream"},
	{ID: "dandelionsprout", Name: "Dandelion Sprout Anti-Malware", Group: GroupSecurity,
		URL:      "https://raw.githubusercontent.com/DandelionSprout/adfilt/master/Alternate%20versions%20Anti-Malware%20List/AntiMalwareAdGuardHome.txt",
		Homepage: "https://github.com/DandelionSprout/adfilt", License: "NOASSERTION"},
	{ID: "scam-blocklist", Name: "Scam Blocklist", Group: GroupSecurity,
		URL:      "https://raw.githubusercontent.com/durablenapkin/scamblocklist/master/adguard.txt",
		Homepage: "https://github.com/durablenapkin/scamblocklist", License: "MIT"},
	{ID: "stalkerware", Name: "Stalkerware Indicators", Group: GroupSecurity,
		URL:      "https://raw.githubusercontent.com/AssoEchap/stalkerware-indicators/master/generated/hosts",
		Homepage: "https://github.com/AssoEchap/stalkerware-indicators", License: "See upstream"},

	// Device and vendor telemetry.
	{ID: "windows-spyblocker", Name: "WindowsSpyBlocker", Group: GroupTelemetry,
		URL:      "https://raw.githubusercontent.com/crazy-max/WindowsSpyBlocker/master/data/hosts/spy.txt",
		Homepage: "https://github.com/crazy-max/WindowsSpyBlocker", License: "MIT"},
	{ID: "smart-tv", Name: "Smart TV trackers", Group: GroupTelemetry,
		URL:      "https://raw.githubusercontent.com/Perflyst/PiHoleBlocklist/master/SmartTV-AGH.txt",
		Homepage: "https://github.com/Perflyst/PiHoleBlocklist", License: "MIT"},
	{ID: "android-trackers", Name: "Android trackers", Group: GroupTelemetry,
		URL:      "https://raw.githubusercontent.com/Perflyst/PiHoleBlocklist/master/android-tracking.txt",
		Homepage: "https://github.com/Perflyst/PiHoleBlocklist", License: "MIT"},
	{ID: "hagezi-native-amazon", Name: "HaGeZi native Amazon", Group: GroupTelemetry,
		URL: "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/native.amazon.txt", Homepage: hageziHome, License: "GPL-3.0"},
	{ID: "hagezi-native-apple", Name: "HaGeZi native Apple", Group: GroupTelemetry,
		URL: "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/native.apple.txt", Homepage: hageziHome, License: "GPL-3.0"},
	{ID: "hagezi-native-samsung", Name: "HaGeZi native Samsung", Group: GroupTelemetry,
		URL: "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/native.samsung.txt", Homepage: hageziHome, License: "GPL-3.0"},
	{ID: "hagezi-native-tiktok", Name: "HaGeZi native TikTok", Group: GroupTelemetry,
		URL: "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/native.tiktok.txt", Homepage: hageziHome, License: "GPL-3.0"},
	{ID: "hagezi-native-tiktok-extended", Name: "HaGeZi native TikTok Extended", Group: GroupTelemetry,
		URL: "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/native.tiktok.extended.txt", Homepage: hageziHome, License: "GPL-3.0"},
	{ID: "hagezi-native-winoffice", Name: "HaGeZi native Windows/Office", Group: GroupTelemetry,
		URL: "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/native.winoffice.txt", Homepage: hageziHome, License: "GPL-3.0"},
	{ID: "hagezi-native-huawei", Name: "HaGeZi native Huawei", Group: GroupTelemetry,
		URL: "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/native.huawei.txt", Homepage: hageziHome, License: "GPL-3.0"},
	{ID: "hagezi-native-lgwebos", Name: "HaGeZi native LG webOS", Group: GroupTelemetry,
		URL: "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/native.lgwebos.txt", Homepage: hageziHome, License: "GPL-3.0"},
	{ID: "hagezi-native-oppo-realme", Name: "HaGeZi native OPPO/Realme", Group: GroupTelemetry,
		URL: "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/native.oppo-realme.txt", Homepage: hageziHome, License: "GPL-3.0"},
	{ID: "hagezi-native-roku", Name: "HaGeZi native Roku", Group: GroupTelemetry,
		URL: "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/native.roku.txt", Homepage: hageziHome, License: "GPL-3.0"},
	{ID: "hagezi-native-vivo", Name: "HaGeZi native Vivo", Group: GroupTelemetry,
		URL: "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/native.vivo.txt", Homepage: hageziHome, License: "GPL-3.0"},
	{ID: "hagezi-native-xiaomi", Name: "HaGeZi native Xiaomi", Group: GroupTelemetry,
		URL: "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/native.xiaomi.txt", Homepage: hageziHome, License: "GPL-3.0"},

	// Optional extras.
	{ID: "hagezi-popupads", Name: "HaGeZi Pop-up Ads", Group: GroupExtras,
		URL: "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/popupads.txt", Homepage: hageziHome, License: "GPL-3.0"},
	{ID: "hagezi-gambling", Name: "HaGeZi Gambling", Group: GroupExtras,
		URL: "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/gambling.txt", Homepage: hageziHome, License: "GPL-3.0"},
	{ID: "hagezi-nsfw", Name: "HaGeZi NSFW", Group: GroupExtras,
		URL: "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/nsfw.txt", Homepage: hageziHome, License: "GPL-3.0"},
	{ID: "hagezi-bypass", Name: "HaGeZi DoH/VPN/Proxy Bypass", Group: GroupExtras,
		URL: "https://raw.githubusercontent.com/hagezi/dns-blocklists/main/adblock/doh-vpn-proxy-bypass.txt", Homepage: hageziHome, License: "GPL-3.0",
		Description: "Stops devices from sidestepping DNS filtering with encrypted resolvers or known proxies."},
}

// All returns a copy so callers cannot mutate the catalogue.
func All() []Preset {
	out := make([]Preset, len(presets))
	copy(out, presets)
	return out
}

// ByID resolves one preset id.
func ByID(id string) (Preset, bool) {
	for _, p := range presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}
