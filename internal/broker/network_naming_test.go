package broker

import (
	"context"
	"strings"
	"testing"
)

func TestNetworkNamingIsStagedInInterfaceEditor(t *testing.T) {
	s, api, out := uxNetworkFixture(t)
	editorCommands(t, s, "execute network rename ?")
	if len(api.requests) != 0 || !strings.Contains(out.String(), "set name NEW_NAME") {
		t.Fatal("naming help executed or omitted effects", out)
	}
	if _, err := s.executeLine(context.Background(), "execute network rename ip1 lan0", nil); err == nil || len(api.requests) != 0 {
		t.Fatal("old execute command still persisted a name")
	}
	editorCommands(t, s, "config network interface", "edit ip1", "set name ?")
	out.Reset()
	editorCommands(t, s, "set name lan0", "show", "diff")
	if len(api.requests) != 1 || !s.editorDirty() || !strings.Contains(out.String(), "lan0 (next reboot/device activation) (staged)") || !strings.Contains(out.String(), "Persistent Name") {
		t.Fatal("name was not staged visibly", api.requests, out)
	}
	editorCommands(t, s, "exit", "edit ip1")
	if s.editorDirty() || s.networkEdit.view.Interfaces["ip1"].PersistentName != "" {
		t.Fatal("exit kept the unsaved name")
	}
	editorCommands(t, s, "set name lan0", "next", "save")
	last := api.requests[len(api.requests)-1]
	if len(api.requests) != 2 || last.Action != "save" || last.Changes[0].Field != "name" || last.Changes[0].Values[0] != "lan0" {
		t.Fatal("save lost the typed name change", api.requests)
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
