package vpnserver

import "testing"

func TestResolveEndpointHostPort_EmptyHostUsesFallback(t *testing.T) {
	host, port := resolveEndpointHostPort("", 443)
	if host != "127.0.0.1" {
		t.Fatalf("got host %q, want %q", host, "127.0.0.1")
	}
	if port != 443 {
		t.Fatalf("got port %d, want %d", port, 443)
	}
}

func TestResolveEndpointHostPort_HostWithPort(t *testing.T) {
	host, port := resolveEndpointHostPort("vpn.example.com:8443", 443)
	if host != "vpn.example.com" {
		t.Fatalf("got host %q, want %q", host, "vpn.example.com")
	}
	if port != 8443 {
		t.Fatalf("got port %d, want %d", port, 8443)
	}
}

func TestBuildClientConfigMap_UsesTunAddressList(t *testing.T) {
	cfg := Config{
		EndpointHost:      "vpn.example.com",
		ListenPort:        8443,
		WebsocketPath:     "/vpn",
		TLSServerName:     "vpn.example.com",
		ClientInsecureTLS: true,
		ClientTunName:     "sb-tun",
		ClientTunCIDR:     "172.19.0.1/30",
	}
	client := Client{UUID: "11111111-1111-1111-1111-111111111111"}

	built := buildClientConfigMap(cfg, client)
	inbounds, ok := built["inbounds"].([]any)
	if !ok || len(inbounds) == 0 {
		t.Fatalf("inbounds is missing or invalid: %T", built["inbounds"])
	}
	tunInbound, ok := inbounds[0].(map[string]any)
	if !ok {
		t.Fatalf("tun inbound has unexpected type: %T", inbounds[0])
	}

	if _, exists := tunInbound["inet4_address"]; exists {
		t.Fatalf("legacy tun field inet4_address must not be generated")
	}

	addresses, ok := tunInbound["address"].([]string)
	if !ok {
		t.Fatalf("address has unexpected type: %T", tunInbound["address"])
	}
	if len(addresses) != 1 || addresses[0] != "172.19.0.1/30" {
		t.Fatalf("unexpected tun addresses: %#v", addresses)
	}
	if _, exists := tunInbound["route_exclude_address"]; exists {
		t.Fatalf("route_exclude_address must not be set for domain endpoint")
	}
	if tunInbound["sniff"] != true {
		t.Fatalf("tun sniff must be enabled, got %#v", tunInbound["sniff"])
	}

	dnsCfg, ok := built["dns"].(map[string]any)
	if !ok {
		t.Fatalf("dns config is missing or invalid: %T", built["dns"])
	}
	if dnsCfg["final"] != "dns-remote" {
		t.Fatalf("dns final must be dns-remote, got %#v", dnsCfg["final"])
	}

	servers, ok := dnsCfg["servers"].([]any)
	if !ok || len(servers) < 1 {
		t.Fatalf("dns servers are missing or invalid: %#v", dnsCfg["servers"])
	}

	firstServer, ok := servers[0].(map[string]any)
	if !ok {
		t.Fatalf("dns server has unexpected type: %T", servers[0])
	}
	if firstServer["tag"] != "dns-remote" {
		t.Fatalf("first dns server tag mismatch: %#v", firstServer["tag"])
	}
	if firstServer["type"] != "udp" {
		t.Fatalf("first dns server type mismatch: %#v", firstServer["type"])
	}
	if firstServer["server"] != "1.1.1.1" {
		t.Fatalf("first dns server address mismatch: %#v", firstServer["server"])
	}
	if firstServer["detour"] != "vless-out" {
		t.Fatalf("dns remote detour mismatch: %#v", firstServer["detour"])
	}

	outbounds, ok := built["outbounds"].([]any)
	if !ok || len(outbounds) < 3 {
		t.Fatalf("outbounds is missing or invalid: %T", built["outbounds"])
	}
	directOutbound, ok := outbounds[1].(map[string]any)
	if !ok {
		t.Fatalf("direct outbound has unexpected type: %T", outbounds[1])
	}
	if directOutbound["tag"] != "direct" || directOutbound["type"] != "direct" {
		t.Fatalf("direct outbound mismatch: %#v", directOutbound)
	}

	routeCfg, ok := built["route"].(map[string]any)
	if !ok {
		t.Fatalf("route config is missing or invalid: %T", built["route"])
	}
	resolver, ok := routeCfg["default_domain_resolver"].(map[string]any)
	if !ok {
		t.Fatalf("default_domain_resolver is missing or invalid: %T", routeCfg["default_domain_resolver"])
	}
	if resolver["server"] != "dns-remote" {
		t.Fatalf("default_domain_resolver server mismatch: %#v", resolver["server"])
	}
	rules, ok := routeCfg["rules"].([]any)
	if !ok || len(rules) < 2 {
		t.Fatalf("route rules are missing or invalid: %#v", routeCfg["rules"])
	}

	dnsRule, ok := rules[0].(map[string]any)
	if !ok {
		t.Fatalf("dns route rule has unexpected type: %T", rules[0])
	}
	if dnsRule["protocol"] != "dns" || dnsRule["action"] != "hijack-dns" {
		t.Fatalf("dns route rule mismatch: %#v", dnsRule)
	}
}

func TestBuildClientConfigMap_AddsRouteExcludeAddressForIPEndpoint(t *testing.T) {
	cfg := Config{
		EndpointHost:      "111.88.141.226:8443",
		ListenPort:        8443,
		WebsocketPath:     "/vpn",
		TLSServerName:     "111.88.141.226",
		ClientInsecureTLS: true,
		ClientTunName:     "sb-tun",
		ClientTunCIDR:     "172.19.0.1/30",
	}
	client := Client{UUID: "11111111-1111-1111-1111-111111111111"}

	built := buildClientConfigMap(cfg, client)
	inbounds, ok := built["inbounds"].([]any)
	if !ok || len(inbounds) == 0 {
		t.Fatalf("inbounds is missing or invalid: %T", built["inbounds"])
	}
	tunInbound, ok := inbounds[0].(map[string]any)
	if !ok {
		t.Fatalf("tun inbound has unexpected type: %T", inbounds[0])
	}

	excluded, ok := tunInbound["route_exclude_address"].([]string)
	if !ok {
		t.Fatalf("route_exclude_address has unexpected type: %T", tunInbound["route_exclude_address"])
	}
	if len(excluded) != 1 || excluded[0] != "111.88.141.226/32" {
		t.Fatalf("unexpected route_exclude_address: %#v", excluded)
	}
}

func TestSplitAndTrimCSV(t *testing.T) {
	values := splitAndTrimCSV(" 172.19.0.1/30 , ,10.10.0.1/30 ")
	if len(values) != 2 {
		t.Fatalf("unexpected values count: %d", len(values))
	}
	if values[0] != "172.19.0.1/30" || values[1] != "10.10.0.1/30" {
		t.Fatalf("unexpected parsed values: %#v", values)
	}
}
