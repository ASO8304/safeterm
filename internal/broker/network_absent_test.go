package broker

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"safeterm/internal/networkmanager"
)

func TestAbsentInterfaceCleanupIsExplicitLocalAndReviewed(t *testing.T) {
	s, api, out := uxNetworkFixture(t)
	api.view.AbsentInterfaces = map[string]networkmanager.Interface{
		"old0":    {Name: "old0", ID: "old0", Addresses: []string{"192.0.2.10/24"}, Files: []string{"etc/netplan/01-netcfg.yaml"}},
		"denied0": {Name: "denied0", ID: "denied0", ReadOnly: "source filename is outside administrator policy"},
	}
	editorCommands(t, s, "config network interface", "show", "show absent", "delete ?")
	for _, want := range []string{"retained on disk", "old0", "/etc/netplan/01-netcfg.yaml", "source filename is outside administrator policy"} {
		if !strings.Contains(out.String(), want) {
			t.Fatal("cleanup inventory or policy missing", want, out)
		}
	}
	if got := s.completionCandidates(context.Background(), "delete ", ""); !reflect.DeepEqual(got, []string{"old0"}) {
		t.Fatal("delete suggested a live or denied device", got)
	}
	if _, err := s.executeLine(context.Background(), "delete ip1", nil); err == nil {
		t.Fatal("live interface deletion accepted")
	}
	if len(api.requests) != 1 || s.editorDirty() {
		t.Fatal("inventory/completion changed configuration")
	}
	out.Reset()
	editorCommands(t, s, "delete old0", "diff", "show")
	if len(api.requests) != 1 || !s.editorDirty() || len(api.view.AbsentInterfaces) != 2 || !strings.Contains(out.String(), "Delete on confirmation") || strings.Contains(out.String(), "No unsaved interface edits") {
		t.Fatal("deletion not staged visibly and locally", out)
	}
	editorCommands(t, s, "cancel", "config network interface")
	if len(s.networkEdit.view.AbsentInterfaces) != 2 || s.editorDirty() {
		t.Fatal("cancel saved deletion")
	}
	editorCommands(t, s, "delete old0", "save")
	if _, retained := api.view.AbsentInterfaces["old0"]; retained || len(api.view.Interfaces) != 2 {
		t.Fatal("save lost cleanup or affected live interfaces", api.view)
	}
}
