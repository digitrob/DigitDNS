package web

import (
	"net/http"
)

// The subscriber portal is the one page in this project a customer sees. Every word it
// shows lives here, in one bundle, because the page has no JavaScript it can rely on —
// the error page deliberately carries no <script> at all, so the theme is resolved on
// the server, before the template runs, rather than by a client script.
//
// The portal is English-only. The theme preference remains because it is a real feature
// and its toggle works with JavaScript disabled.

const (
	portalThemeDark  = "dark"
	portalThemeLight = "light"

	// The theme preference is stored client-side and read back on the next visit. It is
	// deliberately not HttpOnly — nothing secret is in it and a future script may want
	// to read it. Secure is set only when the page was reached over TLS: a Secure
	// cookie on a plain-http panel would never be stored, silently undoing the choice on
	// every page load, so it is gated on r.TLS rather than unconditional.
	portalThemeCookie = "hdns_theme"

	// A subscriber sets this once. The cookie outliving the subscription is harmless;
	// the cookie expiring inside it means the page flips palette on its own.
	portalPrefMaxAge = 365 * 24 * 60 * 60
)

// portalText is every word the portal shows. One rule holds it together:
//
//   - no field may be empty. A missing string on this page is not a blank — it is a
//     step of a DNS setup guide that silently is not there.
//   - purely technical strings are NOT here. "Primary DNS (IPv4)", "DNS over TLS (DoT)"
//     and the address values themselves stay literal in the template, because they are
//     addresses to type into a console, not prose.
//
// The portal is English-only; the theme toggle stays because it is a real feature that
// works with JavaScript disabled.
type portalText struct {
	// Page chrome.
	DocTitle    string // <title> of the portal page
	ErrDocTitle string // <title> of the error page
	// Subtitle is the line under the brand heading. It names the product and the
	// engine, with the running version templated in at render time so a released
	// build never advertises a stale one. The heading itself is the operator's own
	// Title from the subscription settings (see portalData.Title), not a translated
	// string — a reseller's brand is the same in every language they sell in.
	Subtitle     string
	PillActive   string
	ThemeToLight string // label of the link that switches TO light
	ThemeToDark  string // label of the link that switches TO dark
	ThemeHint    string // title/aria-label on the theme link

	// What just happened, shown as the line under the topbar or on the error card.
	StatusRegistered string
	StatusConfirmed  string
	StatusPreview    string
	ErrExpiredTitle  string
	ErrExpiredNote   string
	ErrInvalidTitle  string
	ErrInvalidNote   string
	ErrIPLabel       string

	// Identity card.
	CardIdentity string
	LabelAccount string
	LabelUUID    string
	CopiedUUID   string
	BtnCopy      string

	// IP card.
	CardIPStatus    string
	PillBound       string
	LabelDetectedIP string
	HintModem       string
	// LabelRegisteredIPs heads the list of the addresses currently bound to the
	// subscription (v3.1.0). LabelRegisteredNone is shown instead when the list
	// is empty, so the card never renders a heading over nothing.
	LabelRegisteredIPs  string
	LabelRegisteredNone string
	// LabelRegisteredCount is glued to the counts in Go (used / max), the same way
	// WordRemaining is glued to the remaining figure, so it is a field and not a
	// literal in the template.
	LabelRegisteredCount string
	BtnUpdateIP          string

	// Traffic card. WordRemaining, Unlimited, LifetimeLabel and the three cycle names are
	// formatted in Go rather than in the template, because each is glued to a number.
	CardTraffic    string
	LabelRemaining string
	LabelAutoRenew string
	PillQuotaSpent string
	WordRemaining  string
	Unlimited      string
	LifetimeLabel  string
	CycleDaily     string
	CycleWeekly    string
	CycleMonthly   string

	// Countdown card.
	CardCountdown string
	UnitDays      string
	UnitHours     string
	UnitMins      string
	LabelEndDate  string

	// Policies card.
	CardPolicies string
	PolicyAll    string

	// Endpoints card.
	CardEndpoints   string
	HintTapCopy     string
	CopiedPrimary   string
	CopiedSecondary string
	CopiedDoH       string
	CopiedDoT       string

	// Setup guide. One slice per device, one element per numbered step; the DNS values
	// live in a .snippet block after the list rather than mid-sentence, which is what
	// lets a step be a plain translatable string and keeps every address LTR-isolated
	// and copy-shaped instead of buried in prose.
	CardGuide      string
	GuideArrowNote string
	GuideTabsAria  string
	TabPs          string
	TabXbox        string
	TabSwitch      string
	TabWindows     string
	TabApple       string
	TabAndroid     string
	TabRouter      string

	PsTitle     string
	PsSteps     []string
	XboxTitle   string
	XboxSteps   []string
	SwitchTitle string
	SwitchSteps []string
	WinTitle    string
	WinSteps    []string
	RouterTitle string
	RouterSteps []string

	// Apple and Android are two platforms each, not two ordered procedures, so they read
	// as paragraphs. Android's first line is the one that matters: Private DNS takes a
	// hostname and refuses an IP address, and the old markup offered it "<server>:853",
	// which cannot work on any Android device at all.
	AppleTitle     string
	ApplePhone     string
	AppleMac       string
	AndroidTitle   string
	AndroidPrivate string
	// AndroidNoDomain replaces the hostname snippet when no domain is configured.
	// Android's Private DNS field rejects an IP address, so printing one there
	// would be an instruction that cannot be followed.
	AndroidNoDomain string
	AndroidStatic   string

	// The four strings js/portal.js shows. They are bundle fields rather than literals
	// inside the script so the script carries no copy of its own: the template renders
	// them onto data- attributes (#toast and #traffic-remaining) and the script reads
	// them back, so there is one spelling of each message.
	//
	// WordRemaining and Unlimited above are deliberately *not* duplicated here: the
	// script re-renders the traffic figures after a sync, and reusing the very fields Go
	// used for the first paint is what stops the wording changing under the subscriber
	// when they press the button.
	MsgCopied   string
	MsgCopyFail string

	// The register card (Phase B): where the subscriber pastes the
	// out-of-band secret and presses the one write the portal still offers.
	CardRegister      string
	LabelSecret       string
	PlaceholderSecret string
	HintSecret        string
	MsgRegisterOK     string
	MsgRegisterFail   string
	MsgRegisterDenied string
	// The custom-address row (v3.1.0): the subscriber whose gaming device sits
	// behind a different NAT than the phone holding the page types that device's
	// address here. LabelCustomIP and PlaceholderCustomIP name the field; the
	// button posts it with the same out-of-band secret.
	LabelCustomIP       string
	PlaceholderCustomIP string
	BtnRegisterCustomIP string
	HintCustomIP        string
	// One line per machine-readable `reason` the registration API can return.
	// The portal is bilingual and the API answers in English, so these are what
	// the subscriber actually reads when a bind is refused: telling someone with
	// a lapsed plan that their secret was wrong sends them to the wrong problem.
	MsgRegisterSuspended string
	MsgRegisterExpired   string
	MsgRegisterQuota     string
	MsgRegisterConflict  string
	StatusOverview       string
}

