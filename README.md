# 🛡️ Family Time

Family-friendly screen-time rules for UniFi networks. One small binary with a
built-in web app: parents group devices by kid, then block apps ("YouTube",
"Roblox"), whole categories, specific websites, or *all* internet — on a
schedule ("school nights, 8pm–7am") or right now ("pause for 30 minutes").

Enforcement runs **on your UniFi gateway** as native traffic rules with
built-in schedules, so blocking keeps working even when Family Time isn't running.

## Requirements

- A UniFi Cloud Gateway (tested on UCG Max, UniFi Network 10.4.x)
- An API key: UniFi console → **Settings → Control Plane → Integrations**
  (don't worry — the setup wizard shows you exactly where to find this)

## Installing Family Time

No app store, no account — Family Time is one small program you download
and run. Get it from the [Family Time website](https://dsandor.github.io/familytime/)
or the [latest release](https://github.com/dsandor/familytime/releases/latest).

### On a Mac

1. Download the Mac version — **Apple Silicon** for newer Macs (M1 and
   later), **Intel** for older ones. Not sure which you have? Click the
   Apple menu in the top-left corner → **About This Mac**: if it says
   "Chip: Apple …", choose Apple Silicon.
2. In your Downloads folder, double-click the file you downloaded. It
   unpacks into a folder — inside is a file called **familytime**.
3. Double-click **familytime**. A Terminal window opens and says
   *Family Time is running*. If your Mac asks whether you're sure you
   want to open it, click **Open** — the app is signed and checked by
   Apple.
4. Open your web browser and go to **http://localhost:8080**. The setup
   wizard walks you through the rest.

### On Windows

1. Download the Windows version.
2. In your Downloads folder, right-click the file → **Extract All…** →
   **Extract**.
3. In the folder that appears, double-click **familytime.exe**.
   - If you see "Windows protected your PC", click **More info**, then
     **Run anyway**. (The Windows version isn't code-signed yet.)
   - If the firewall asks, click **Allow access**.
4. Leave the black window open — that's Family Time running — and go to
   **http://localhost:8080** in your web browser.

### On Linux

    tar xzf familytime_*_linux_*.tar.gz
    ./familytime

Then open http://localhost:8080.

### Good to know

- **Keep the Family Time window open** while you're using the controls.
  If you close it, schedules you've already set **keep working** — they
  live on your UniFi gateway — but the one-tap controls won't load again
  until you restart Family Time.
- For controls that are always available, run it on a computer that
  stays on — a desktop, a mini PC, anything.

## Building from source

Requires Go 1.24+.

    go build -o familytime ./cmd/familytime
    ./familytime
    # open http://localhost:8080 and follow the setup wizard

Flags: `--port` (default 8080), `--data` (default: your OS config dir +
`/familytime/familytime.json`). If a `.env` file with `UNIFI_API_KEY=...` sits in
the working directory, setup offers to use it automatically.

## How it works

Every Family Time rule is a real UniFi traffic rule tagged `[family-time] ` in its
description, with the schedule enforced by the gateway itself. Family Time never
modifies or deletes rules it didn't create. Deleting a rule/profile in
Family Time removes its gateway rules; deleting a Family Time-made rule in the UniFi
app is tolerated (Family Time forgets it on the next cleanup pass).

## Enrolling devices

iPhones and iPads use a "Private Wi-Fi Address" by default: your UniFi
gateway sees a randomized MAC and a generic name like "iPhone 72:68" instead
of the device's real hardware address, so it never shows up reliably in the
device picker.

Family Time works around this with a self-enrollment page. Settings shows an
address (e.g. `http://192.168.0.42:8080/enroll`) — open it in the browser
**on the device you want to add**. The server identifies the device by the
IP address of that very connection against the gateway's live client list
(never anything the browser claims about itself), then lets you name it and
pick its group right there. No PIN needed — the page can only ever affect
the device that opened it.

**Names stay in sync with UniFi.** Whenever a device gets a name in Family
Time — during enrollment, or by renaming it inline in a group's device list
(the ✎ button next to a member) — that name is pushed to UniFi as the
device's client alias, best-effort, over the same gateway API Family Time
already talks to. A sync failure never blocks the save; it's only logged.
The reverse direction works too: rename a client in the UniFi app, and the
new name shows up next time Family Time reads the device list — there's
nothing to configure either way.

**Caveat:** iOS's *"Rotate Wi-Fi Address"* setting (the non-default,
Rotating option under a Wi-Fi network's Private Address setting) changes the
private MAC address periodically, which will make an enrolled device look
new again and require re-enrolling. For a set-it-and-forget-it setup, use
the default *"Fixed"* private address on kids' devices for your home
network — or turn Private Address off for that network entirely.

## Security notes

- The web UI is protected by a parent PIN (bcrypt-hashed; login backs off
  after repeated failures). Sessions last 30 days per browser.
- The gateway's self-signed TLS certificate is pinned on first use
  (SHA-256). If UniFi regenerates it (e.g. firmware update), Family Time shows a
  banner and Settings offers one-tap re-trust.
- The API key and PIN hash live in the data file (created mode 0600). Treat
  that file like a password.
- Family Time binds to all interfaces on your LAN. Don't port-forward it.

## Development

    go test ./...                       # unit tests (no gateway needed)
    FAMILYTIME_E2E=1 go test ./internal/e2e/ -v   # opt-in live gateway test

The UI is embedded via go:embed (`web/static/`) — no Node toolchain; Alpine
is vendored. `go build` is the whole pipeline.

## Troubleshooting

- **"Can't reach your UniFi gateway"** — check the gateway address in
  Settings, and that you're on the same network.
- **"The gateway rejected the API key"** — regenerate a key in the UniFi
  console and update it in Settings.
- **Certificate banner** — expected after UniFi updates; re-trust in
  Settings.
- **An app preset doesn't fully block the app** — domain lists live in
  `internal/rules/presets.go`; add the missing domains and rebuild.

## ⚖️ Dual Licensing & Commercial Use

This project is dual-licensed under the **GNU Affero General Public License v3.0 (AGPL-3.0)** and a commercial license.

### 🟢 Open Source Use (AGPL-3.0)
You are free to use, modify, and distribute this software for personal or educational use, and within open-source projects, provided that:
* Any modifications or derivative works are also open-sourced under the AGPL-3.0.
* If you host this software as a network service (SaaS), you must make your modified source code publicly available.

### 🔵 Commercial Use
If you want to integrate this software into a proprietary, closed-source product, or run it commercially without complying with the AGPL-3.0 terms, you must purchase a commercial license.

For commercial licensing options, pricing, and inquiries, please contact: **[your-email@example.com]**

