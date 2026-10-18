package siteconfig

import (
	"fmt"
	"path/filepath"
	"strings"

	"safeterm/internal/config"
)

func Builtin(name config.RunnerProfile) bool {
	switch name {
	case config.ProfileDiagnostics, config.ProfileNetworkObserver, config.ProfileLogReader, config.ProfileServiceManager:
		return true
	}
	return false
}
func User(name string) string   { return "sft-site-" + name }
func Unit(name string) string   { return "safeterm-runner-site-" + name + ".service" }
func Socket(name string) string { return "/run/safeterm-site-" + name + "/runner.sock" }
func Base(p config.ProfileDefinition) config.RunnerProfile {
	if p.Template != "" {
		return p.Template
	}
	return p.Name
}
func ValidateProfile(p config.ProfileDefinition) error {
	base := Base(p)
	if !Builtin(base) {
		return fmt.Errorf("unsupported isolation template %s", base)
	}
	if p.Template != "" {
		if !SiteName.MatchString(string(p.Name)) || Builtin(p.Name) || p.RunAsUser != User(string(p.Name)) || p.SocketPath != Socket(string(p.Name)) {
			return fmt.Errorf("invalid managed profile identity/socket")
		}
	} else {
		identities := map[config.RunnerProfile]string{config.ProfileDiagnostics: "safeterm-diagnostics", config.ProfileNetworkObserver: "safeterm-network", config.ProfileLogReader: "safeterm-log-reader", config.ProfileServiceManager: "safeterm-service-manager"}
		if p.RunAsUser != identities[base] || p.SocketPath != "/run/"+p.RunAsUser+"/runner.sock" {
			return fmt.Errorf("built-in identity/socket cannot change")
		}
	}
	kind := config.ExecutorProcess
	if base == config.ProfileServiceManager {
		kind = config.ExecutorSystemd
	}
	if p.ExecutorKind != kind {
		return fmt.Errorf("executor kind conflicts with template")
	}
	if p.MemoryLimitMiB < 32 || p.MemoryLimitMiB > 4096 || p.ProcessLimit < 4 || p.ProcessLimit > 256 {
		return fmt.Errorf("memory must be 32..4096 MiB and process_limit 4..256")
	}
	if p.TimeoutSeconds < 0 || p.TimeoutSeconds > 86400 || p.OutputLimitBytes < 0 || p.OutputLimitBytes > 16<<20 || p.OutputLimitBytes > 0 && p.OutputLimitBytes < 1024 || p.MaxSessions < 0 || p.MaxSessions > 64 || p.IdleTimeoutSeconds < 0 || p.IdleTimeoutSeconds > 3600 {
		return fmt.Errorf("runtime limits outside allowed bounds")
	}
	seen := map[string]bool{}
	for _, cap := range p.Capabilities {
		if base != config.ProfileNetworkObserver || cap != "CAP_NET_RAW" || seen[cap] {
			return fmt.Errorf("capability %s is outside template ceiling", cap)
		}
		seen[cap] = true
	}
	if kind == config.ExecutorSystemd {
		if p.ControllerExecutableRef != "sudo" || strings.Join(p.ControllerArgsPrefix, "\x00") != "-n\x00/usr/bin/systemctl\x00--no-pager" {
			return fmt.Errorf("privileged controller cannot change")
		}
	} else if p.ControllerExecutableRef != "" || len(p.ControllerArgsPrefix) > 0 {
		return fmt.Errorf("process template has a controller")
	}
	return nil
}

// ValidateTransition permits only provisionable profiles; tests can retain the
// generic catalog engine separately from this installed deployment boundary.
func ValidateCatalog(c *config.Config) error {
	count := 0
	seenUsers := map[string]bool{}
	seenSockets := map[string]bool{}
	for _, p := range c.Profiles {
		if err := ValidateProfile(p); err != nil {
			return fmt.Errorf("profile %s: %w", p.Name, err)
		}
		if seenUsers[p.RunAsUser] || seenSockets[p.SocketPath] {
			return fmt.Errorf("profile identities must be distinct")
		}
		seenUsers[p.RunAsUser] = true
		seenSockets[p.SocketPath] = true
		if p.Template != "" {
			count++
		}
	}
	if count > MaxProfiles {
		return fmt.Errorf("too many site profiles")
	}
	return nil
}
func RunnerUnit(p config.ProfileDefinition) (string, error) {
	if err := ValidateProfile(p); err != nil {
		return "", err
	}
	base := Base(p)
	timeout := p.TimeoutSeconds
	if timeout == 0 {
		timeout = 30
		if base == config.ProfileLogReader || base == config.ProfileNetworkObserver {
			timeout = 1800
		}
	}
	output := p.OutputLimitBytes
	if output == 0 {
		output = 1 << 20
	}
	sessions := p.MaxSessions
	if sessions == 0 {
		sessions = 64
	}
	idle := p.IdleTimeoutSeconds
	if idle == 0 {
		idle = 300
	}
	nnp, suid, caps, ambient, families, groups := "yes", "yes", strings.Join(p.Capabilities, " "), strings.Join(p.Capabilities, " "), "AF_UNIX", "safeterm-audit"
	if base == config.ProfileNetworkObserver {
		families += " AF_INET AF_INET6 AF_PACKET"
	}
	if base == config.ProfileLogReader {
		groups += " adm"
	}
	if base == config.ProfileServiceManager {
		nnp = "no"
		suid = "no"
		caps = "CAP_SETUID CAP_SETGID CAP_AUDIT_WRITE"
		ambient = ""
		families += " AF_NETLINK"
	}
	runtime := filepath.Base(filepath.Dir(p.SocketPath))
	return fmt.Sprintf(`[Unit]
Description=SafeTerm managed runner %s
Requires=safeterm-audit.service
After=safeterm-audit.service network.target
Before=safeterm-gateway.service
[Service]
Type=simple
User=%s
Group=safeterm-gateway
SupplementaryGroups=%s
RuntimeDirectory=%s
RuntimeDirectoryMode=0750
ExecStart=/usr/local/bin/safeterm-runner --config /var/lib/safeterm/generated --profile %s --timeout %ds --output-limit %d --stream-chunk 16384 --stream-queue 64 --stream-max-sessions %d --stream-idle-timeout %ds --interrupt-grace 2s --terminate-grace 2s
NoNewPrivileges=%s
CapabilityBoundingSet=%s
AmbientCapabilities=%s
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
PrivateDevices=yes
RestrictAddressFamilies=%s
RestrictSUIDSGID=%s
LockPersonality=yes
MemoryDenyWriteExecute=yes
MemoryMax=%dM
TasksMax=%d
[Install]
WantedBy=multi-user.target
`, p.Name, p.RunAsUser, groups, runtime, p.Name, timeout, output, sessions, idle, nnp, caps, ambient, families, suid, p.MemoryLimitMiB, p.ProcessLimit), nil
}
