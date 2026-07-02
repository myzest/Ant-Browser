package proxy

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const chainSocks5Prefix = "chain+socks5://"

type chainSocks5Hop struct {
	Protocol string `json:"protocol"`
	Server   string `json:"server"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type chainClashHop struct {
	Type string                 `json:"type"`
	Name string                 `json:"name,omitempty"`
	Node map[string]interface{} `json:"node"`
}

type chainSocks5Config struct {
	LocalPort int             `json:"localPort,omitempty"`
	First     json.RawMessage `json:"first"`
	Second    chainSocks5Hop  `json:"second"`

	FirstHop      chainSocks5Hop
	FirstClashHop *chainClashHop
}

func IsChainSocks5Proxy(src string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(src)), chainSocks5Prefix)
}

func ParseChainSocks5Config(src string) (*chainSocks5Config, error) {
	raw := strings.TrimSpace(src)
	if !IsChainSocks5Proxy(raw) {
		return nil, fmt.Errorf("不是链式代理配置")
	}
	encoded := raw[len(chainSocks5Prefix):]
	if strings.TrimSpace(encoded) == "" {
		return nil, fmt.Errorf("链式代理配置为空")
	}

	decoded, err := url.QueryUnescape(encoded)
	if err != nil {
		return nil, fmt.Errorf("链式代理配置解码失败: %w", err)
	}

	var cfg chainSocks5Config
	if err := json.Unmarshal([]byte(decoded), &cfg); err != nil {
		return nil, fmt.Errorf("链式代理配置 JSON 解析失败: %w", err)
	}
	if len(cfg.First) == 0 || strings.TrimSpace(string(cfg.First)) == "null" {
		return nil, fmt.Errorf("第一层代理配置不能为空")
	}

	firstHop, firstClashHop, err := parseChainFirstHop(cfg.First)
	if err != nil {
		return nil, err
	}
	if firstClashHop != nil {
		cfg.FirstClashHop = firstClashHop
	} else {
		cfg.FirstHop = firstHop
	}

	if err := validateChainSocks5Hop("第二层", cfg.Second); err != nil {
		return nil, err
	}
	if cfg.LocalPort < 0 || cfg.LocalPort > 65535 {
		return nil, fmt.Errorf("本地监听端口必须在 1-65535 之间")
	}
	cfg.FirstHop.Protocol = normalizeChainHopProtocol(cfg.FirstHop.Protocol)
	cfg.Second.Protocol = normalizeChainHopProtocol(cfg.Second.Protocol)
	return &cfg, nil
}

func parseChainFirstHop(raw json.RawMessage) (chainSocks5Hop, *chainClashHop, error) {
	var probe struct {
		Type     string `json:"type"`
		Protocol string `json:"protocol"`
	}
	_ = json.Unmarshal(raw, &probe)
	if strings.EqualFold(strings.TrimSpace(probe.Type), "clash") || strings.EqualFold(strings.TrimSpace(probe.Protocol), "clash") {
		var clashHop chainClashHop
		if err := json.Unmarshal(raw, &clashHop); err != nil {
			return chainSocks5Hop{}, nil, fmt.Errorf("第一层 Clash 配置解析失败: %w", err)
		}
		clashHop.Type = "clash"
		if clashHop.Node == nil {
			return chainSocks5Hop{}, nil, fmt.Errorf("第一层 Clash 配置缺少 node")
		}
		if err := validateChainClashHop(clashHop); err != nil {
			return chainSocks5Hop{}, nil, err
		}
		return chainSocks5Hop{}, &clashHop, nil
	}

	var hop chainSocks5Hop
	if err := json.Unmarshal(raw, &hop); err != nil {
		return chainSocks5Hop{}, nil, fmt.Errorf("第一层代理配置解析失败: %w", err)
	}
	if err := validateChainSocks5Hop("第一层", hop); err != nil {
		return chainSocks5Hop{}, nil, err
	}
	hop.Protocol = normalizeChainHopProtocol(hop.Protocol)
	return hop, nil, nil
}

func validateChainClashHop(hop chainClashHop) error {
	nodeType := strings.ToLower(strings.TrimSpace(getMapString(hop.Node, "type")))
	if nodeType == "" {
		return fmt.Errorf("第一层 Clash 节点缺少 type")
	}
	if strings.TrimSpace(getMapString(hop.Node, "server")) == "" {
		return fmt.Errorf("第一层 Clash 节点缺少 server")
	}
	port := getMapInt(hop.Node, "port")
	if port < 1 || port > 65535 {
		return fmt.Errorf("第一层 Clash 节点 port 必须在 1-65535 之间")
	}
	switch nodeType {
	case "http", "socks5", "vmess", "vless", "trojan", "ss", "shadowsocks":
		return nil
	case "ssr":
		return fmt.Errorf("第一层 Clash 节点不支持 ShadowsocksR")
	case "hysteria", "hysteria2":
		return fmt.Errorf("第一层 Clash 节点暂不支持 hysteria/hysteria2 链式前置")
	default:
		return fmt.Errorf("第一层 Clash 节点不支持的类型: %s", nodeType)
	}
}

func normalizeChainHopProtocol(protocol string) string {
	normalized := strings.ToLower(strings.TrimSpace(protocol))
	if normalized == "" {
		return "socks5"
	}
	return normalized
}

func validateChainSocks5Hop(label string, hop chainSocks5Hop) error {
	if strings.TrimSpace(hop.Server) == "" {
		return fmt.Errorf("%s代理地址不能为空", label)
	}
	if hop.Port < 1 || hop.Port > 65535 {
		return fmt.Errorf("%s代理端口必须在 1-65535 之间", label)
	}
	protocol := strings.ToLower(strings.TrimSpace(hop.Protocol))
	if protocol != "" && protocol != "socks5" && protocol != "http" {
		return fmt.Errorf("%s协议仅支持 http 或 socks5", label)
	}
	if strings.TrimSpace(hop.Password) != "" && strings.TrimSpace(hop.Username) == "" {
		return fmt.Errorf("%s填写密码时请同时填写账号", label)
	}
	return nil
}

// ParseProxyNode 解析代理节点
func ParseProxyNode(node string) (string, map[string]interface{}, error) {
	src := strings.TrimSpace(node)
	if src == "" {
		return "", nil, fmt.Errorf("代理节点为空")
	}
	l := strings.ToLower(src)
	if strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "socks5://") {
		return src, nil, nil
	}
	if strings.HasPrefix(l, "clash://") || strings.Contains(l, "type:") || strings.Contains(l, "proxies:") {
		outbound, standard, err := parseClashNode(src)
		if err != nil {
			return "", nil, err
		}
		if standard != "" {
			return standard, nil, nil
		}
		if outbound != nil {
			return "", outbound, nil
		}
	}
	outbound, err := buildXrayOutbound(src)
	if err != nil {
		return "", nil, err
	}
	return "", outbound, nil
}

func buildXrayOutbound(node string) (map[string]interface{}, error) {
	l := strings.ToLower(node)
	if strings.HasPrefix(l, "vmess://") {
		return buildOutboundVmess(node)
	}
	if strings.HasPrefix(l, "vless://") {
		return buildOutboundVless(node)
	}
	if strings.HasPrefix(l, "trojan://") {
		return buildOutboundTrojan(node)
	}
	if strings.HasPrefix(l, "ss://") {
		return buildOutboundSS(node)
	}
	if strings.HasPrefix(l, "ssr://") {
		return nil, fmt.Errorf("不支持 ShadowsocksR 协议，Xray 不支持 SSR，请使用 SS/vmess/vless/trojan")
	}
	if strings.HasPrefix(l, "hysteria2://") || strings.HasPrefix(l, "hysteria://") {
		return buildOutboundHysteria2(node)
	}
	return nil, fmt.Errorf("不支持的节点协议")
}