// portalEn is the portal's only bundle. The console menu names are the exact strings
// printed on the device, not prose, so a subscriber can match them on screen.
var portalEn = portalText{
	DocTitle:     "Smart Subscription",
	ErrDocTitle:  "Subscription Error",
	Subtitle:     "DigitDNS %s • Low-Latency SmartDNS Engine",
	PillActive:   "ACTIVE",
	ThemeToLight: "Light mode",
	ThemeToDark:  "Dark mode",
	ThemeHint:    "Switch between light and dark",

	StatusRegistered: "Your new IP address has been registered and synced.",
	StatusConfirmed:  "Your IP address is confirmed and the subscription is active.",
	StatusPreview: "This link was opened by a messenger or browser preview, so your IP was not registered. " +
		"To connect, press the “Update my IP” button at the top of this page.",
	ErrExpiredTitle: "Your subscription has expired",
	ErrExpiredNote:  "Your subscription plan has expired. Please contact support.",
	ErrInvalidTitle: "This subscription link is not valid",
	ErrInvalidNote:  "Invalid or unknown subscription token.",
	ErrIPLabel:      "Your detected IP address:",

	CardIdentity: "Subscription details",
	LabelAccount: "Account name:",
	LabelUUID:    "UUID:",
	CopiedUUID:   "UUID copied.",
	BtnCopy:      "Copy",

	CardIPStatus:    "IP status",
	PillBound:       "Registered",
	LabelDetectedIP: "Detected IP:",
	HintModem: "Your IP changes every time your modem or connection restarts. " +
		"When that happens, open this page again and press “Update my IP”.",
	LabelRegisteredIPs:   "Registered addresses:",
	LabelRegisteredNone:  "No address is registered yet. Use the box below to register the address this device is seen from.",
	LabelRegisteredCount: "%d of %d addresses used",
	BtnUpdateIP:          "Update my IP",

	CardTraffic:    "Traffic usage",
	LabelRemaining: "Remaining:",
	LabelAutoRenew: "Quota resets automatically",
	PillQuotaSpent: "The quota for this period is used up",
	WordRemaining:  "remaining",
	Unlimited:      "Unlimited",
	LifetimeLabel:  "Lifetime",
	CycleDaily:     "daily",
	CycleWeekly:    "weekly",
	CycleMonthly:   "monthly",

	CardCountdown: "Time left on this subscription",
	UnitDays:      "Days",
	UnitHours:     "Hours",
	UnitMins:      "Mins",
	LabelEndDate:  "Expires on:",

	CardPolicies: "Games and services enabled on this subscription:",
	PolicyAll:    "Every game and service enabled on the server (Full Inherit)",

	CardEndpoints:   "Your dedicated DNS endpoints",
	HintTapCopy:     "tap any address to copy it",
	CopiedPrimary:   "Primary DNS copied.",
	CopiedSecondary: "Secondary DNS copied.",
	CopiedDoH:       "DoH URL copied.",
	CopiedDoT:       "DoT address copied.",

	CardGuide:      "How to set this DNS up on your device",
	GuideArrowNote: "you can also move between tabs with the ← and → keys",
	GuideTabsAria:  "Device setup guides",
	TabPs:          "PlayStation (PS4/PS5)",
	TabXbox:        "Xbox (Series / One)",
	TabSwitch:      "Nintendo Switch",
	TabWindows:     "Windows 10 / 11",
	TabApple:       "iOS & macOS",
	TabAndroid:     "Android",
	TabRouter:      "Modem / router",

	PsTitle: "Setting the DNS on a PlayStation",
	PsSteps: []string{
		"Open Settings on the console and go to Network.",
		"Choose Set Up Internet Connection and pick how you connect (Wi-Fi or LAN).",
		"Choose Custom so the manual settings appear.",
		"Leave IP Address on Automatic and DHCP Host Name on Do Not Specify.",
		"Under DNS Settings choose Manual and enter the two values below.",
		"Leave MTU on Automatic and Proxy Server on Do Not Use, then run Test Internet Connection.",
	},
	XboxTitle: "Setting the DNS on an Xbox",
	XboxSteps: []string{
		"Press the Xbox button and open Settings.",
		"Go to General ➔ Network settings ➔ Advanced settings.",
		"Open DNS settings and switch it to Manual.",
		"Enter the two values below as the primary and secondary IPv4 DNS, then save.",
	},
	SwitchTitle: "Setting the DNS on a Nintendo Switch",
	SwitchSteps: []string{
		"Go to System Settings ➔ Internet ➔ Internet Settings.",
		"Select the Wi-Fi network you are on and choose Change Settings.",
		"Set DNS Settings to Manual and enter the two values below.",
	},
	WinTitle: "Setting the DNS on Windows 10 / 11",
	WinSteps: []string{
		"Open Settings ➔ Network & Internet.",
		"Click the connection you are using (Wi-Fi or Ethernet) and press Edit next to DNS server assignment.",
		"Change Automatic to Manual and turn the IPv4 switch on.",
		"Enter the two values below as Preferred DNS and Alternate DNS, then press Save.",
	},
	RouterTitle: "Setting the DNS on a modem or router",
	RouterSteps: []string{
		"Open your modem's admin page in a browser (usually 192.168.1.1 or 192.168.0.1).",
		"Find the WAN, Internet or DHCP section and change DNS from Automatic to Manual/Static.",
		"Enter the two values below, save, and restart the modem once. Every device in the house then uses this DNS with no setup of its own.",
	},
	AppleTitle: "Setting the DNS on iOS and macOS",
	ApplePhone: "iPhone and iPad: go to Settings ➔ Wi-Fi ➔ the (i) icon next to your network ➔ Configure DNS ➔ Manual, " +
		"delete the addresses already listed, then use Add Server to add the address below.",
	AppleMac:     "Mac: go to System Settings ➔ Network ➔ Details ➔ the DNS tab, add the address below with the (+) button, and press OK.",
	AndroidTitle: "Setting the DNS on Android",
	AndroidPrivate: "Option 1 — Private DNS (recommended, encrypted): go to Settings ➔ Network & internet ➔ Private DNS, " +
		"choose “Private DNS provider hostname” and enter the hostname below. " +
		"This field accepts a domain name only, so if your server has no domain and TLS certificate, use option 2 instead.",
	AndroidNoDomain: "This server has no domain and TLS certificate yet, so Android's Private DNS " +
		"cannot be used (the field will not accept an IP address). Use option 2 below, or ask " +
		"the server administrator to add a domain.",
	AndroidStatic: "Option 2 — static Wi-Fi settings: go to Settings ➔ Wi-Fi ➔ long-press your network ➔ Modify network ➔ Advanced, " +
		"set IP settings to Static and enter the two values below as DNS 1 and DNS 2.",

	MsgCopied:   "Copied.",
	MsgCopyFail: "Could not copy automatically — please select the text by hand.",

	CardRegister:         "Register or update your IP",
	LabelSecret:          "Registration secret:",
	PlaceholderSecret:    "Paste your registration secret here",
	HintSecret:           "Your provider handed you the registration secret when you bought the plan. After a modem reset, open this page, paste the secret, and press the button.",
	LabelCustomIP:        "Register a specific address:",
	PlaceholderCustomIP:  "e.g. 203.0.113.9",
	BtnRegisterCustomIP:  "Register this address",
	HintCustomIP:         "Use this when the device you want to protect (a console, a TV box) reaches the internet from a different address than the device showing this page.",
	MsgRegisterOK:        "Your new IP address was registered!",
	MsgRegisterFail:      "Registration failed — check the secret and try again.",
	MsgRegisterDenied:    "Wrong registration secret, or an invalid link.",
	MsgRegisterSuspended: "This subscription has been suspended by the operator. Please contact support.",
	MsgRegisterExpired:   "This subscription has expired. Please contact support to renew it.",
	MsgRegisterQuota:     "This subscription's traffic allowance is used up. Please contact support to top it up.",
	MsgRegisterConflict:  "This IP address is already registered to a different subscription. Please contact support.",
	StatusOverview:       "This is your subscription overview; the address this device was seen from is shown below.",
}

