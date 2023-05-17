package ping

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"math"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var pinger = map[Protocol]Factory{}

type Factory func(url *url.URL, op *Option) (Ping, error)

func Register(protocol Protocol, factory Factory) {
	pinger[protocol] = factory
}

func Load(protocol Protocol) Factory {
	return pinger[protocol]
}

// Protocol ...
type Protocol int

func (protocol Protocol) String() string {
	switch protocol {
	case TCP:
		return "tcp"
	case HTTP:
		return "http"
	case HTTPS:
		return "https"
	}
	return "unknown"
}

const (
	// TCP is tcp protocol
	TCP Protocol = iota
	// HTTP is http protocol
	HTTP
	// HTTPS is https protocol
	HTTPS
)

// NewProtocol convert protocol string to Protocol
func NewProtocol(protocol string) (Protocol, error) {
	switch strings.ToLower(protocol) {
	case TCP.String():
		return TCP, nil
	case HTTP.String():
		return HTTP, nil
	case HTTPS.String():
		return HTTPS, nil
	}
	return 0, fmt.Errorf("protocol %s not support", protocol)
}

type Option struct {
	Timeout  time.Duration //连接超时
	Resolver *net.Resolver // 自定义DNS域名解析
	Proxy    *url.URL      // Http代理(格式：http://192.168.3.157:32126）
	UA       string        // 浏览器UA标识
}

// Target is a ping
type Target struct {
	Protocol Protocol
	Host     string
	IP       string
	Port     int
	Proxy    string

	Counter  int
	Interval time.Duration
	Timeout  time.Duration
}

func (target Target) String() string {
	return fmt.Sprintf("%s://%s:%d", target.Protocol, target.Host, target.Port)
}

type Stats struct {
	Connected   bool                    `json:"connected"`
	Error       error                   `json:"error"`
	Duration    time.Duration           `json:"duration"`
	DNSDuration time.Duration           `json:"DNSDuration"`
	Address     string                  `json:"address"`
	Meta        map[string]fmt.Stringer `json:"meta"`
	Extra       fmt.Stringer            `json:"extra"`
}

func (s *Stats) FormatMeta() string {
	keys := make([]string, 0, len(s.Meta))
	for key := range s.Meta {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for i, key := range keys {
		builder.WriteString(key)
		builder.WriteString("=")
		builder.WriteString(s.Meta[key].String())
		if i < len(keys)-1 {
			builder.WriteString(" ")
		}
	}
	return builder.String()
}

type Ping interface {
	Ping(ctx context.Context) *Stats
}

func NewPinger(out io.Writer, url *url.URL, ping Ping, interval time.Duration, counter int) *Pinger {
	return &Pinger{
		stopC:    make(chan struct{}),
		counter:  counter,
		interval: interval,
		out:      out,
		url:      url,
		ping:     ping,
	}
}

type Pinger struct {
	ping Ping

	stopOnce sync.Once
	stopC    chan struct{}

	out io.Writer

	url *url.URL

	interval time.Duration
	counter  int

	minDuration   time.Duration
	maxDuration   time.Duration
	totalDuration time.Duration
	total         int
	failedTotal   int
}

func (p *Pinger) Stop() {
	p.stopOnce.Do(func() {
		close(p.stopC)
	})
}

func (p *Pinger) Done() <-chan struct{} {
	return p.stopC
}

func (p *Pinger) Ping() {
	defer p.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-p.Done()
		cancel()
	}()

	interval := DefaultInterval
	if p.interval > 0 {
		interval = p.interval
	}
	timer := time.NewTimer(1)
	defer timer.Stop()

	stop := false
	p.minDuration = time.Duration(math.MaxInt64)
	for !stop {
		select {
		case <-timer.C:
			stats := p.ping.Ping(ctx)
			p.logStats(stats)
			if p.total++; p.counter > 0 && p.total > p.counter-1 {
				stop = true
			}
			timer.Reset(interval)
		case <-p.Done():
			stop = true
		}
	}
}

func (p *Pinger) Summarize() {

	const tpl = `
Ping statistics %s
	%d probes sent.
	%d successful, %d failed.
Approximate trip times:
	Minimum = %s, Maximum = %s, Average = %s`

	_, _ = fmt.Fprintf(p.out, tpl, p.url.String(), p.total, p.total-p.failedTotal, p.failedTotal, p.minDuration, p.maxDuration, p.totalDuration/time.Duration(p.total))
}

