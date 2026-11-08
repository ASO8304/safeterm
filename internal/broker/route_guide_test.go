package broker

import (
	"context"
	"strings"
	"testing"

	"safeterm/internal/networkmanager"
)

func guideInput(value string) <-chan byte {
	ch := make(chan byte, len(value))
	for _, b := range []byte(value) {
		ch <- b
	}
	close(ch)
	return ch
}
func TestGuidedRoutesValidateAndRemainUnsavedUntilSave(t *testing.T) {
	s, api, out := uxNetworkFixture(t)
	editorCommands(t, s, "config network route")
	_, err := s.executeLine(context.Background(), "add", guideInput("bad\r198.51.100.0/24\rdirect\rip1\rauto\r"))
	if err != nil {
		t.Fatal(err)
	}
	name := s.networkEdit.item
	r := s.networkEdit.view.Routes[name]
	if r.Destination != "198.51.100.0/24" || r.Interface != "ip1" || r.Gateway != "" || r.Metric != nil || !s.editorDirty() {
		t.Fatal("incorrect guided draft", r)
	}
	if len(api.requests) != 1 || !strings.Contains(out.String(), "Draft ready") {
		t.Fatal("guide saved or applied networking", api.requests, out)
	}
	editorCommands(t, s, "set metric 42", "next")
	_, err = s.executeLine(context.Background(), "edit 1", guideInput("\r\r\r\r"))
	if err != nil {
		t.Fatal(err)
	}
	if *s.networkEdit.view.Routes[name].Metric != 42 {
		t.Fatal("edit lost default")
	}
	editorCommands(t, s, "save")
	if len(api.view.Routes) != 1 {
		t.Fatal("guided route did not save")
	}
}
func TestRouteGuideCancelPreservesExistingDraftAndDefaultProtection(t *testing.T) {
	s, api, _ := uxNetworkFixture(t)
	api.view.Routes["default"] = networkmanager.Route{Name: "default", Interface: "ip1", Destination: "0.0.0.0/0", Gateway: "192.0.2.1"}
	editorCommands(t, s, "config network route", "edit 1", "set metric 20")
	before := len(s.networkEdit.changes)
	for _, input := range []string{"198.51.100.0/24\rcancel\r", "\x03"} {
		_, err := s.executeLine(context.Background(), "add", guideInput(input))
		if err != nil || len(s.networkEdit.changes) != before || len(s.networkEdit.view.Routes) != 1 {
			t.Fatal("cancel changed prior draft", err)
		}
	}
	_, err := s.executeLine(context.Background(), "edit 1", guideInput("\r\r\r\r"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.executeLine(context.Background(), "delete 1", nil); err == nil {
		t.Fatal("guide bypassed default-route protection")
	}
}
