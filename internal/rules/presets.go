package rules

// Preset is a curated app bundle parents can block with one tap. Domain
// lists are maintained here — adding an app is a data change, not a code
// change. Efficacy of each list is checked live in the E2E task; a preset
// that can't be blocked by domain alone graduates to UniFi app_ids.
type Preset struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Emoji   string   `json:"emoji"`
	Domains []string `json:"domains"`
}

var presets = []Preset{
	{"youtube", "YouTube", "📺", []string{"youtube.com", "youtu.be", "googlevideo.com", "ytimg.com", "youtube-nocookie.com", "youtubei.googleapis.com"}},
	{"tiktok", "TikTok", "🎵", []string{"tiktok.com", "tiktokv.com", "tiktokcdn.com", "byteoversea.com", "musical.ly"}},
	{"instagram", "Instagram", "📸", []string{"instagram.com", "cdninstagram.com", "instagr.am"}},
	{"snapchat", "Snapchat", "👻", []string{"snapchat.com", "snap.com", "sc-cdn.net", "snapkit.com"}},
	{"roblox", "Roblox", "🧱", []string{"roblox.com", "rbxcdn.com", "rbx.com", "robloxlabs.com"}},
	{"fortnite", "Fortnite", "🔫", []string{"fortnite.com", "epicgames.com", "epicgames.dev", "ol.epicgames.com"}},
	{"minecraft", "Minecraft", "⛏️", []string{"minecraft.net", "minecraftservices.com", "mojang.com"}},
	{"discord", "Discord", "💬", []string{"discord.com", "discordapp.com", "discord.gg", "discordapp.net", "discord.media"}},
	{"twitch", "Twitch", "🎮", []string{"twitch.tv", "ttvnw.net", "jtvnw.net", "twitchcdn.net"}},
	{"netflix", "Netflix", "🍿", []string{"netflix.com", "nflxvideo.net", "nflximg.net", "nflxext.com", "nflxso.net"}},
	{"disneyplus", "Disney+", "🏰", []string{"disneyplus.com", "disney-plus.net", "dssott.com", "bamgrid.com"}},
}

func Presets() []Preset { return presets }

func PresetByID(id string) (Preset, bool) {
	for _, p := range presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// Category maps a family-friendly label to a UniFi DPI app-category id
// (integers, verified accepted by the API 2026-07-02; labels get one visual
// confirmation against the UniFi UI during the E2E task).
type Category struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Emoji   string `json:"emoji"`
	UnifiID int    `json:"-"`
}

var categories = []Category{
	{"social", "Social Media", "💬", 8},
	{"games", "Gaming", "🎮", 10},
	{"streaming", "Video Streaming", "📺", 4},
}

func Categories() []Category { return categories }

func CategoryByID(id string) (Category, bool) {
	for _, c := range categories {
		if c.ID == id {
			return c, true
		}
	}
	return Category{}, false
}