// portalStatus is what the handler worked out actually happened, kept as a value rather
// than a sentence so the handler never touches a translated string. Two of the five render
// the error card instead of the portal.
type portalStatus int

const (
	portalStatusRegistered portalStatus = iota // a new address was bound
	portalStatusConfirmed                      // the address was already the bound one
	portalStatusPreview                        // an unfurler opened the link; nothing was bound
	portalStatusExpired                        // the subscription is over
	portalStatusInvalid                        // no such token
	// portalStatusOverview is the read-only portal (Phase B): the page rendered
	// without any binding having happened, which is now the normal case for
	// every visit to /sub/<token>.
	portalStatusOverview
)

// ok reports whether this status renders the portal. The preview counts: the record was
// read, everything on the page is real, and only the binding did not happen.
func (s portalStatus) ok() bool {
	return s == portalStatusRegistered || s == portalStatusConfirmed || s == portalStatusPreview || s == portalStatusOverview
}

// warn reports whether the status line should be marked as a caveat rather than a success.
// It selects between two literal icon branches in the template, because a class attribute
// on this page may not contain a template action.
func (s portalStatus) warn() bool { return s == portalStatusPreview }

// httpStatus is the HTTP status the page carries. An unknown token answered
// 200 for its whole life — a friendly error page wearing a success code —
// which reads as "resource exists" to every prober, prefetcher and cache, and
// is what the T3MP3ST external-attacker pass flagged. The body stays the
// friendly error card; only the status tells the truth now: 404 for a token
// that does not exist, 410 for a plan that is over.
func (s portalStatus) httpStatus() int {
	switch s {
	case portalStatusExpired:
		return http.StatusGone
	case portalStatusInvalid:
		return http.StatusNotFound
	default:
		return http.StatusOK
	}
}

