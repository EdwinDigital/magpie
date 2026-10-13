// Package enterprisedistribution supplies the tenant-scoped API behind an
// enterprise portal. An operator configures source and signing; requests do not.
package enterprisedistribution

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/yetone/magpie/internal/distribution"
	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/enterprisebuild"
)

type Principal struct {
	TenantID  string    `json:"tenantId"`
	Role      string    `json:"role"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}
type Config struct {
	Store        string
	BaseURL      string
	Principals   []Principal
	BuildOptions enterprisebuild.Options
	SigningKey   ed25519.PrivateKey
	SigningKeyID string
	Build        func(context.Context, enterprisebuild.Options) (enterprisebuild.Result, error)
}
type Job struct {
	ID              string                  `json:"id"`
	TenantID        string                  `json:"tenantId"`
	DistributionID  string                  `json:"distributionId"`
	Status          string                  `json:"status"`
	ProfileRevision int                     `json:"profileRevision"`
	Version         string                  `json:"version"`
	BuildNumber     int                     `json:"buildNumber"`
	Target          string                  `json:"target"`
	Error           string                  `json:"error,omitempty"`
	Result          *enterprisebuild.Result `json:"result,omitempty"`
}
type queued struct {
	job     Job
	options enterprisebuild.Options
}
type ticket struct {
	file   string
	tenant string
	expiry time.Time
}
type Server struct {
	cfg     Config
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	jobs    map[string]Job
	tickets map[string]ticket
	queue   chan queued
	wg      sync.WaitGroup
}

var safeID = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)
var safeVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func New(parent context.Context, cfg Config) (*Server, error) {
	if cfg.Store == "" || !filepath.IsAbs(cfg.Store) {
		return nil, fmt.Errorf("distribution storage must be an absolute directory")
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("distribution base URL must be HTTPS")
	}
	if len(cfg.Principals) == 0 {
		return nil, fmt.Errorf("tenant access configuration is required")
	}
	seen := map[string]bool{}
	for _, p := range cfg.Principals {
		if !safeID.MatchString(p.TenantID) || (p.Role != "admin" && p.Role != "employee") || len(p.Token) < 32 || seen[p.Token] {
			return nil, fmt.Errorf("invalid or duplicate access principal")
		}
		seen[p.Token] = true
	}
	if err := os.MkdirAll(cfg.Store, 0o700); err != nil {
		return nil, err
	}
	if cfg.Build == nil {
		cfg.Build = enterprisebuild.Build
	}
	ctx, cancel := context.WithCancel(parent)
	s := &Server{cfg: cfg, ctx: ctx, cancel: cancel, jobs: map[string]Job{}, tickets: map[string]ticket{}, queue: make(chan queued, 32)}
	records, err := os.ReadDir(filepath.Join(cfg.Store, "job-records"))
	if err != nil && !os.IsNotExist(err) {
		cancel()
		return nil, err
	}
	for _, file := range records {
		b, err := os.ReadFile(filepath.Join(cfg.Store, "job-records", file.Name()))
		var job Job
		if err != nil || json.Unmarshal(b, &job) != nil || file.Name() != job.ID+".json" || !safeID.MatchString(job.TenantID) || !safeID.MatchString(job.DistributionID) {
			cancel()
			return nil, fmt.Errorf("cannot read a distribution job record")
		}
		if job.Status == "queued" || job.Status == "building" || job.Status == "signing" || job.Status == "verifying" {
			job.Status = "failed"
			job.Error = "worker stopped before completing this job"
			if err := s.saveJob(job); err != nil {
				cancel()
				return nil, err
			}
		}
		s.jobs[job.ID] = job
	}
	s.wg.Add(1)
	go s.work()
	return s, nil
}

func (s *Server) Close() { s.cancel(); s.wg.Wait() }

func (s *Server) saveJob(job Job) error {
	if err := os.MkdirAll(filepath.Join(s.cfg.Store, "job-records"), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return edit.WriteAtomic(filepath.Join(s.cfg.Store, "job-records", job.ID+".json"), b)
}
func (s *Server) principal(r *http.Request) (Principal, bool) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == r.Header.Get("Authorization") {
		return Principal{}, false
	}
	for _, p := range s.cfg.Principals {
		if subtle.ConstantTimeCompare([]byte(token), []byte(p.Token)) == 1 && (p.ExpiresAt.IsZero() || time.Now().Before(p.ExpiresAt)) {
			return p, true
		}
	}
	return Principal{}, false
}
func reply(rw http.ResponseWriter, code int, value any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.Header().Set("Cache-Control", "no-store")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.WriteHeader(code)
	_ = json.NewEncoder(rw).Encode(value)
}
func bad(rw http.ResponseWriter, code int, err error) {
	reply(rw, code, map[string]string{"error": err.Error()})
}
func decode(rw http.ResponseWriter, r *http.Request, out any) error {
	d := json.NewDecoder(http.MaxBytesReader(rw, r.Body, 4<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return fmt.Errorf("one JSON value is required")
	}
	return nil
}
func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func validTarget(t string) bool {
	return t == "darwin/arm64" || t == "darwin/amd64" || t == "windows/amd64" || t == "windows/arm64"
}

func (s *Server) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/download/") {
		s.download(rw, r)
		return
	}
	p, ok := s.principal(r)
	if !ok {
		bad(rw, 401, fmt.Errorf("enterprise authentication required"))
		return
	}
	mux := http.NewServeMux()
	admin := func(f http.HandlerFunc) http.HandlerFunc {
		return func(rw http.ResponseWriter, r *http.Request) {
			if p.Role != "admin" {
				bad(rw, 403, fmt.Errorf("administrator access required"))
				return
			}
			f(rw, r)
		}
	}
	mux.HandleFunc("POST /v1/profiles", admin(func(rw http.ResponseWriter, r *http.Request) { s.profile(rw, r, p) }))
	mux.HandleFunc("GET /v1/profiles/{id}/{revision}", func(rw http.ResponseWriter, r *http.Request) {
		if !safeID.MatchString(r.PathValue("id")) || !regexp.MustCompile(`^[1-9][0-9]*$`).MatchString(r.PathValue("revision")) {
			http.NotFound(rw, r)
			return
		}
		file := filepath.Join(s.cfg.Store, "profiles", p.TenantID, r.PathValue("id"), r.PathValue("revision"), "profile.json")
		b, err := os.ReadFile(file)
		if err != nil {
			http.NotFound(rw, r)
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.Header().Set("Cache-Control", "no-store")
		_, _ = rw.Write(b)
	})
	mux.HandleFunc("POST /v1/builds", admin(func(rw http.ResponseWriter, r *http.Request) { s.build(rw, r, p) }))
	mux.HandleFunc("GET /v1/builds/{id}", func(rw http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		j, ok := s.jobs[r.PathValue("id")]
		s.mu.Unlock()
		if !ok || j.TenantID != p.TenantID {
			http.NotFound(rw, r)
			return
		}
		reply(rw, 200, j)
	})
	mux.HandleFunc("POST /v1/builds/{id}/publish", admin(func(rw http.ResponseWriter, r *http.Request) { s.publish(rw, r, p) }))
	mux.HandleFunc("GET /v1/downloads", func(rw http.ResponseWriter, r *http.Request) { s.downloads(rw, r, p) })
	mux.HandleFunc("GET /v1/updates/{tenant}/{id}/{channel}/manifest.json", func(rw http.ResponseWriter, r *http.Request) {
		if r.PathValue("tenant") != p.TenantID || !safeID.MatchString(r.PathValue("id")) || !safeID.MatchString(r.PathValue("channel")) {
			http.NotFound(rw, r)
			return
		}
		file := filepath.Join(s.cfg.Store, "channels", p.TenantID, r.PathValue("id"), r.PathValue("channel")+".json")
		b, err := os.ReadFile(file)
		if err != nil {
			http.NotFound(rw, r)
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.Header().Set("Cache-Control", "no-store")
		_, _ = rw.Write(b)
	})
	mux.HandleFunc("GET /v1/artifacts/{tenant}/{id}/{version}/{target}/{name}", func(rw http.ResponseWriter, r *http.Request) {
		if r.PathValue("tenant") != p.TenantID || !safeID.MatchString(r.PathValue("id")) || !safeVersion.MatchString(r.PathValue("version")) || !validTarget(strings.Replace(r.PathValue("target"), "-", "/", 1)) || !filepath.IsLocal(r.PathValue("name")) || filepath.Base(r.PathValue("name")) != r.PathValue("name") {
			http.NotFound(rw, r)
			return
		}
		file := filepath.Join(s.cfg.Store, "published", p.TenantID, r.PathValue("id"), r.PathValue("version"), r.PathValue("target"), r.PathValue("name"))
		if _, err := os.Stat(filepath.Join(filepath.Dir(file), ".published")); err != nil {
			http.NotFound(rw, r)
			return
		}
		http.ServeFile(rw, r, file)
	})
	mux.ServeHTTP(rw, r)
}

func (s *Server) profile(rw http.ResponseWriter, r *http.Request, p Principal) {
	var in struct {
		Profile distribution.Profile `json:"profile"`
		Logo    string               `json:"logoBase64"`
	}
	if err := decode(rw, r, &in); err != nil {
		bad(rw, 400, err)
		return
	}
	if in.Profile.TenantID != p.TenantID || !safeID.MatchString(in.Profile.DistributionID) || in.Profile.ProfileRevision < 1 {
		bad(rw, 403, fmt.Errorf("profile is outside this enterprise"))
		return
	}
	if in.Profile.Updates.Mode == "platform" {
		feed := strings.TrimRight(s.cfg.BaseURL, "/") + "/v1/updates/" + p.TenantID + "/" + in.Profile.DistributionID + "/" + in.Profile.Updates.Channel + "/manifest.json"
		if in.Profile.Updates.FeedURL != "" && in.Profile.Updates.FeedURL != feed {
			bad(rw, 400, fmt.Errorf("update feed must match this distribution channel"))
			return
		}
		in.Profile.Updates.FeedURL = feed
	}
	b, err := base64.StdEncoding.DecodeString(in.Logo)
	if err != nil || len(b) > 2<<20 {
		bad(rw, 400, fmt.Errorf("invalid logo"))
		return
	}
	stage, err := os.MkdirTemp(s.cfg.Store, "profile-stage-")
	if err != nil {
		bad(rw, 500, err)
		return
	}
	defer os.RemoveAll(stage)
	jsonData, _ := json.Marshal(in.Profile)
	profile := filepath.Join(stage, "input.json")
	logo := filepath.Join(stage, "input.png")
	if err := os.WriteFile(profile, jsonData, 0o600); err != nil {
		bad(rw, 500, err)
		return
	}
	if err := os.WriteFile(logo, b, 0o600); err != nil {
		bad(rw, 500, err)
		return
	}
	prepared := filepath.Join(stage, "prepared")
	out, err := enterprisebuild.Prepare(profile, logo, prepared)
	if err != nil {
		bad(rw, 400, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root := filepath.Join(s.cfg.Store, "profiles", p.TenantID, out.DistributionID)
	entries, err := os.ReadDir(root)
	if err != nil && !os.IsNotExist(err) {
		bad(rw, 500, err)
		return
	}
	if out.ProfileRevision != len(entries)+1 {
		bad(rw, 409, fmt.Errorf("profile revision must be the next immutable revision"))
		return
	}
	if len(entries) > 0 {
		previous, err := os.ReadFile(filepath.Join(root, fmt.Sprint(out.ProfileRevision-1), "profile.json"))
		var old distribution.Profile
		if err != nil || json.Unmarshal(previous, &old) != nil {
			bad(rw, 500, fmt.Errorf("previous profile cannot be read"))
			return
		}
		if old.Identity != out.Identity {
			bad(rw, 409, fmt.Errorf("distribution identity is immutable"))
			return
		}
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		bad(rw, 500, err)
		return
	}
	if err := os.Rename(prepared, filepath.Join(root, fmt.Sprint(out.ProfileRevision))); err != nil {
		bad(rw, 500, err)
		return
	}
	s.audit(p, "profile", out.DistributionID)
	reply(rw, 201, out)
}

func (s *Server) build(rw http.ResponseWriter, r *http.Request, p Principal) {
	var in struct {
		DistributionID  string `json:"distributionId"`
		ProfileRevision int    `json:"profileRevision"`
		Version         string `json:"version"`
		BuildNumber     int    `json:"buildNumber"`
		Target          string `json:"target"`
	}
	if err := decode(rw, r, &in); err != nil {
		bad(rw, 400, err)
		return
	}
	if !safeID.MatchString(in.DistributionID) || in.ProfileRevision < 1 || !safeVersion.MatchString(in.Version) || in.BuildNumber < 1 || in.BuildNumber > 65535 || !validTarget(in.Target) {
		bad(rw, 400, fmt.Errorf("invalid build selection"))
		return
	}
	root := filepath.Join(s.cfg.Store, "profiles", p.TenantID, in.DistributionID, fmt.Sprint(in.ProfileRevision))
	if _, err := os.Stat(filepath.Join(root, "profile.json")); err != nil {
		http.NotFound(rw, r)
		return
	}
	j := Job{ID: randomID(), TenantID: p.TenantID, DistributionID: in.DistributionID, Status: "queued", ProfileRevision: in.ProfileRevision, Version: in.Version, BuildNumber: in.BuildNumber, Target: in.Target}
	o := s.cfg.BuildOptions
	o.ProfilePath = filepath.Join(root, "profile.json")
	o.LogoPath = filepath.Join(root, "logo.png")
	o.Output = filepath.Join(s.cfg.Store, "jobs", j.ID)
	o.Target = in.Target
	o.Version = in.Version
	o.BuildNumber = in.BuildNumber
	if o.BaseRuntime != "" {
		rt, err := enterprisebuild.ReadRuntime(o.BaseRuntime)
		if err != nil || rt.Target != o.Target {
			bad(rw, 500, fmt.Errorf("approved runtime unavailable for target"))
			return
		}
		o.BaseSHA256 = rt.SHA256
	} else if o.Source != "" {
		commit, err := enterprisebuild.SourceCommit(s.ctx, o.Source)
		if err != nil {
			bad(rw, 500, err)
			return
		}
		o.SourceCommit = commit
	}
	s.mu.Lock()
	if err := s.saveJob(j); err != nil {
		s.mu.Unlock()
		bad(rw, 500, err)
		return
	}
	s.jobs[j.ID] = j
	s.mu.Unlock()
	select {
	case s.queue <- queued{j, o}:
		s.audit(p, "build", j.ID)
		reply(rw, 202, j)
	default:
		s.mu.Lock()
		j.Status = "failed"
		j.Error = "build queue was full"
		s.jobs[j.ID] = j
		_ = s.saveJob(j)
		s.mu.Unlock()
		bad(rw, 429, fmt.Errorf("build queue is full"))
	}
}

func (s *Server) work() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case q := <-s.queue:
			j := q.job
			j.Status = "building"
			s.mu.Lock()
			s.jobs[j.ID] = j
			_ = s.saveJob(j)
			s.mu.Unlock()
			q.options.Progress = func(state string) { s.mu.Lock(); j.Status = state; s.jobs[j.ID] = j; _ = s.saveJob(j); s.mu.Unlock() }
			ctx, cancel := context.WithTimeout(s.ctx, 30*time.Minute)
			result, err := s.cfg.Build(ctx, q.options)
			cancel()
			if err != nil {
				j.Status = "failed"
				j.Error = err.Error()
			} else {
				if result.Profile.TenantID != j.TenantID || result.Profile.DistributionID != j.DistributionID || result.Profile.ProfileRevision != j.ProfileRevision || result.Version != j.Version || result.BuildNumber != j.BuildNumber || result.Target != j.Target {
					j.Status = "failed"
					j.Error = "worker result differs from the frozen build request"
				} else {
					j.Status = "ready"
					j.Result = &result
				}
			}
			s.mu.Lock()
			s.jobs[j.ID] = j
			if err := s.saveJob(j); err != nil {
				j.Status = "failed"
				j.Error = "job result could not be persisted"
				s.jobs[j.ID] = j
			}
			s.mu.Unlock()
		}
	}
}

func (s *Server) publish(rw http.ResponseWriter, r *http.Request, p Principal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[r.PathValue("id")]
	if !ok || j.TenantID != p.TenantID {
		http.NotFound(rw, r)
		return
	}
	if j.Status != "ready" || j.Result == nil || !j.Result.Signed || j.Result.Dirty {
		bad(rw, 409, fmt.Errorf("only verified signed production jobs may publish"))
		return
	}
	result := *j.Result
	if result.Profile.TenantID != p.TenantID || result.Profile.DistributionID != j.DistributionID {
		bad(rw, 409, fmt.Errorf("worker returned a different enterprise"))
		return
	}
	dir := filepath.Join(s.cfg.Store, "published", p.TenantID, j.DistributionID, result.Version, strings.ReplaceAll(result.Target, "/", "-"))
	if _, err := os.Stat(dir); err == nil {
		bad(rw, 409, fmt.Errorf("published artifacts are immutable"))
		return
	}
	stage, err := os.MkdirTemp(s.cfg.Store, "publish-stage-")
	if err != nil {
		bad(rw, 500, err)
		return
	}
	defer os.RemoveAll(stage)
	for name := range result.Assets {
		if filepath.Base(name) != name || !filepath.IsLocal(name) {
			bad(rw, 400, fmt.Errorf("invalid worker artifact"))
			return
		}
		b, err := os.ReadFile(filepath.Join(s.cfg.Store, "jobs", j.ID, name))
		if err != nil {
			bad(rw, 500, err)
			return
		}
		hash := sha256.Sum256(b)
		a := result.Assets[name]
		if a.SHA256 != hex.EncodeToString(hash[:]) || a.Size != int64(len(b)) {
			bad(rw, 409, fmt.Errorf("worker artifact differs from its build metadata"))
			return
		}
		if err := os.WriteFile(filepath.Join(stage, name), b, 0o600); err != nil {
			bad(rw, 500, err)
			return
		}
	}
	var manifest distribution.Manifest
	var previous *distribution.Manifest
	if result.Profile.Updates.Mode == "platform" {
		channelPath := filepath.Join(s.cfg.Store, "channels", p.TenantID, j.DistributionID, result.Profile.Updates.Channel+".json")
		sequence := uint64(1)
		if b, err := os.ReadFile(channelPath); err == nil {
			var old distribution.Manifest
			if json.Unmarshal(b, &old) != nil {
				bad(rw, 500, fmt.Errorf("channel manifest cannot be read"))
				return
			}
			if err := distribution.VerifyManifest(old, result.Profile); err != nil {
				bad(rw, 500, err)
				return
			}
			if old.Version == result.Version && old.ProfileRevision == result.Profile.ProfileRevision {
				previous = &old
			}
			sequence = old.Sequence + 1
		} else if !os.IsNotExist(err) {
			bad(rw, 500, err)
			return
		}
		base := strings.TrimRight(s.cfg.BaseURL, "/") + "/v1/artifacts/" + p.TenantID + "/" + j.DistributionID + "/" + result.Version + "/" + strings.ReplaceAll(result.Target, "/", "-")
		manifest, err = enterprisebuild.SignManifest(result, stage, base, s.cfg.SigningKeyID, sequence, s.cfg.SigningKey)
		if err != nil {
			bad(rw, 400, err)
			return
		}
		if previous != nil {
			for name, asset := range previous.Assets {
				if _, exists := manifest.Assets[name]; exists {
					bad(rw, 409, fmt.Errorf("published assets cannot be replaced"))
					return
				}
				manifest.Assets[name] = asset
			}
			payload, err := manifest.SigningBytes()
			if err != nil {
				bad(rw, 500, err)
				return
			}
			manifest.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(s.cfg.SigningKey, payload))
		}
	}
	metadata, _ := json.Marshal(result)
	if err := os.WriteFile(filepath.Join(stage, "build-metadata.json"), metadata, 0o600); err != nil {
		bad(rw, 500, err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		bad(rw, 500, err)
		return
	}
	if err := os.Rename(stage, dir); err != nil {
		bad(rw, 500, err)
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(dir)
		}
	}()
	if err := os.WriteFile(filepath.Join(dir, ".published"), []byte(j.ID), 0o600); err != nil {
		bad(rw, 500, err)
		return
	}
	if manifest.Sequence > 0 {
		channelPath := filepath.Join(s.cfg.Store, "channels", p.TenantID, j.DistributionID, result.Profile.Updates.Channel+".json")
		b, _ := json.Marshal(manifest)
		if err := os.MkdirAll(filepath.Dir(channelPath), 0o700); err != nil {
			bad(rw, 500, err)
			return
		}
		if err := edit.WriteAtomic(channelPath, b); err != nil {
			bad(rw, 500, err)
			return
		}
	}
	committed = true
	j.Status = "published"
	s.jobs[j.ID] = j
	if err := s.saveJob(j); err != nil {
		bad(rw, 500, err)
		return
	}
	s.audit(p, "publish", j.ID)
	reply(rw, 201, j)
}

func (s *Server) downloads(rw http.ResponseWriter, r *http.Request, p Principal) {
	type item struct {
		Name    string `json:"name"`
		URL     string `json:"url"`
		Version string `json:"version"`
		Target  string `json:"target"`
		SHA256  string `json:"sha256"`
	}
	items := []item{}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, t := range s.tickets {
		if time.Now().After(t.expiry) {
			delete(s.tickets, id)
		}
	}
	if len(s.tickets) > 4096 {
		bad(rw, 429, fmt.Errorf("too many active download grants"))
		return
	}
	root := filepath.Join(s.cfg.Store, "published", p.TenantID)
	err := filepath.WalkDir(root, func(file string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() != "build-metadata.json" {
			return nil
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(file), ".published")); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		var result enterprisebuild.Result
		if json.Unmarshal(b, &result) != nil || !result.Signed || result.Profile.TenantID != p.TenantID {
			return fmt.Errorf("invalid published metadata")
		}
		for name, a := range result.Assets {
			if filepath.Base(name) != name {
				return fmt.Errorf("invalid artifact name")
			}
			id := randomID()
			s.tickets[id] = ticket{filepath.Join(filepath.Dir(file), name), p.TenantID, time.Now().Add(5 * time.Minute)}
			items = append(items, item{name, strings.TrimRight(s.cfg.BaseURL, "/") + "/download/" + id, result.Version, result.Target, a.SHA256})
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		bad(rw, 500, err)
		return
	}
	reply(rw, 200, items)
}

func (s *Server) download(rw http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/download/")
	s.mu.Lock()
	t, ok := s.tickets[id]
	s.mu.Unlock()
	if !ok || time.Now().After(t.expiry) || r.Method != "GET" {
		http.NotFound(rw, r)
		return
	}
	rw.Header().Set("Cache-Control", "private, no-store")
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(t.file)+`"`)
	http.ServeFile(rw, r, t.file)
}

func (s *Server) audit(p Principal, action, id string) {
	f, err := os.OpenFile(filepath.Join(s.cfg.Store, "audit.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		log.Printf("enterprise audit: %v", err)
		return
	}
	defer f.Close()
	b, _ := json.Marshal(map[string]any{"at": time.Now().UTC(), "tenantId": p.TenantID, "role": p.Role, "action": action, "id": id})
	if _, err := io.Copy(f, bytes.NewReader(append(b, '\n'))); err != nil {
		log.Printf("enterprise audit: %v", err)
	}
}
