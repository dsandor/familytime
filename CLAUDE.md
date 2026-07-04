This project will be a golang executable with a built in web ui. The application will be an easy to use app that lets parents turn applications and website access off. Under the covers the app will interact with the unifi api on the cloud gateway (/network/default/settings/traffic-and-firewall-rules). The purpose of the app is to provide a simple to use interface for setting time boundaries for app usage. it should also provide a way to target the rules by time and by device. parents should be able to 'tag or name' a device as a kids device and set rules that target that device or a group of devices. Users will provide an api key and then be allowed to set global (all devices) rules and rules for a group of devices.  the key is to make this extremely easy and family friendly for non-technical users. The ui needs to be beautiful and simple.


NOTES on the unifi.

# UniFi Cloud Gateway Max — Programmatic Domain Firewall Rules

Reference for managing domain-based block/restrict rules on a UniFi Cloud Gateway Max via API.

There are two APIs available directly on the gateway, and domain rules are reachable through both.

---

## Option 1: Official Local Network API (API Key)

Recent UniFi Network releases ship an official, documented REST API on the gateway itself.

**Setup:**
1. Generate an API key in the UniFi console: **Settings → Control Plane → Integrations**
2. Call the API with the key in a header

**Base URL:**
```
https://<gateway-ip>/proxy/network/integration/v1/...
```

**Authentication:**
```
X-API-KEY: <your-api-key>
```

**Coverage:** The official API has been expanding and now includes:
- Firewall zones and policies
- ACL rules
- DNS policies

Domain-based blocking lives in the firewall policies / DNS policies area of the newer zone-based firewall.

**Pros:**
- Key-based auth (no session/cookie management)
- Documented and stable
- Won't break on firmware upgrades

**Cons:**
- Newer; endpoint coverage may not yet include everything the UI can do — verify your firmware's API version exposes the domain policy endpoints you need

---

## Option 2: Undocumented Internal v2 API (What the UI Uses)

The mature, battle-tested route the community has used for years. This is the same API the UniFi web UI calls.

**Authentication (session + CSRF):**
1. `POST https://<gateway-ip>/api/auth/login` with JSON body:
   ```json
   { "username": "<local-admin>", "password": "<password>" }
   ```
2. Capture the session cookie **and** the CSRF token from the response
3. Send both with all subsequent requests

> **Note:** A *local admin* account is required to get firewall rule permissions. UniFi Cloud (SSO) accounts won't work for this.

**Domain-based blocks are "Traffic Rules":**

| Action | Method | Endpoint |
|--------|--------|----------|
| List rules | `GET` | `/proxy/network/v2/api/site/default/trafficrules` |
| Create rule | `POST` | `/proxy/network/v2/api/site/default/trafficrules` |
| Update rule | `PUT` | `/proxy/network/v2/api/site/{site}/trafficrules/{id}` |
| Delete rule | `DELETE` | `/proxy/network/v2/api/site/{site}/trafficrules/{id}` |

**Quirks:**
- A successful `PUT` returns **201**, not 200
- `GET` is not allowed on an individual rule ID — list all rules and filter

**Domain-block rule payload (key fields):**
```json
{
  "action": "BLOCK",
  "matching_target": "DOMAIN",
  "domains": [
    { "domain": "example.com" }
  ],
  "target_devices": [ ... ],
  "enabled": true
}
```

> **Tip:** The easiest way to see the exact, current schema is to create one rule in the UI, then `GET` the trafficrules list back and copy the shape.

**Pros:**
- Full parity with the UI — everything the UI can do, this API can do
- Well-explored by the community

**Cons:**
- Unofficial; payload shapes and endpoints can shift between Network app versions
- Session/cookie + CSRF auth is more fiddly than an API key

---

## Recommended Approach

1. **Check the official v1 API first** on your firmware version — if it exposes the domain policy endpoints you need, use it (stable, key-based, upgrade-safe).
2. **Fall back to the v2 internal API** if the official one doesn't cover domain rules yet. Just pin your expectations to your current Network app version and re-verify after upgrades.

---

## Useful References

- **Ubiquiti Community Wiki — Controller API:** https://ubntwiki.com/products/software/unifi-controller/api
- **Home Assistant integration (great payload schema reference):** https://github.com/sirkirby/unifi-network-rules — manages UDM/Cloud Gateway firewall policies and traffic rules via this same API; its source shows working request/response shapes
- **Official API getting started:** https://help.ui.com/hc/en-us/articles/30076656117655-Getting-Started-with-the-Official-UniFi-API
- **UniFi Traffic & Policy Management overview:** https://help.ui.com/hc/en-us/articles/5546542486551-Traffic-Policy-Management-in-UniFi

UNIFI_API_KEY is located in the .env file.