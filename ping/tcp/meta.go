package tcp

import (
	"fmt"
	"strings"
	"time"
)

var _ fmt.Stringer = (*Meta)(nil)

type Meta struct {
	version    int
	dnsNames   []string
	serverName string
	notBefore  time.Time
	notAfter   time.Time
}

func (m Meta) String() string {
	return fmt.Sprintf(
		"server_name=%s version=%d dns_names=%s (%s~%s)",
		m.serverName,
		m.version,
		strings.Join(m.dnsNames, ","),
		formatTime(m.notBefore),
		formatTime(m.notAfter),
	)
}

func formatTime(t time.Time) string {
	return t.Format("2006-01-02")
}
