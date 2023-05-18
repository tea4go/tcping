package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/cloverstd/tcping/ping"
	"github.com/cloverstd/tcping/ping/http"
	"github.com/cloverstd/tcping/ping/tcp"
	"github.com/spf13/cobra"
)

var (
	showVersion bool
	version     string
	counter     int
	timeout     string
	interval    string
	sigs        chan os.Signal

	httpMethod string
	httpUA     string

	dnsServer []string
)

var rootCmd = cobra.Command{
	Use:   "tcping host port",
	Short: "tcping is a tcp ping",
	Long:  "tcping is a ping over tcp connection",
	Example: `
  1. 通过 TCP ping
	> tcping google.com
  2. 使用自定义端口通过 TCP ping
	> tcping --tls 10.45.52.153 40083
  3. 通过 Http ping
  	> tcping http://google.com
  4. 通过 Https ping
  	> tcping https://cn.bing.com/
  5. 通过代理 Http ping
  	> tcping --proxy http://192.168.3.8:32121 http://google.com
	`,
	Run: func(cmd *cobra.Command, args []string) {
		if showVersion {
			fmt.Printf("Version: %s\n", version)
			return
		}
		if len(args) == 0 {
			cmd.Usage()
			return
		}
		if len(args) > 2 {
			cmd.Println("无效的命令参数!")
			return
		}

		url, err := ping.ParseAddress(args[0])
		if err != nil {
			fmt.Printf("%s 是一个无效的目。\n", args[0])
			return
		}

		defaultPort := "80"
		if port := url.Port(); port != "" {
			defaultPort = port
		} else if url.Scheme == "https" {
			defaultPort = "443"
		}
		if len(args) > 1 {
			defaultPort = args[1]
		}
		port, err := strconv.Atoi(defaultPort)
		if err != nil {
			cmd.Printf("%s 是一个无效的端口。\n", defaultPort)
			return
		}
		url.Host = fmt.Sprintf("%s:%d", url.Hostname(), port)

		timeoutDuration, err := ping.ParseDuration(timeout)
		if err != nil {
			cmd.Println("解析超时失败，", err)
			cmd.Usage()
			return
		}

		intervalDuration, err := ping.ParseDuration(interval)
		if err != nil {
			cmd.Println("解析间隔失败，", err)
			cmd.Usage()
			return
		}

		protocol, err := ping.NewProtocol(url.Scheme)
		if err != nil {
			cmd.Println("无效协议，", err)
			cmd.Usage()
			return
		}

		option := ping.Option{
			Timeout: timeoutDuration,
		}
		if len(dnsServer) != 0 {
			option.Resolver = &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (conn net.Conn, err error) {
					for _, addr := range dnsServer {
						if conn, err = net.Dial("udp", addr+":53"); err != nil {
							continue
						} else {
							return conn, nil
						}
					}
					return
				},
			}
		}
		pingFactory := ping.Load(protocol)
		p, err := pingFactory(url, &option)
		if err != nil {
			cmd.Println("加载执行器(pinger)失败，", err)
			cmd.Usage()
			return
		}

		pinger := ping.NewPinger(os.Stdout, url, p, intervalDuration, counter)
		sigs = make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go pinger.Ping()
		select {
		case <-sigs:
		case <-pinger.Done():
		}
		pinger.Stop()
		pinger.Summarize()
	},
}

func fixProxy(proxy string, op *ping.Option) error {
	if proxy == "" {
		return nil
	}
	u, err := url.Parse(proxy)
	op.Proxy = u
	return err
}

func init() {
	version = "v0.1.2"
	rootCmd.Flags().StringVar(&httpMethod, "http-method", "GET", `在 http 模式下使用自定义 HTTP 方法而不是 GET。`)
	ua := rootCmd.Flags().String("user-agent", "tcping", `在 http 模式下使用自定义 UA。`)
	meta := rootCmd.Flags().Bool("meta", false, `带有元信息。`)
	tls := rootCmd.Flags().Bool("tls", false, `是否TLS。`)
	proxy := rootCmd.Flags().String("proxy", "", "使用 HTTP 代理。")

	ping.Register(ping.HTTP, func(url *url.URL, op *ping.Option) (ping.Ping, error) {
		if err := fixProxy(*proxy, op); err != nil {
			return nil, err
		}
		op.UA = *ua
		return http.New(httpMethod, url.String(), op, *meta)
	})
	ping.Register(ping.HTTPS, func(url *url.URL, op *ping.Option) (ping.Ping, error) {
		if err := fixProxy(*proxy, op); err != nil {
			return nil, err
		}
		op.UA = *ua
		return http.New(httpMethod, url.String(), op, *meta)
	})
	ping.Register(ping.TCP, func(url *url.URL, op *ping.Option) (ping.Ping, error) {
		port, err := strconv.Atoi(url.Port())
		if err != nil {
			return nil, err
		}
		return tcp.New(url.Hostname(), port, op, *tls), nil
	})
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "显示版本并退出。")
	rootCmd.Flags().IntVarP(&counter, "counter", "c", ping.DefaultCounter, "Ping的次数。")
	rootCmd.Flags().StringVarP(&timeout, "timeout", "T", "3s", `连接超时，单位是 "ns 纳秒", "us|µs 微秒", "ms 毫秒", "s 秒", "m 分", "h 时"`)
	rootCmd.Flags().StringVarP(&interval, "interval", "I", "1s", `Ping的间隔，单位是 "ns 纳秒", "us|µs 微秒", "ms 毫秒", "s 秒", "m 分", "h 时"`)

	rootCmd.Flags().StringArrayVarP(&dnsServer, "dns-server", "D", nil, `使用指定的 DNS 解析服务器。`)

}

func main() {

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
