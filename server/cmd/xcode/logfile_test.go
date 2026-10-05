package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "xcode.log")
	l, err := openRotatingLog(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Repeat("x", 39) + "\n" // 40 bytes
	for i := 0; i < 3; i++ {
		l.Write([]byte(line))
	}
	l.Close()
	cur, _ := os.ReadFile(path)
	old, _ := os.ReadFile(path + ".1")
	if len(old) != 80 || len(cur) != 40 {
		t.Fatalf("old=%d cur=%d, want 80 and 40", len(old), len(cur))
	}

	// 重新開啟時接著寫，不覆蓋
	l, _ = openRotatingLog(path, 100)
	l.Write([]byte(line))
	l.Close()
	if cur, _ := os.ReadFile(path); len(cur) != 80 {
		t.Fatalf("after reopen = %d bytes, want 80", len(cur))
	}
}

func TestDashboardURL(t *testing.T) {
	for _, c := range []struct{ addr, want string }{
		{"[::]:8097", "http://127.0.0.1:8097/dashboard"},
		{"0.0.0.0:8097", "http://127.0.0.1:8097/dashboard"},
		{"192.168.1.2:9000", "http://192.168.1.2:9000/dashboard"},
	} {
		a, err := net.ResolveTCPAddr("tcp", c.addr)
		if err != nil {
			t.Fatal(err)
		}
		if got := dashboardURL(a); got != c.want {
			t.Errorf("%s: got %s, want %s", c.addr, got, c.want)
		}
	}
}
