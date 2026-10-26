//go:build linux

package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"safeterm/internal/resources"
)

func TestResourceServiceChecksEveryGroupMember(t *testing.T) {
	for _, group := range []bool{false, true} {
		t.Run(fmt.Sprint(group), func(t *testing.T) {
			want := []string{"database.service"}
			service := &resources.Service{Unit: want[0]}
			if group {
				want = append(want, "api.service", "web.service")
				service = &resources.Service{Units: want}
			}
			var got []string
			err := checkResourceServiceWith(service, func(name string, args ...string) ([]byte, error) {
				if name != "/usr/bin/systemctl" || !reflect.DeepEqual(args[:len(args)-1], []string{"show", "--property=LoadState", "--value"}) {
					t.Fatalf("unexpected command %s %v", name, args)
				}
				got = append(got, args[len(args)-1])
				return []byte("loaded\n"), nil
			})
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("checked=%v want=%v err=%v", got, want, err)
			}
		})
	}
}

func TestResourceServiceCheckReportsFailedMember(t *testing.T) {
	for _, state := range []string{"not-found", "masked", ""} {
		t.Run(state, func(t *testing.T) {
			err := checkResourceServiceWith(&resources.Service{Units: []string{"db.service", "api.service", "web.service"}}, func(_ string, args ...string) ([]byte, error) {
				if args[len(args)-1] == "db.service" {
					return []byte("loaded\n"), nil
				}
				return []byte(state + "\n"), nil
			})
			if err == nil || !strings.Contains(err.Error(), `units[1] "api.service"`) || !strings.Contains(err.Error(), "LoadState=") {
				t.Fatalf("missing member diagnostics: %v", err)
			}
		})
	}
	err := checkResourceServiceWith(&resources.Service{Unit: "api.service"}, func(string, ...string) ([]byte, error) {
		return nil, fmt.Errorf("systemd unavailable")
	})
	if err == nil || !strings.Contains(err.Error(), `unit "api.service"`) || !strings.Contains(err.Error(), "systemd unavailable") {
		t.Fatalf("missing command diagnostics: %v", err)
	}
}

func TestResourceServiceCheckRejectsInvalidPlansBeforeSystemctl(t *testing.T) {
	for _, service := range []*resources.Service{
		nil, {}, {Unit: "--all"}, {Units: []string{""}},
		{Unit: "db.service", Units: []string{"api.service"}},
		{Units: []string{"db.service", "db.service"}},
	} {
		err := checkResourceServiceWith(service, func(string, ...string) ([]byte, error) {
			t.Fatal("invalid plan reached systemctl")
			return nil, nil
		})
		if err == nil {
			t.Fatalf("accepted invalid service %+v", service)
		}
	}
}

func TestResourceGroupCheckAndApplyUseAllMembers(t *testing.T) {
	for _, check := range []bool{true, false} {
		t.Run(fmt.Sprint(check), func(t *testing.T) {
			r, _ := resourceFixture(t)
			item, err := resources.Parse("resources.d/ali-stack.json", []byte(`{"id":"ali-stack","description":"Stack","service":{"units":["db.service","api.service"],"actions":{"restart":{"roles":["viewer"]}}}}`))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			r.services = func(s *resources.Service) error {
				return checkResourceServiceWith(s, func(_ string, args ...string) ([]byte, error) {
					got = append(got, args[len(args)-1])
					return []byte("loaded\n"), nil
				})
			}
			if err = r.apply([]resources.Named{item}, check, ""); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, []string{"db.service", "api.service"}) {
				t.Fatalf("checked %v", got)
			}
		})
	}
}
