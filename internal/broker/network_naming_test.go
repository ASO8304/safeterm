package broker

import (
	"strings"
	"testing"
)

func TestNetworkNamingHelpIsReadOnlyAndUsesTypedRequest(t *testing.T) {
	s, api, out := uxNetworkFixture(t)
	editorCommands(t, s, "execute network rename ?")
	if len(api.requests) != 0 || !strings.Contains(out.String(), "CURRENT NEW") || !strings.Contains(out.String(), "no live rename") {
		t.Fatal("naming help executed or omitted effects", out)
	}
	editorCommands(t, s, "execute network rename ip1 lan0")
	if len(api.requests) != 1 || api.requests[0].Action != "rename" || api.requests[0].Interface != "ip1" || api.requests[0].NewName != "lan0" {
		t.Fatal("naming request lost its typed fields", api.requests)
	}
}

func TestInterfaceMACAndPersistentNameAreVisibleWithoutMutation(t *testing.T) {
	s, api, out := uxNetworkFixture(t)
	api.view.MACs = map[string]string{"ip0": "00:0c:29:ef:e4:51", "ip1": "00:0c:29:ef:e4:52"}
	api.view.Names = map[string]string{"ip1": "lan0"}
	editorCommands(t, s, "config network interface", "show")
	if !strings.Contains(out.String(), "MAC ADDRESS") || !strings.Contains(out.String(), "00:0c:29:ef:e4:52") {
		t.Fatal("MAC missing from table", out)
	}
	out.Reset()
	editorCommands(t, s, "edit ip1", "show")
	for _, value := range []string{"MAC Address:", "00:0c:29:ef:e4:52", "Persistent Name:", "lan0 (next reboot/device activation)"} {
		if !strings.Contains(out.String(), value) {
			t.Fatal("naming metadata missing", value, out)
		}
	}
	if len(api.requests) != 1 || s.editorDirty() {
		t.Fatal("identity display wrote configuration")
	}
}
