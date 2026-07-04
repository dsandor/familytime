package unifi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrUnauthorized = errors.New("unifi: unauthorized — check the API key")
	ErrNotFound     = errors.New("unifi: not found")
	ErrCertChanged  = errors.New("unifi: gateway TLS certificate changed since setup")
)

const v2Site = "default" // v2 API uses the site's internalReference

type Client struct {
	baseURL string
	apiKey  string
	hc      *http.Client
}

// New builds a client for the gateway at host. Host may be a bare IP/name
// ("192.168.0.1") or a full URL (tests pass httptest URLs).
//
// UniFi gateways serve a self-signed certificate, so CA verification cannot
// succeed. Instead the cert is pinned trust-on-first-use: setup captures its
// SHA-256 fingerprint via FetchCertFingerprint and every later connection
// must present the same leaf cert. certFingerprint is "" only during that
// first fetch and in unit tests.
func New(host, apiKey, certFingerprint string) *Client {
	base := host
	if !strings.Contains(base, "://") {
		base = "https://" + base
	}
	return &Client{
		baseURL: strings.TrimRight(base, "/"),
		apiKey:  apiKey,
		hc: &http.Client{
			Timeout:   10 * time.Second,
			Transport: &http.Transport{TLSClientConfig: pinnedTLSConfig(certFingerprint)},
		},
	}
}

// pinnedTLSConfig skips CA-chain verification (self-signed cert) and instead
// verifies the leaf certificate's SHA-256 fingerprint when one is pinned.
func pinnedTLSConfig(fingerprint string) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, // CA chain can't verify a self-signed cert; the leaf is pinned below
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if fingerprint == "" {
				return nil // first-use fetch or unit test against httptest
			}
			if len(rawCerts) == 0 {
				return ErrCertChanged
			}
			sum := sha256.Sum256(rawCerts[0])
			if hex.EncodeToString(sum[:]) != fingerprint {
				return ErrCertChanged
			}
			return nil
		},
	}
}

// FetchCertFingerprint connects to host and returns the SHA-256 hex
// fingerprint of its leaf TLS certificate. Called once at setup
// (trust-on-first-use) and again from the Settings "trust new certificate"
// action. Returns "" without error for plain-HTTP hosts (tests).
func FetchCertFingerprint(ctx context.Context, host string) (string, error) {
	base := host
	if !strings.Contains(base, "://") {
		base = "https://" + base
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" {
		return "", nil
	}
	addr := u.Host
	if u.Port() == "" {
		addr += ":443"
	}
	d := tls.Dialer{Config: &tls.Config{InsecureSkipVerify: true}} // fingerprint capture only — result is pinned for all later connections
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return "", fmt.Errorf("unifi: can't reach gateway: %w", err)
	}
	defer conn.Close()
	certs := conn.(*tls.Conn).ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return "", errors.New("unifi: gateway presented no certificate")
	}
	sum := sha256.Sum256(certs[0].Raw)
	return hex.EncodeToString(sum[:]), nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("unifi: marshal: %w", err)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-KEY", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("unifi: can't reach gateway: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return fmt.Errorf("%w (HTTP %d)", ErrUnauthorized, resp.StatusCode)
	case resp.StatusCode == 404:
		return fmt.Errorf("%w: %s", ErrNotFound, path)
	case resp.StatusCode >= 400:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("unifi: HTTP %d on %s %s: %s", resp.StatusCode, method, path, msg)
	}
	if out == nil {
		return nil
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil // some writes return an empty body
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("unifi: decode %s: %w", path, err)
	}
	return nil
}

// --- official v1 API (read-only) ---

type v1Page[T any] struct {
	Offset     int `json:"offset"`
	Count      int `json:"count"`
	TotalCount int `json:"totalCount"`
	Data       []T `json:"data"`
}

func (c *Client) Version(ctx context.Context) (string, error) {
	var out struct {
		ApplicationVersion string `json:"applicationVersion"`
	}
	err := c.do(ctx, http.MethodGet, "/proxy/network/integration/v1/info", nil, &out)
	return out.ApplicationVersion, err
}

