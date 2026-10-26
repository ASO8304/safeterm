package networkmanager

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"safeterm/internal/audit"
	"safeterm/internal/contract"
)

type Options struct {
	Root, State string
	Policy      func() (Policy, error)
	Interfaces  func() map[string]string
	Backend     Backend
	Audit       audit.Sink
	TrialLock   func() (io.Closer, error)
}
type candidate struct {
	Owner, Base, Revision string
	View                  View
}
type transaction struct {
	Owner, RequestID, Phase string
	Deadline                time.Time
	Files                   []File
	Changed                 map[string][]byte
	OldNames, NewNames      map[string]string
	Targets                 []string
	Sources                 map[string]string
	PolicyHash              string
}
type Manager struct {
	mu        sync.Mutex
	opts      Options
	pending   *transaction
	trial     Trial
	closed    bool
	lastError string
	trialLock io.Closer
}

func New(opts Options) (*Manager, error) {
	if opts.Root == "" || opts.State == "" || opts.Policy == nil || opts.Interfaces == nil || opts.Backend == nil {
		return nil, fmt.Errorf("network: incomplete manager options")
	}
	if err := os.MkdirAll(opts.State, 0700); err != nil {
		return nil, err
	}
	if err := checkPrivateDir(opts.Root, opts.State); err != nil {
		return nil, err
	}
	m := &Manager{opts: opts}
	var tx transaction
	if err := m.load("pending.json", &tx); err == nil {
		m.pending = &tx
		if err := m.acquireTrialLock(); err != nil {
			return nil, err
		}
		if err := m.revertLocked(context.Background()); err != nil {
			m.releaseTrialLock()
			return nil, fmt.Errorf("network: startup recovery failed: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return m, nil
}
func DecodeJSON(data []byte, out any) error {
	if len(data) > 16<<20 {
		return fmt.Errorf("network: JSON exceeds limit")
	}
	tokens := json.NewDecoder(bytes.NewReader(data))
	count := 0
	var walk func(int) error
	walk = func(depth int) error {
		count++
		if depth > 32 || count > 100000 {
			return fmt.Errorf("network: excessive JSON structure")
		}
		token, err := tokens.Token()
		if err != nil {
			return err
		}
		if d, ok := token.(json.Delim); ok {
			switch d {
			case '{':
				seen := map[string]bool{}
				for tokens.More() {
					key, err := tokens.Token()
					if err != nil {
						return err
					}
					s, ok := key.(string)
					if !ok || seen[s] {
						return fmt.Errorf("network: duplicate JSON key")
					}
					seen[s] = true
					if err := walk(depth + 1); err != nil {
						return err
					}
				}
			case '[':
				for tokens.More() {
					if err := walk(depth + 1); err != nil {
						return err
					}
				}
			default:
				return fmt.Errorf("network: invalid JSON structure")
			}
			_, err = tokens.Token()
			return err
		}
		return nil
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := tokens.Token(); err != io.EOF {
		return fmt.Errorf("network: trailing JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}
func ReadPolicy(root string) (Policy, error) {
	f, err := readTrusted(root, "etc/safeterm/network.json")
	if os.IsNotExist(err) {
		return DefaultPolicy(), nil
	}
	if err != nil {
		return Policy{}, err
	}
	var p Policy
	if err := DecodeJSON(f.Data, &p); err != nil {
		return p, fmt.Errorf("network: network.json: %w", err)
	}
	return p, p.Validate()
}
func (m *Manager) load(name string, out any) error {
	data, err := readPrivate(m.opts.Root, filepath.Join(m.opts.State, name))
	if err != nil {
		return err
	}
	return DecodeJSON(data, out)
}
func (m *Manager) save(name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return privateWrite(filepath.Join(m.opts.State, name), data)
}
func (m *Manager) remove(name string) error {
	err := os.Remove(filepath.Join(m.opts.State, name))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	dir, err := os.Open(m.opts.State)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func owner(req *contract.ExecutionRequest) string {
	return req.Metadata.BrokerType + ":" + req.Metadata.UserID
}
func roleAllowed(roles, allowed []string) bool {
	for _, r := range roles {
		if contains(allowed, r) {
			return true
		}
	}
	return false
}
func routeIdentity(r Route) string {
	r.Name = ""
	r.ReadOnly = ""
	data, _ := json.Marshal(r)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
func (m *Manager) snapshot(p Policy) (*Snapshot, error) {
	s, err := Discover(m.opts.Root, p, m.opts.Interfaces())
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	if err := m.load("route-names.json", &names); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for old, r := range s.View.Routes {
		if name := names[routeIdentity(r)]; name != "" && name != old {
			if !namePattern.MatchString(name) {
				return nil, fmt.Errorf("network: invalid stored route alias")
			}
			if _, ok := s.View.Routes[name]; ok {
				return nil, fmt.Errorf("network: route alias conflict")
			}
			delete(s.View.Routes, old)
			r.Name = name
			s.View.Routes[name] = r
			s.routeNodes[name] = s.routeNodes[old]
			delete(s.routeNodes, old)
		}
	}
	return s, nil
}
func (m *Manager) report(ctx context.Context, req *contract.ExecutionRequest, outcome, version string) error {
	if m.opts.Audit == nil {
		return nil
	}
	event := audit.Event{SchemaVersion: audit.SchemaVersion, RequestID: req.RequestID, CatalogVersion: version, Component: audit.ComponentConfigManager, Profile: "file-writer", Operation: req.Command, Outcome: outcome, BrokerType: req.Metadata.BrokerType, UserID: req.Metadata.UserID, ResourceID: "network", OccurredAt: time.Now().UTC()}
	if event.CatalogVersion == "" {
		event.CatalogVersion = "network-v1"
	}
	return m.opts.Audit.Record(ctx, event)
}
func (m *Manager) Handle(ctx context.Context, req *contract.ExecutionRequest) (*contract.ExecutionResponse, error) {
	if ctx == nil || req == nil || req.RequestID == "" || req.Metadata.UserID == "" || (req.Metadata.BrokerType != "ssh" && req.Metadata.BrokerType != "console") {
		return nil, fmt.Errorf("network: invalid authenticated request")
	}
	if !req.Metadata.SessionExpiresAt.IsZero() && !time.Now().Before(req.Metadata.SessionExpiresAt) {
		return nil, fmt.Errorf("network: session expired")
	}
	if len(req.Arguments) != 1 {
		return nil, fmt.Errorf("network: invalid request arguments")
	}
	data, ok := req.Arguments["payload"]
	if !ok || len(data) > 256<<10 {
		return nil, fmt.Errorf("network: invalid request payload")
	}
	var request Request
	if err := DecodeJSON([]byte(data), &request); err != nil {
		return nil, err
	}
	if req.Command != "network_"+request.Action {
		return nil, fmt.Errorf("network: action mismatch")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, fmt.Errorf("network: manager stopped")
	}
	p, err := m.opts.Policy()
	if err != nil {
		return nil, err
	}
	if err = p.Validate(); err != nil {
		return nil, err
	}
	read := request.Action == "read" || request.Action == "diff" || request.Action == "status"
	roles := p.EditRoles
	if read {
		roles = p.ReadRoles
	}
	if !roleAllowed(req.Metadata.ClaimedRoles, roles) {
		_ = m.report(ctx, req, "validation-rejected", "network-v1")
		return nil, fmt.Errorf("network: unauthorized action")
	}
	reply, err := m.dispatch(ctx, req, request, p)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(reply)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	return &contract.ExecutionResponse{RequestID: req.RequestID, Result: contract.ResultSuccess, Stdout: string(encoded), ExitCode: 0, StartedAt: now, EndedAt: now}, nil
}
func (m *Manager) dispatch(ctx context.Context, req *contract.ExecutionRequest, r Request, p Policy) (Reply, error) {
	if r.Action == "confirm" {
		return m.confirm(ctx, req, p)
	}
	if r.Action == "rollback" {
		return m.rollback(ctx, req, p)
	}
	if m.pending != nil {
		if r.Action == "read" || r.Action == "status" {
			view := View{Interfaces: map[string]Interface{}, Routes: map[string]Route{}, Pending: true, Owner: m.pending.Owner, Deadline: m.pending.Deadline.UTC().Format(time.RFC3339)}
			if r.Action == "status" {
				text, err := m.opts.Backend.Status(ctx, p.Interfaces)
				return Reply{View: &view, Text: text}, err
			}
			return Reply{View: &view, Text: "A network trial is pending confirmation."}, nil
		}
		return Reply{}, fmt.Errorf("network: a trial is pending; confirm or rollback it first")
	}
	s, err := m.snapshot(p)
	if err != nil {
		return Reply{}, err
	}
	view := s.View
	var c candidate
	cErr := m.load("candidate.json", &c)
	if cErr != nil && !os.IsNotExist(cErr) {
		return Reply{}, cErr
	}
	hasCandidate := cErr == nil
	if hasCandidate && c.Owner == owner(req) {
		if c.Base != s.View.Version && r.Action != "discard" {
			return Reply{}, fmt.Errorf("network: Netplan sources or policy changed; execute network discard before editing")
		}
		view = cloneView(c.View)
		view.Version = c.Revision
	}
	switch r.Action {
	case "read":
		if err := m.report(ctx, req, "read", s.View.Version); err != nil {
			return Reply{}, err
		}
		return Reply{View: &view}, nil
	case "status":
		text, err := m.opts.Backend.Status(ctx, p.Interfaces)
		return Reply{Text: text}, err
	case "diff":
		return Reply{Text: Diff(s.View, view)}, nil
	case "discard":
		if hasCandidate && c.Owner != owner(req) {
			return Reply{}, fmt.Errorf("network: candidate belongs to another identity")
		}
		if err := m.report(ctx, req, "rollback-authorized", s.View.Version); err != nil {
			return Reply{}, err
		}
		return Reply{Text: "Candidate discarded; active Netplan files unchanged."}, m.remove("candidate.json")
	case "save":
		if hasCandidate && c.Owner != owner(req) {
			return Reply{}, fmt.Errorf("network: candidate belongs to another identity")
		}
		if r.Version != view.Version {
			return Reply{}, fmt.Errorf("network: stale editor version; reload before saving")
		}
		modified, err := Modify(view, r.Changes)
		if err != nil {
			return Reply{}, err
		}
		if err = ValidateView(modified); err != nil {
			return Reply{}, err
		}
		if _, err = s.Render(modified); err != nil {
			return Reply{}, err
		}
		if err = m.report(ctx, req, "preview-created", s.View.Version); err != nil {
			return Reply{}, err
		}
		revision := randomID()
		modified.Version = revision
		c = candidate{Owner: owner(req), Base: s.View.Version, Revision: revision, View: modified}
		if err = m.save("candidate.json", c); err != nil {
			return Reply{}, err
		}
		return Reply{View: &modified, Text: "Network candidate saved. Active network and /etc/netplan are unchanged."}, nil
	case "apply":
		if err := m.acquireTrialLock(); err != nil {
			return Reply{}, err
		}
		defer func() {
			if m.pending == nil {
				m.releaseTrialLock()
			}
		}()
		if !hasCandidate || c.Owner != owner(req) {
			return Reply{}, fmt.Errorf("network: no candidate owned by this identity")
		}
		if c.Base != s.View.Version {
			return Reply{}, fmt.Errorf("network: Netplan sources changed; reload candidate")
		}
		changed, err := s.Render(c.View)
		if err != nil {
			return Reply{}, err
		}
		if len(changed) == 0 {
			return Reply{}, fmt.Errorf("network: candidate has no changes")
		}
		if err = m.opts.Backend.Validate(ctx, s.Files, changed); err != nil {
			return Reply{}, err
		}
		latest, err := m.snapshot(p)
		if err != nil || latest.View.Version != s.View.Version {
			return Reply{}, fmt.Errorf("network: sources changed during validation")
		}
		if err = m.report(ctx, req, "apply-authorized", s.View.Version); err != nil {
			return Reply{}, err
		}
		tx := &transaction{Owner: owner(req), RequestID: req.RequestID, Phase: "trial", Deadline: time.Now().Add(time.Duration(p.ConfirmSeconds+30) * time.Second), Changed: changed, OldNames: map[string]string{}, NewNames: map[string]string{}, Sources: map[string]string{}, PolicyHash: policyHash(p)}
		if err = m.load("route-names.json", &tx.OldNames); err != nil && !os.IsNotExist(err) {
			return Reply{}, err
		}
		for _, route := range c.View.Routes {
			tx.NewNames[routeIdentity(route)] = route.Name
		}
		for _, file := range s.Files {
			tx.Sources[file.Path] = fileDigest(file)
			if _, ok := changed[file.Path]; ok {
				tx.Files = append(tx.Files, file)
			}
		}
		for name, old := range s.View.Interfaces {
			if !equal(old, c.View.Interfaces[name]) || !equal(routesFor(s.View, name), routesFor(c.View, name)) {
				tx.Targets = append(tx.Targets, name)
			}
		}
		if err = m.save("pending.json", tx); err != nil {
			return Reply{}, err
		}
		m.pending = tx
		if err = m.writeOverrides(tx); err != nil {
			recovery := m.revertLocked(context.Background())
			return Reply{}, fmt.Errorf("network: create trial files: %v; recovery: %v", err, recovery)
		}
		trial, err := m.opts.Backend.Begin(context.Background(), p.ConfirmSeconds)
		if err != nil {
			recovery := m.revertLocked(context.Background())
			return Reply{}, fmt.Errorf("network: start trial: %v; recovery: %v", err, recovery)
		}
		m.trial = trial
		ready, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err = trial.Ready(ready)
		cancel()
		if err != nil {
			recovery := m.revertLocked(context.Background())
			return Reply{}, fmt.Errorf("network: trial readiness: %v; recovery: %v", err, recovery)
		}
		tx.Deadline = time.Now().Add(time.Duration(p.ConfirmSeconds) * time.Second)
		if err = m.save("pending.json", tx); err != nil {
			_ = m.revertLocked(context.Background())
			return Reply{}, err
		}
		go m.watch(trial, tx.Deadline)
		return Reply{Text: "Trial applied. Reconnect if needed, then execute network confirm before " + tx.Deadline.UTC().Format(time.RFC3339)}, nil
	default:
		return Reply{}, fmt.Errorf("network: unknown action")
	}
}
func (m *Manager) watch(trial Trial, deadline time.Time) {
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-trial.Done():
	case <-timer.C:
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.trial != trial || m.pending == nil {
		return
	}
	if err := m.revertLocked(context.Background()); err != nil {
		m.lastError = err.Error()
	}
}
func (m *Manager) confirm(ctx context.Context, req *contract.ExecutionRequest, p Policy) (Reply, error) {
	tx := m.pending
	if tx == nil || m.trial == nil || tx.Owner != owner(req) {
		return Reply{}, fmt.Errorf("network: no live trial owned by this identity")
	}
	if time.Now().After(tx.Deadline) {
		_ = m.revertLocked(context.Background())
		return Reply{}, fmt.Errorf("network: confirmation deadline expired")
	}
	if err := m.verifyOverrides(tx); err != nil {
		return Reply{}, err
	}
	if err := m.verifySources(tx, p); err != nil {
		return Reply{}, err
	}
	for _, f := range tx.Files {
		current, err := readTrusted(m.opts.Root, f.Path)
		if err != nil || !sameFile(current, f) {
			return Reply{}, fmt.Errorf("network: original file changed during trial: %s", f.Path)
		}
	}
	if err := m.report(ctx, req, "apply-authorized", "network-v1"); err != nil {
		return Reply{}, err
	}
	tx.Phase = "committing"
	if err := m.save("pending.json", tx); err != nil {
		return Reply{}, err
	}
	for _, file := range tx.Files {
		if err := replaceFile(m.opts.Root, file, tx.Changed[file.Path]); err != nil {
			recovery := m.revertLocked(context.Background())
			return Reply{}, fmt.Errorf("network: commit: %v; recovery: %v", err, recovery)
		}
	}
	if err := m.save("route-names.json", tx.NewNames); err != nil {
		_ = m.revertLocked(context.Background())
		return Reply{}, err
	}
	if err := m.trial.Finish(true); err != nil {
		recovery := m.revertLocked(context.Background())
		return Reply{}, fmt.Errorf("network: confirmation failed: %v; recovery: %v", err, recovery)
	}
	if err := m.removeOverrides(tx); err != nil {
		return Reply{}, err
	}
	tx.Phase = "confirmed"
	if err := m.save("last.json", tx); err != nil {
		return Reply{}, err
	}
	if err := m.remove("candidate.json"); err != nil {
		return Reply{}, err
	}
	if err := m.remove("pending.json"); err != nil {
		return Reply{}, err
	}
	m.pending = nil
	m.trial = nil
	m.releaseTrialLock()
	if err := m.report(ctx, req, "applied", "network-v1"); err != nil {
		return Reply{}, fmt.Errorf("network: configuration confirmed, completion audit unavailable: %w", err)
	}
	return Reply{Text: "Network configuration confirmed and persisted to its original Netplan files."}, nil
}
func sameFile(a, b File) bool {
	return bytes.Equal(a.Data, b.Data) && a.Mode == b.Mode && a.UID == b.UID && a.GID == b.GID && equal(a.Attrs, b.Attrs)
}
func validTransaction(tx *transaction) error {
	if tx == nil || len(tx.Files) == 0 || len(tx.Files) > maxFiles || len(tx.Changed) != len(tx.Files) {
		return fmt.Errorf("network: invalid recovery transaction")
	}
	for _, f := range tx.Files {
		if filepath.Dir(f.Path) != "etc/netplan" || !namePattern.MatchString(filepath.Base(f.Path)) || !strings.HasSuffix(f.Path, ".yaml") {
			return fmt.Errorf("network: invalid recovery path")
		}
		if _, ok := tx.Changed[f.Path]; !ok {
			return fmt.Errorf("network: missing replacement data")
		}
	}
	return nil
}
func (m *Manager) runtimePath(f File) string {
	return filepath.Join(m.opts.Root, "run/netplan", filepath.Base(f.Path))
}
func (m *Manager) writeOverrides(tx *transaction) error {
	if err := validTransaction(tx); err != nil {
		return err
	}
	dir := filepath.Join(m.opts.Root, "run/netplan")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := checkParents(m.opts.Root, filepath.Join(dir, "placeholder")); err != nil {
		return err
	}
	for _, f := range tx.Files {
		if err := writeNew(m.opts.Root, m.runtimePath(f), tx.Changed[f.Path]); err != nil {
			return err
		}
	}
	return nil
}
func (m *Manager) verifyOverrides(tx *transaction) error {
	for _, f := range tx.Files {
		current, err := readTrusted(m.opts.Root, "run/netplan/"+filepath.Base(f.Path))
		if err != nil || !bytes.Equal(current.Data, tx.Changed[f.Path]) {
			return fmt.Errorf("network: trial override changed: %s", filepath.Base(f.Path))
		}
	}
	return nil
}
func (m *Manager) removeOverrides(tx *transaction) error {
	for _, f := range tx.Files {
		relative := "run/netplan/" + filepath.Base(f.Path)
		current, err := readTrusted(m.opts.Root, relative)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !bytes.Equal(current.Data, tx.Changed[f.Path]) {
			return fmt.Errorf("network: refusing to remove changed trial override %s", relative)
		}
		if err = os.Remove(m.runtimePath(f)); err != nil {
			return err
		}
	}
	return nil
}
func (m *Manager) revertLocked(ctx context.Context) error {
	tx := m.pending
	if tx == nil {
		return nil
	}
	if err := validTransaction(tx); err != nil {
		return err
	}
	if m.trial != nil {
		_ = m.trial.Finish(false)
	}
	if err := m.removeOverrides(tx); err != nil {
		return err
	}
	for _, file := range tx.Files {
		current, err := readTrusted(m.opts.Root, file.Path)
		if err != nil {
			return err
		}
		if sameFile(current, file) {
			continue
		}
		expected := file
		expected.Data = tx.Changed[file.Path]
		if !sameFile(current, expected) {
			return fmt.Errorf("network: external edit prevents recovery of %s", file.Path)
		}
		if err := replaceFile(m.opts.Root, current, file.Data); err != nil {
			return err
		}
	}
	if err := m.save("route-names.json", tx.OldNames); err != nil {
		return err
	}
	if err := m.opts.Backend.Restore(ctx); err != nil {
		return err
	}
	if err := m.remove("pending.json"); err != nil {
		return err
	}
	m.pending = nil
	m.trial = nil
	m.releaseTrialLock()
	req := &contract.ExecutionRequest{RequestID: tx.RequestID, Command: "network_rollback", Metadata: contract.Metadata{UserID: "network-recovery"}}
	_ = m.report(context.Background(), req, "rollback-succeeded", "network-v1")
	return nil
}
func (m *Manager) rollback(ctx context.Context, req *contract.ExecutionRequest, p Policy) (Reply, error) {
	if err := m.report(ctx, req, "rollback-authorized", "network-v1"); err != nil {
		return Reply{}, err
	}
	if m.pending != nil {
		if err := m.revertLocked(context.Background()); err != nil {
			return Reply{}, err
		}
		return Reply{Text: "Trial rolled back; previous persistent files restored."}, nil
	}
	var tx transaction
	if err := m.load("last.json", &tx); err != nil {
		return Reply{}, fmt.Errorf("network: no last confirmed configuration to roll back: %w", err)
	}
	if err := validTransaction(&tx); err != nil {
		return Reply{}, err
	}
	for _, name := range tx.Targets {
		if !contains(p.Interfaces, name) {
			return Reply{}, fmt.Errorf("network: rollback interface is outside current policy")
		}
	}
	for _, f := range tx.Files {
		if len(p.Files) > 0 && !contains(p.Files, filepath.Base(f.Path)) {
			return Reply{}, fmt.Errorf("network: rollback file is outside policy")
		}
		current, err := readTrusted(m.opts.Root, f.Path)
		expected := f
		expected.Data = tx.Changed[f.Path]
		if err != nil || !sameFile(current, expected) {
			return Reply{}, fmt.Errorf("network: files changed since confirmation; refusing rollback")
		}
	}
	tx.Phase = "rollback"
	if err := m.save("pending.json", tx); err != nil {
		return Reply{}, err
	}
	m.pending = &tx
	if err := m.revertLocked(context.Background()); err != nil {
		return Reply{}, err
	}
	_ = m.remove("last.json")
	_ = m.remove("candidate.json")
	return Reply{Text: "Last confirmed network change rolled back."}, nil
}
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return m.revertLocked(context.Background())
}
func Diff(a, b View) string {
	lines := []string{}
	for name, old := range a.Interfaces {
		next := b.Interfaces[name]
		for _, pair := range []struct {
			key  string
			a, b any
		}{{"dhcp4", old.DHCP4, next.DHCP4}, {"addresses", old.Addresses, next.Addresses}, {"dns", old.DNS, next.DNS}, {"search-domains", old.SearchDomains, next.SearchDomains}, {"mtu", old.MTU, next.MTU}} {
			if !equal(pair.a, pair.b) {
				av, _ := json.Marshal(pair.a)
				bv, _ := json.Marshal(pair.b)
				lines = append(lines, fmt.Sprintf("interface %s %s: %s -> %s", name, pair.key, av, bv))
			}
		}
	}
	for name, old := range a.Routes {
		next, ok := b.Routes[name]
		if !ok {
			lines = append(lines, "route "+name+": deleted")
		} else if !equal(old, next) {
			data, _ := json.Marshal(next)
			lines = append(lines, "route "+name+": "+string(data))
		}
	}
	for name, r := range b.Routes {
		if _, ok := a.Routes[name]; !ok {
			data, _ := json.Marshal(r)
			lines = append(lines, "route "+name+": added "+string(data))
		}
	}
	sort.Strings(lines)
	if len(lines) == 0 {
		return "No pending changes."
	}
	return strings.Join(lines, "\n")
}

func fileDigest(f File) string {
	data, _ := json.Marshal(f)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func policyHash(p Policy) string {
	data, _ := json.Marshal(p)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func (m *Manager) verifySources(tx *transaction, p Policy) error {
	if tx.PolicyHash != policyHash(p) {
		return fmt.Errorf("network: policy changed during trial")
	}
	s, err := Discover(m.opts.Root, p, m.opts.Interfaces())
	if err != nil {
		return err
	}
	count := 0
	overrides := map[string]bool{}
	for _, f := range tx.Files {
		overrides["run/netplan/"+filepath.Base(f.Path)] = true
	}
	for _, file := range s.Files {
		if overrides[file.Path] {
			continue
		}
		if tx.Sources[file.Path] != fileDigest(file) {
			return fmt.Errorf("network: sources changed during trial: %s", file.Path)
		}
		count++
	}
	if count != len(tx.Sources) {
		return fmt.Errorf("network: source file set changed during trial")
	}
	return nil
}

func (m *Manager) acquireTrialLock() error {
	if m.trialLock != nil || m.opts.TrialLock == nil {
		return nil
	}
	lock, err := m.opts.TrialLock()
	if err != nil {
		return err
	}
	m.trialLock = lock
	return nil
}
func (m *Manager) releaseTrialLock() {
	if m.trialLock != nil {
		_ = m.trialLock.Close()
		m.trialLock = nil
	}
}
