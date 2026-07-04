package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"familytime/internal/store"
	"familytime/internal/unifi"
)

const (
	sessionCookie = "familytime_session"
	sessionTTL    = 30 * 24 * time.Hour
)

var pinRe = regexp.MustCompile(`^[0-9]{4,6}$`)

func hashPIN(pin string) (string, error) {
	if !pinRe.MatchString(pin) {
		return "", fmt.Errorf("PIN must be 4–6 digits")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	return string(h), err
}

func checkPIN(hash, pin string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pin)) == nil
}

func newSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func sign(secret string, expiry int64) string {
	m := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(m, "%d", expiry)
	return hex.EncodeToString(m.Sum(nil))
}

func (s *Server) issueSession(w http.ResponseWriter) {
	secret := s.store.Snapshot().Auth.SessionSecret
	exp := s.now().Add(sessionTTL).Unix()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    fmt.Sprintf("%d.%s", exp, sign(secret, exp)),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

func (s *Server) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
}

func (s *Server) validSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	expStr, sig, ok := strings.Cut(c.Value, ".")
	if !ok {
		return false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || s.now().Unix() > exp {
		return false
	}
	secret := s.store.Snapshot().Auth.SessionSecret
	if secret == "" {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(sign(secret, exp)))
}

// auth wraps protected handlers with session validation.
func (s *Server) auth(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.validSession(r) {
			fail(w, http.StatusUnauthorized, "Please enter your PIN.")
			return
		}
		h(w, r)
	})
}

// loginGuard rate-limits PIN attempts: after 5 consecutive failures the
// login locks, doubling from 30s up to 15min.
type loginGuard struct {
	mu          sync.Mutex
	fails       int
	lockedUntil time.Time
}

func (g *loginGuard) allowed(now time.Time) (bool, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if now.Before(g.lockedUntil) {
		return false, time.Until(g.lockedUntil)
	}
	return true, 0
}

func (g *loginGuard) fail(now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.fails++
	if g.fails >= 5 {
		lock := 30 * time.Second << uint(min(g.fails-5, 5)) // 30s → 16m cap
		if lock > 15*time.Minute {
			lock = 15 * time.Minute
		}
		g.lockedUntil = now.Add(lock)
	}
}

func (g *loginGuard) reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.fails = 0
	g.lockedUntil = time.Time{}
}

// --- handlers ---

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{
		"configured": s.store.IsConfigured(),
		"authed":     s.validSession(r),
	}
	if !s.store.IsConfigured() {
		gw := s.detectGateway()
		out["suggestedGateway"] = gw
		out["envApiKey"] = os.Getenv("UNIFI_API_KEY") != "" && gw != ""
	}
	writeJSON(w, 200, out)
}

// localIP returns this machine's LAN IPv4 address using a UDP "dial" — no
// packets are actually sent, it just asks the OS which local address it
// would route through to reach the internet. Shared by guessGateway
// (assumes the gateway sits at .1 on that /24) and Settings' enrollUrl (the
// address a phone on the same LAN can actually reach this server at).
func localIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		return ""
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP.To4() == nil {
		return ""
	}
	return addr.IP.To4().String()
}

// guessGateway assumes the common home layout: local /24 with the gateway
// at .1.
func guessGateway() string {
	ip := localIP()
	if ip == "" {
		return ""
	}
	i := strings.LastIndexByte(ip, '.')
	if i < 0 {
		return ""
	}
	return ip[:i+1] + "1"
}

// normalizeHost strips a URL scheme and surrounding whitespace so hosts can
// be compared regardless of how they were typed ("192.168.0.1" vs
// "https://192.168.0.1/").
func normalizeHost(h string) string {
	h = strings.TrimSpace(h)
	if i := strings.Index(h, "://"); i >= 0 {
		h = h[i+3:]
	}
	return strings.TrimSuffix(h, "/")
}

// envKeyAllowedFor reports whether the server-side UNIFI_API_KEY may be used
// for the given requested host: only when it equals the auto-detected
// gateway, so a request naming some other host can never silently borrow
// the server's key.
func (s *Server) envKeyAllowedFor(host string) bool {
	gw := s.detectGateway()
	return gw != "" && normalizeHost(host) == normalizeHost(gw)
}

// handleTestConnection lets the setup wizard verify gateway + key before
// committing anything. Only available while unconfigured — afterwards,
// gateway changes live in Settings behind the PIN.
func (s *Server) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	if s.store.IsConfigured() {
		fail(w, http.StatusConflict, "Family Time is already set up.")
		return
	}
	var in struct{ Host, APIKey string }
	if err := readJSON(r, &in); err != nil || in.Host == "" {
		fail(w, http.StatusBadRequest, "Gateway address is required.")
		return
	}
	if in.APIKey == "" && s.envKeyAllowedFor(in.Host) {
		in.APIKey = os.Getenv("UNIFI_API_KEY")
	}
	fp, err := unifi.FetchCertFingerprint(r.Context(), in.Host)
	if err != nil {
		failErr(w, err)
		return
	}
	version, err := s.newAPI(in.Host, in.APIKey, fp).Version(r.Context())
	if err != nil {
		failErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"version": version})
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if s.store.IsConfigured() {
		fail(w, http.StatusConflict, "Family Time is already set up.")
		return
	}
	var in struct{ Host, APIKey, PIN string }
	if err := readJSON(r, &in); err != nil || in.Host == "" {
		fail(w, http.StatusBadRequest, "Gateway address is required.")
		return
	}
	if in.APIKey == "" && s.envKeyAllowedFor(in.Host) {
		in.APIKey = os.Getenv("UNIFI_API_KEY") // key stays server-side
	}
	if in.APIKey == "" {
		fail(w, http.StatusBadRequest, "An API key is required.")
		return
	}
	pinHash, err := hashPIN(in.PIN)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	fp, err := unifi.FetchCertFingerprint(r.Context(), in.Host)
	if err != nil {
		failErr(w, err)
		return
	}
	api := s.newAPI(in.Host, in.APIKey, fp)
	if _, err := api.Version(r.Context()); err != nil {
		failErr(w, err)
		return
	}
	site, err := api.FirstSite(r.Context())
	if err != nil {
		failErr(w, err)
		return
	}
	err = s.store.Update(func(d *store.Data) error {
		d.Gateway = store.Gateway{Host: in.Host, APIKey: in.APIKey, SiteID: site.ID, SiteName: site.InternalReference, CertFingerprint: fp}
		d.Auth = store.Auth{PINHash: pinHash, SessionSecret: newSecret()}
		return nil
	})
	if err != nil {
		fail(w, http.StatusInternalServerError, "Couldn't save settings: "+err.Error())
		return
	}
	s.issueSession(w)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.store.IsConfigured() {
		fail(w, http.StatusConflict, "Family Time isn't set up yet.")
		return
	}
	if ok, wait := s.guard.allowed(s.now()); !ok {
		fail(w, http.StatusTooManyRequests, fmt.Sprintf("Too many tries — wait %d seconds.", int(wait.Seconds())+1))
		return
	}
	var in struct{ PIN string }
	if err := readJSON(r, &in); err != nil {
		fail(w, http.StatusBadRequest, "Bad request.")
		return
	}
	if !checkPIN(s.store.Snapshot().Auth.PINHash, in.PIN) {
		s.guard.fail(s.now())
		fail(w, http.StatusUnauthorized, "Wrong PIN.")
		return
	}
	s.guard.reset()
	s.issueSession(w)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.clearSession(w)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
