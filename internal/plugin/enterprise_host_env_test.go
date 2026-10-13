package plugin

import (
	"strings"
	"testing"
)

func TestPluginHostGetsItsClientsGatewayAddress(t *testing.T) {
	storeSandbox(t)
	t.Setenv("MAGPIE_ADDR", "127.0.0.1:39551")
	n := 0
	for _, v := range hostEnv([]string{"MAGPIE_ADDR=127.0.0.1:3425", "OTHER=value"}) {
		if strings.HasPrefix(v, "MAGPIE_ADDR=") {
			n++
			if v != "MAGPIE_ADDR=127.0.0.1:39551" {
				t.Fatal("plugin host points to the public gateway", v)
			}
		}
	}
	if n != 1 {
		t.Fatal("plugin host has ambiguous gateway addresses", n)
	}
}

func TestPluginHostFollowsAGatewayPortChange(t *testing.T) {
	storeSandbox(t)
	t.Setenv("MAGPIE_ADDR", "127.0.0.1:39552")
	h := &host{gatewayAddr: "127.0.0.1:39551"}
	if !h.proxyMoved() {
		t.Fatal("a plugin kept the old client address after a port change")
	}
}
