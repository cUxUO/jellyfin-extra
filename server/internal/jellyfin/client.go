// Package jellyfin 是轉碼伺服器用到的 Jellyfin API 子集。
package jellyfin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrUnauthorized 表示 token 無效或已過期。
var ErrUnauthorized = errors.New("jellyfin: unauthorized")

// ErrNotFound 表示項目不存在，或這個使用者沒有權限看。
var ErrNotFound = errors.New("jellyfin: not found")

type Client struct {
	base *url.URL
	http *http.Client
}

// New 建立 client。baseURL 應是內網位址（例如 http://jellyfin:8096），
// 原始檔串流會走這條路，繞去對外 HTTPS 只會增加弱主機的 TLS 負擔。
func New(baseURL string) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("jellyfin url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("jellyfin url: unsupported scheme %q", u.Scheme)
	}
	return &Client{base: u, http: &http.Client{Timeout: 15 * time.Second}}, nil
}

// AuthHeader 是 Jellyfin 10.8 之後的標準驗證標頭。
func AuthHeader(token string) string {
	return fmt.Sprintf(`MediaBrowser Token="%s"`, token)
}

type User struct {
	ID   string `json:"Id"`
	Name string `json:"Name"`
}

type PlaybackInfo struct {
	MediaSources []MediaSource `json:"MediaSources"`
	ErrorCode    string        `json:"ErrorCode"`
}

type MediaSource struct {
	ID           string        `json:"Id"`
	Container    string        `json:"Container"`
	RunTimeTicks int64         `json:"RunTimeTicks"`
	Bitrate      int64         `json:"Bitrate"`
	MediaStreams []MediaStream `json:"MediaStreams"`
}

type MediaStream struct {
	Type           string `json:"Type"` // Video、Audio、Subtitle…
	Index          int    `json:"Index"`
	Codec          string `json:"Codec"`
	Profile        string `json:"Profile"`
	Width          int    `json:"Width"`
	Height         int    `json:"Height"`
	BitDepth       int    `json:"BitDepth"`
	BitRate        int64  `json:"BitRate"`
	Channels       int    `json:"Channels"`
	Language       string `json:"Language"`
	Title          string `json:"DisplayTitle"`
	IsDefault      bool   `json:"IsDefault"`
	IsExternal     bool   `json:"IsExternal"`
	VideoRange     string `json:"VideoRange"`     // SDR、HDR
	VideoRangeType string `json:"VideoRangeType"` // SDR、HDR10、HLG、DOVI…
}

// CurrentUser 用 token 查出使用者，同時也是 token 的驗證。
func (c *Client) CurrentUser(ctx context.Context, token string) (*User, error) {
	var u User
	if err := c.getJSON(ctx, token, "/Users/Me", nil, &u); err != nil {
		return nil, err
	}
	if u.ID == "" {
		return nil, ErrUnauthorized
	}
	return &u, nil
}

func (c *Client) PlaybackInfo(ctx context.Context, token, userID, itemID string) (*PlaybackInfo, error) {
	var info PlaybackInfo
	q := url.Values{"userId": {userID}}
	if err := c.getJSON(ctx, token, "/Items/"+url.PathEscape(itemID)+"/PlaybackInfo", q, &info); err != nil {
		return nil, err
	}
	if info.ErrorCode != "" {
		return nil, fmt.Errorf("jellyfin: playback info: %s", info.ErrorCode)
	}
	return &info, nil
}

// StreamURL 是不經轉碼的原始檔網址，支援 Range。
func (c *Client) StreamURL(itemID, mediaSourceID string) string {
	u := *c.base
	u.Path += "/Videos/" + url.PathEscape(itemID) + "/stream"
	q := url.Values{"static": {"true"}}
	if mediaSourceID != "" {
		q.Set("mediaSourceId", mediaSourceID)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (c *Client) getJSON(ctx context.Context, token, path string, q url.Values, out any) error {
	u := *c.base
	u.Path += path
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", AuthHeader(token))
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("jellyfin: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode != http.StatusOK:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("jellyfin: GET %s: %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("jellyfin: decode %s: %w", path, err)
	}
	return nil
}