// line is the sentence under the topbar. It used to be computed in the handler and then
// dropped on the floor — the template rendered a fixed heading and never read it — so the
// difference between "your new address is registered" and "your address was already the
// registered one" reached nobody.
func (s portalStatus) line(t portalText) string {
	switch s {
	case portalStatusConfirmed:
		return t.StatusConfirmed
	case portalStatusPreview:
		return t.StatusPreview
	case portalStatusOverview:
		return t.StatusOverview
	default:
		return t.StatusRegistered
	}
}

// errText is the headline and the second line of the error card.
func (s portalStatus) errText(t portalText) (title, note string) {
	if s == portalStatusExpired {
		return t.ErrExpiredTitle, t.ErrExpiredNote
	}
	return t.ErrInvalidTitle, t.ErrInvalidNote
}

// portalPref reads one preference in the order query → cookie → default, and stores the
// query form so the choice survives the next visit. A link, not a form: the query param
// is what makes the toggle work with JavaScript disabled, which the error page needs
// because it has no script at all.
func portalPref(w http.ResponseWriter, r *http.Request, param, cookie string, valid func(string) bool, def string) string {
	if v := r.URL.Query().Get(param); valid(v) {
		http.SetCookie(w, &http.Cookie{
			Name:     cookie,
			Value:    v,
			Path:     "/",
			MaxAge:   portalPrefMaxAge,
			SameSite: http.SameSiteLaxMode,
			Secure:   r.TLS != nil,
		})
		return v
	}
	if c, err := r.Cookie(cookie); err == nil && valid(c.Value) {
		return c.Value
	}
	return def
}

// negotiatePortalTheme picks the palette: ?theme= wins, then the stored choice, then dark.
//
// There is deliberately no third "auto" state that follows prefers-color-scheme. Honouring
// the OS would mean a second copy of all 25 colour tokens inside a media query — the same
// palette written twice, with nothing keeping the two in step — and it would make the
// toggle's own label unanswerable: with no script on the error page, the server cannot know
// which way "auto" resolved, so a link saying "Light mode" would sometimes point at the
// palette already on screen. Two states, both explicit, both always correctly labelled.
func negotiatePortalTheme(w http.ResponseWriter, r *http.Request) string {
	isTheme := func(v string) bool { return v == portalThemeDark || v == portalThemeLight }
	return portalPref(w, r, "theme", portalThemeCookie, isTheme, portalThemeDark)
}
