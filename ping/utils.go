package ping

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// FormatIP - trim spaces and format IP.
//
// # IP - the provided IP
//
// string - return "" if the input is neither valid IPv4 nor valid IPv6
//
//	return IPv4 in format like "192.168.9.1"
//	return IPv6 in format like "[2002:ac1f:91c5:1::bd59]"
func FormatIP(IP string) (string, error) {

	host := strings.Trim(IP, "[ ]")
	if parseIP := net.ParseIP(host); parseIP != nil {
		// valid ip
		if parseIP.To4() == nil {
			// ipv6
			host = fmt.Sprintf("[%s]", host)
		}
		return host, nil
	}
	return "", fmt.Errorf("error IP format")
}

// ParseDuration parse the t as time.Duration, it will parse t as mills when missing unit.
func ParseDuration(t string) (time.Duration, error) {
	if timeout, err := strconv.ParseInt(t, 10, 64); err == nil {
		return time.Duration(timeout) * time.Millisecond, nil
	}
	return time.ParseDuration(t)
}

// ParseAddress will try to parse addr as url.URL.
func ParseAddress(addr string) (*url.URL, error) {
	if strings.Contains(addr, "://") {
		// it maybe with scheme, try url.Parse
		return url.Parse(addr)
	}
	return url.Parse("tcp://" + addr)
}

func FormatError(err error) string {
	//fmt.Println("===>", err.Error())
	switch err := err.(type) {
	case *url.Error:
		if err.Timeout() {
			return "连接超时"
		}
		return FormatError(err.Err)
	case net.Error:
		if err.Timeout() {
			return "连接超时"
		}
		if oe, ok := err.(*net.OpError); ok {
			switch err := oe.Err.(type) {
			case *os.SyscallError:
				return FormatError(err.Err)
			}
		}
	default:
		if errors.Is(err, context.DeadlineExceeded) {
			return "连接超时"
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
