package pdfgen

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"time"
)

// validateImageURL 校验远程图片 URL：协议与目标地址必须安全。
// 仅允许 http/https；若直接以 IP 作为主机名，则拒绝回环/私网等 SSRF 高危地址。
func validateImageURL(u *url.URL) error {
	if u == nil {
		return errors.New("无效 URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("不支持的协议")
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("无效的 URL")
	}
	if ip := net.ParseIP(host); ip != nil && isForbiddenIP(ip) {
		return errors.New("SSRF 已拦截: 目标地址不可访问")
	}
	return nil
}

// safeDialContext 解析目标主机并在拨号前校验所有解析 IP，防止 SSRF/DNS 重绑定。
// 任一解析结果落入内部/特殊地址段即整体拒绝，避免 DNS 多值绕过。
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	d := net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	for _, ip := range ips {
		if isForbiddenIP(ip.IP) {
			return nil, fmt.Errorf("SSRF 已拦截: 目标地址 %s 不可访问", ip.IP)
		}
	}
	var lastErr error
	for _, ip := range ips {
		conn, dialErr := d.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	return nil, lastErr
}

// isForbiddenIP 判定 IP 是否属于回环/私网/链路本地等 SSRF 高危地址段。
func isForbiddenIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		// 100.64.0.0/10 CGNAT
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
		// 198.18.0.0/15 基准测试网段
		if ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19) {
			return true
		}
	}
	return false
}
