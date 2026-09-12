package vaultcluster

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/hashicorp/vault/api"
)

type Client struct {
	API *api.Client
	Cfg Config
}

func New(cfg Config) (*Client, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("VAULT_ADDR is not set")
	}
	ac := api.DefaultConfig()
	ac.Address = cfg.Addr
	if cfg.HTTPTimeout > 0 {
		ac.Timeout = cfg.HTTPTimeout
	}
	vc, err := api.NewClient(ac)
	if err != nil {
		return nil, err
	}
	if cfg.Token != "" {
		vc.SetToken(cfg.Token)
	}
	return &Client{API: vc, Cfg: cfg}, nil
}

func (c *Client) WithToken(token string) *Client {
	clone, err := c.API.Clone()
	if err != nil {
		cp := *c
		return &cp
	}
	clone.SetToken(token)
	cp := *c
	cp.API = clone
	cp.Cfg.Token = token
	return &cp
}

type HTTPResult struct {
	Status int
	Body   []byte
}

func (c *Client) Do(method, path string, body any) (HTTPResult, error) {
	path = strings.TrimPrefix(path, "/")
	query := ""
	if i := strings.Index(path, "?"); i >= 0 {
		query = path[i+1:]
		path = path[:i]
	}
	req := c.API.NewRequest(method, "/v1/"+path)
	if query != "" {
		vals, err := url.ParseQuery(query)
		if err != nil {
			return HTTPResult{}, err
		}
		req.Params = vals
	}
	if body != nil {
		if err := req.SetJSONBody(body); err != nil {
			return HTTPResult{}, err
		}
	}
	resp, err := c.API.RawRequest(req)
	if err != nil && resp == nil {
		return HTTPResult{Status: 0}, err
	}
	if resp == nil {
		return HTTPResult{}, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return HTTPResult{Status: resp.StatusCode, Body: b}, nil
}

func (c *Client) Must(method, path string, body any) (HTTPResult, error) {
	r, err := c.Do(method, path, body)
	if err != nil {
		return r, err
	}
	if r.Status < 200 || r.Status >= 300 {
		return r, fmt.Errorf("%s %s failed (HTTP %d): %s", method, path, r.Status, strings.TrimSpace(string(r.Body)))
	}
	return r, nil
}