func (c *Client) FirstSite(ctx context.Context) (Site, error) {
	var page v1Page[Site]
	if err := c.do(ctx, http.MethodGet, "/proxy/network/integration/v1/sites", nil, &page); err != nil {
		return Site{}, err
	}
	if len(page.Data) == 0 {
		return Site{}, errors.New("unifi: gateway reports no sites")
	}
	return page.Data[0], nil
}

func (c *Client) ListClients(ctx context.Context, siteID string) ([]NetClient, error) {
	var all []NetClient
	offset := 0
	for {
		var page v1Page[NetClient]
		path := fmt.Sprintf("/proxy/network/integration/v1/sites/%s/clients?limit=200&offset=%d", siteID, offset)
		if err := c.do(ctx, http.MethodGet, path, nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Data...)
		offset += page.Count
		if offset >= page.TotalCount || page.Count == 0 {
			return all, nil
		}
	}
}

// --- internal v2 API (traffic rules) ---

func (c *Client) trafficRulesPath() string {
	return "/proxy/network/v2/api/site/" + v2Site + "/trafficrules"
}

func (c *Client) ListTrafficRules(ctx context.Context) ([]TrafficRule, error) {
	var out []TrafficRule
	err := c.do(ctx, http.MethodGet, c.trafficRulesPath(), nil, &out)
	return out, err
}

func (c *Client) CreateTrafficRule(ctx context.Context, r TrafficRule) (TrafficRule, error) {
	var out TrafficRule
	err := c.do(ctx, http.MethodPost, c.trafficRulesPath(), r, &out)
	return out, err
}

func (c *Client) UpdateTrafficRule(ctx context.Context, r TrafficRule) error {
	if r.ID == "" {
		return errors.New("unifi: update requires rule _id")
	}
	return c.do(ctx, http.MethodPut, c.trafficRulesPath()+"/"+r.ID, r, nil)
}

func (c *Client) DeleteTrafficRule(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, c.trafficRulesPath()+"/"+id, nil, nil)
}

// --- legacy v1 API (client alias rename) ---
//
// This is the same "unofficial but battle-tested" category as the v2
// trafficrules API above — the official v1 integration API doesn't support
// rename (PATCH/PUT on a client returns 405). Verified live against a UCG
// Max on 2026-07-03: set -> confirm -> clear -> confirm.

// legacyEnvelope wraps every legacy `/api/s/{site}/...` response: a
// meta.rc of "ok" on success, with meta.msg carrying detail on failure.
type legacyEnvelope struct {
	Meta struct {
		RC  string `json:"rc"`
		Msg string `json:"msg,omitempty"`
	} `json:"meta"`
	Data []legacyUser `json:"data,omitempty"`
}

type legacyUser struct {
	ID   string `json:"_id"`
	MAC  string `json:"mac"`
	Name string `json:"name,omitempty"`
}

func (e legacyEnvelope) err(context string) error {
	if e.Meta.RC == "ok" {
		return nil
	}
	msg := e.Meta.RC
	if e.Meta.Msg != "" {
		msg = e.Meta.Msg
	}
	return fmt.Errorf("unifi: %s: %s", context, msg)
}

func (c *Client) legacyUserPath() string {
	return "/proxy/network/api/s/" + v2Site + "/rest/user"
}

// RenameClient sets mac's alias (the name shown in both the UniFi app and
// Family Time) to name. Setting name to "" clears the alias. mac is matched
// case-insensitively against the gateway's client list.
func (c *Client) RenameClient(ctx context.Context, mac, name string) error {
	mac = strings.ToLower(mac)
	var list legacyEnvelope
	if err := c.do(ctx, http.MethodGet, c.legacyUserPath(), nil, &list); err != nil {
		return err
	}
	if err := list.err("listing clients"); err != nil {
		return err
	}
	var id string
	for _, u := range list.Data {
		if strings.ToLower(u.MAC) == mac {
			id = u.ID
			break
		}
	}
	if id == "" {
		return fmt.Errorf("%w: client with mac %s", ErrNotFound, mac)
	}
	var out legacyEnvelope
	if err := c.do(ctx, http.MethodPut, c.legacyUserPath()+"/"+id, map[string]string{"name": name}, &out); err != nil {
		return err
	}
	return out.err("renaming client")
}