func (p *Pinger) formatError(err error) string {
	//fmt.Println("===>", err.Error())
	switch err := err.(type) {
	case *url.Error:
		if err.Timeout() {
			return "超时"
		}
		return p.formatError(err.Err)
	case net.Error:
		if err.Timeout() {
			return "超时"
		}
		if oe, ok := err.(*net.OpError); ok {
			switch err := oe.Err.(type) {
			case *os.SyscallError:
				return p.formatError(err.Err)
			}
		}
	default:
		if errors.Is(err, context.DeadlineExceeded) {
			return "超时"
		}
	}

	if err == io.EOF {
		return "网络主动断开"
	}

	netErr, ok := err.(net.Error)
	if ok {
		if netErr.Timeout() {
			return "网络连接超时"
		}
		if netErr.Temporary() {
			return "网络临时错误"
		}
	}

	opErr, ok := netErr.(*net.OpError)
	if ok {

		switch t := opErr.Err.(type) {
		case *net.DNSError:
			return "域名解析错误"
		case *os.SyscallError:
			if errno, ok := t.Err.(syscall.Errno); ok {
				switch errno {
				case syscall.ECONNREFUSED:
					return fmt.Sprintf("连接被服务器拒绝")
				case syscall.ETIMEDOUT:
					return fmt.Sprintf("网络连接超时")
				}
			}
		}
	}

	if strings.Contains(err.Error(), "forcibly closed") {
		return "远程主机强行关闭了现有连接"
	}

	if strings.Contains(err.Error(), "because it doesn't contain any IP SANs") {
		return "无法验证证书"
	}

	if strings.Contains(err.Error(), "no such host") {
		return "无效域名"
	}
	if strings.Contains(err.Error(), "getaddrinfow") {
		return "域名解析错误"
	}

	if strings.Contains(err.Error(), "closed network connection") {
		return "使用已关闭的网络连接"
	}

	if strings.Contains(err.Error(), "connection refused") {
		return "连接被拒绝"
	}

	if strings.Contains(err.Error(), "server gave HTTP response to HTTPS client") {
		return "服务器需要https访问"
	}

	if strings.Contains(err.Error(), "x509: certificate is not valid") {
		return "无效的网站证书"
	}

	if strings.Contains(err.Error(), "x509: certificate is valid") {
		return "网站证书不匹配"
	}

	if strings.Contains(err.Error(), "actively refused it") {
		return "无法建立连接"
	}

	if strings.Contains(err.Error(), "was forcibly closed by the remote host") {
		return "远程主机强制关闭了现有连接"
	}

	return err.Error()
}

func (p *Pinger) logStats(stats *Stats) {
	if stats.Duration < p.minDuration {
		p.minDuration = stats.Duration
	}
	if stats.Duration > p.maxDuration {
		p.maxDuration = stats.Duration
	}
	p.totalDuration += stats.Duration
	if stats.Error != nil {
		p.failedTotal++
		if errors.Is(stats.Error, context.Canceled) {
			// ignore cancel
			return
		}
	}
	status := "Failed"
	if stats.Connected {
		status = "Connected"
	}

	if stats.Error != nil {
		_, _ = fmt.Fprintf(p.out, "Ping %s(%s) %s(%s) - time=%-10s dns=%-9s",
			p.url.String(), stats.Address, status, p.formatError(stats.Error), stats.Duration.String(), stats.DNSDuration)
	} else {
		_, _ = fmt.Fprintf(p.out, "Ping %s(%s) %s - time=%-10s dns=%-9s",
			p.url.String(), stats.Address, status, stats.Duration.String(), stats.DNSDuration)
	}
	if len(stats.Meta) > 0 {
		_, _ = fmt.Fprintf(p.out, " %s", stats.FormatMeta())
	}
	_, _ = fmt.Fprint(p.out, "\n")
	if stats.Extra != nil {
		_, _ = fmt.Fprintf(p.out, " %s\n", strings.TrimSpace(stats.Extra.String()))
	}
}

// Result ...
type Result struct {
	Counter        int
	SuccessCounter int
	Target         *Target

	MinDuration   time.Duration
	MaxDuration   time.Duration
	TotalDuration time.Duration
}

// Avg return the average time of ping
func (result Result) Avg() time.Duration {
	if result.SuccessCounter == 0 {
		return 0
	}
	return result.TotalDuration / time.Duration(result.SuccessCounter)
}

// Failed return failed counter
func (result Result) Failed() int {
	return result.Counter - result.SuccessCounter
}

func (result Result) String() string {
	const resultTpl = `
Ping statistics {{.Target}}
	{{.Counter}} probes sent.
	{{.SuccessCounter}} successful, {{.Failed}} failed.
Approximate trip times:
	Minimum = {{.MinDuration}}, Maximum = {{.MaxDuration}}, Average = {{.Avg}}`
	t := template.Must(template.New("result").Parse(resultTpl))
	res := bytes.NewBufferString("")
	_ = t.Execute(res, result)
	return res.String()
}
