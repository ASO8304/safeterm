package main

// The public CLI name differs from the legacy signed permission ID. Keep the
// latter stable for existing site policies and show it explicitly in describe.
func catalogOperationName(name string) string {
	if name == "telnet" {
		return "tcp-test"
	}
	return name
}
