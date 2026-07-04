package unifi

// Enum values verified against a live UCG Max (Network 10.4.57) on
// 2026-07-02 — see testdata/trafficrules_probe.json for captured payloads.
const (
	ActionBlock = "BLOCK"

	MatchDomain      = "DOMAIN"
	MatchAppCategory = "APP_CATEGORY"
	MatchInternet    = "INTERNET"

	ModeAlways    = "ALWAYS"
	ModeEveryDay  = "EVERY_DAY"
	ModeEveryWeek = "EVERY_WEEK"
	ModeOneTime   = "ONE_TIME_ONLY"

	TargetTypeClient     = "CLIENT"
	TargetTypeAllClients = "ALL_CLIENTS"

	DirectionTo = "TO"
)

type Domain struct {
	Domain     string `json:"domain"`
	Ports      []int  `json:"ports"`
	PortRanges []any  `json:"port_ranges"`
}

type TargetDevice struct {
	ClientMAC string `json:"client_mac,omitempty"`
	Type      string `json:"type"`
}

type Schedule struct {
	Mode           string   `json:"mode"`
	Date           string   `json:"date,omitempty"` // ONE_TIME_ONLY: "2026-07-03"
	RepeatOnDays   []string `json:"repeat_on_days"` // EVERY_WEEK: ["sun".."sat"]
	TimeAllDay     bool     `json:"time_all_day"`
	TimeRangeStart string   `json:"time_range_start,omitempty"` // "20:00"
	TimeRangeEnd   string   `json:"time_range_end,omitempty"`   // may be earlier than start (crosses midnight)
}

// TrafficRule mirrors the v2 trafficrules payload. Slices must be non-nil on
// create (the API expects [] not null) — NewBlockRule initializes them.
type TrafficRule struct {
	ID               string         `json:"_id,omitempty"`
	Action           string         `json:"action"`
	Description      string         `json:"description"`
	MatchingTarget   string         `json:"matching_target"`
	Domains          []Domain       `json:"domains"`
	AppCategoryIDs   []int          `json:"app_category_ids"`
	AppIDs           []int          `json:"app_ids"`
	IPAddresses      []string       `json:"ip_addresses"`
	IPRanges         []string       `json:"ip_ranges"`
	NetworkIDs       []string       `json:"network_ids"`
	Regions          []string       `json:"regions"`
	Schedule         Schedule       `json:"schedule"`
	TargetDevices    []TargetDevice `json:"target_devices"`
	TrafficDirection string         `json:"traffic_direction"`
	Enabled          bool           `json:"enabled"`
}

// NewBlockRule returns a BLOCK rule with every slice initialized so it
// marshals as [] rather than null.
func NewBlockRule() TrafficRule {
	return TrafficRule{
		Action:           ActionBlock,
		Domains:          []Domain{},
		AppCategoryIDs:   []int{},
		AppIDs:           []int{},
		IPAddresses:      []string{},
		IPRanges:         []string{},
		NetworkIDs:       []string{},
		Regions:          []string{},
		Schedule:         Schedule{RepeatOnDays: []string{}},
		TargetDevices:    []TargetDevice{},
		TrafficDirection: DirectionTo,
	}
}

// Site is an official v1 API site.
type Site struct {
	ID                string `json:"id"`
	InternalReference string `json:"internalReference"`
	Name              string `json:"name"`
}

// NetClient is an official v1 API client (a device on the network).
type NetClient struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	IPAddress   string `json:"ipAddress"`
	MACAddress  string `json:"macAddress"`
	Type        string `json:"type"` // WIRED | WIRELESS
	ConnectedAt string `json:"connectedAt"`
}
