package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func NewClient() *Client {
	return &Client{BaseURL: "https://api.cloudflare.com/client/v4", HTTP: &http.Client{Timeout: 20 * time.Second}}
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type envelope[T any] struct {
	Success bool       `json:"success"`
	Errors  []apiError `json:"errors"`
	Result  T          `json:"result"`
}

func (c *Client) do(ctx context.Context, token, method, path string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("Cloudflare bağlantısı: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e envelope[json.RawMessage]
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&e)
		if len(e.Errors) > 0 {
			return fmt.Errorf("Cloudflare: %s", e.Errors[0].Message)
		}
		return fmt.Errorf("Cloudflare HTTP %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	var raw envelope[json.RawMessage]
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&raw); err != nil {
		return fmt.Errorf("Cloudflare yanıtı: %w", err)
	}
	if !raw.Success {
		if len(raw.Errors) > 0 {
			return fmt.Errorf("Cloudflare: %s", raw.Errors[0].Message)
		}
		return fmt.Errorf("Cloudflare işlemi başarısız")
	}
	return json.Unmarshal(raw.Result, out)
}

type Zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}
type Record struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Content   string `json:"content"`
	TTL       int    `json:"ttl"`
	Proxied   bool   `json:"proxied"`
	Proxiable bool   `json:"proxiable"`
	Priority  int    `json:"priority,omitempty"`
}
type RecordInput struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Proxied  bool   `json:"proxied"`
	Priority int    `json:"priority,omitempty"`
}

func (c *Client) Verify(ctx context.Context, token string) error {
	var v struct {
		Status string `json:"status"`
	}
	if err := c.do(ctx, token, http.MethodGet, "/user/tokens/verify", nil, &v); err != nil {
		return err
	}
	if v.Status != "active" {
		return fmt.Errorf("Cloudflare API token aktif değil")
	}
	return nil
}
func (c *Client) FindZone(ctx context.Context, token, name string) (Zone, error) {
	var zones []Zone
	if err := c.do(ctx, token, http.MethodGet, "/zones?name="+url.QueryEscape(name)+"&status=active&per_page=2", nil, &zones); err != nil {
		return Zone{}, err
	}
	for _, z := range zones {
		if strings.EqualFold(z.Name, name) {
			return z, nil
		}
	}
	return Zone{}, fmt.Errorf("Cloudflare hesabında %s zone'u bulunamadı veya aktif değil", name)
}
func (c *Client) Records(ctx context.Context, token, zoneID string) ([]Record, error) {
	var v []Record
	err := c.do(ctx, token, http.MethodGet, "/zones/"+zoneID+"/dns_records?per_page=500", nil, &v)
	return v, err
}
func (c *Client) CreateRecord(ctx context.Context, token, zoneID string, in RecordInput) (Record, error) {
	var v Record
	err := c.do(ctx, token, http.MethodPost, "/zones/"+zoneID+"/dns_records", in, &v)
	return v, err
}
func (c *Client) UpdateRecord(ctx context.Context, token, zoneID, recordID string, in RecordInput) (Record, error) {
	var v Record
	err := c.do(ctx, token, http.MethodPut, "/zones/"+zoneID+"/dns_records/"+recordID, in, &v)
	return v, err
}
func (c *Client) DeleteRecord(ctx context.Context, token, zoneID, recordID string) error {
	var v json.RawMessage
	return c.do(ctx, token, http.MethodDelete, "/zones/"+zoneID+"/dns_records/"+recordID, nil, &v)
}
func (c *Client) PurgeCache(ctx context.Context, token, zoneID string) error {
	var v json.RawMessage
	return c.do(ctx, token, http.MethodPost, "/zones/"+zoneID+"/purge_cache", map[string]bool{"purge_everything": true}, &v)
}
