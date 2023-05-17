package http_test

import (
	"context"
	"testing"

	tcping "github.com/cloverstd/tcping/ping"
	"github.com/cloverstd/tcping/ping/http"
)

func TestPing(t *testing.T) {
	ping, err := http.New("GET", "https://ssodr.iwhalecloud.com:40083/login/", &tcping.Option{}, false)
	if err != nil {
		t.Fatal(err)
	}

	stats := ping.Ping(context.Background())
	if !stats.Connected {
		t.Fatal(stats.Error)
	}
	stats = ping.Ping(context.Background())
	if !stats.Connected {
		t.Fatal(stats.Error)
	}
}

func TestPingHTTPS(t *testing.T) {
	ping, err := http.New("GET", "https://ssodr1.iwhalecloud.com:40083/login/", &tcping.Option{}, false)
	if err != nil {
		t.Fatal(err)
	}

	stats := ping.Ping(context.Background())
	if !stats.Connected {
		t.Fatal(stats.Error)
	}
	stats = ping.Ping(context.Background())
	if !stats.Connected {
		t.Fatal(stats.Error)
	}
}

func TestPingRedirect(t *testing.T) {
	ping, err := http.New("GET", "http://github.com", &tcping.Option{}, false)
	if err != nil {
		t.Fatal(err)
	}

	stats := ping.Ping(context.Background())
	if !stats.Connected {
		t.Fatal(stats.Error)
	}
	status := stats.Meta["status"].(http.Int)
	if status != 301 {
		t.Fatal("it should not be redirect")
	}
}
