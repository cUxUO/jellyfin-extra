package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jellyfin-extra/server/internal/jellyfin"
	"jellyfin-extra/server/internal/logring"
	"jellyfin-extra/server/internal/session"
)

type fakeSessions struct{}

func (fakeSessions) Snapshot() session.Snapshot {
	return session.Snapshot{MaxSessions: 3, Sessions: []session.SessionInfo{{ID: "abc", Title: "Movie"}}}
}
func (fakeSessions) WorkDirUsage() int64 { return 42 }
func (fakeSessions) WorkDir() string     { return "." }

type fakeSource struct{}

func (fakeSource) BytesRead(id string) int64 { return 7 }
func (fakeSource) TotalRead() int64          { return 99 }

type fakeJF struct{}

func (fakeJF) PublicInfo(context.Context) (*jellyfin.PublicInfo, error) {
	return &jellyfin.PublicInfo{ServerName: "home", Version: "12.1.0"}, nil
}

func request(h http.Handler, path, remote string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = remote
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestDashboardData(t *testing.T) {
	events := logring.New(10)
	events.Write([]byte("2026/10/05 08:00:00 session abc ready\n"))
	h := (&Server{Sessions: fakeSessions{}, Source: fakeSource{}, JF: fakeJF{}, Events: events}).Handler()

	rec := request(h, "/dashboard/data", "192.168.0.10:5000", nil)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var d struct {
		Jellyfin struct {
			OK      bool   `json:"ok"`
			Version string `json:"version"`
		} `json:"jellyfin"`
		Disk struct {
			UsedBytes int64 `json:"usedBytes"`
		} `json:"disk"`
		Sessions struct {
			MaxSessions int `json:"maxSessions"`
			Active      []struct {
				ID          string `json:"id"`
				Title       string `json:"title"`
				SourceBytes int64  `json:"sourceBytes"`
			} `json:"active"`
		} `json:"sessions"`
		Events []string `json:"events"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&d); err != nil {
		t.Fatal(err)
	}
	if !d.Jellyfin.OK || d.Jellyfin.Version != "12.1.0" || d.Disk.UsedBytes != 42 || d.Sessions.MaxSessions != 3 {
		t.Errorf("data = %+v", d)
	}
	if len(d.Sessions.Active) != 1 || d.Sessions.Active[0].Title != "Movie" || d.Sessions.Active[0].SourceBytes != 7 {
		t.Errorf("sessions = %+v", d.Sessions.Active)
	}
	if len(d.Events) != 1 {
		t.Errorf("events = %q", d.Events)
	}

	page := request(h, "/dashboard", "127.0.0.1:5000", nil)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "轉碼伺服器") {
		t.Errorf("page status %d", page.Code)
	}
}

func TestDashboardLANOnly(t *testing.T) {
	h := (&Server{Sessions: fakeSessions{}, Source: fakeSource{}, JF: fakeJF{}}).Handler()
	cases := []struct {
		remote string
		header map[string]string
		want   int
	}{
		{"192.168.0.10:1", nil, 200},
		{"10.0.0.5:1", nil, 200},
		{"100.101.102.103:1", nil, 200}, // Tailscale
		{"[::1]:1", nil, 200},
		{"8.8.8.8:1", nil, 403},
		// 反向代理在內網，但轉送的是外部請求
		{"192.168.1.2:1", map[string]string{"X-Forwarded-For": "203.0.113.5"}, 403},
		{"192.168.1.2:1", map[string]string{"X-Real-IP": "203.0.113.5"}, 403},
	}
	for _, c := range cases {
		if got := request(h, "/dashboard/data", c.remote, c.header).Code; got != c.want {
			t.Errorf("%s %v: status %d, want %d", c.remote, c.header, got, c.want)
		}
	}
}
