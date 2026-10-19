package main

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"
)

// The engine is a simulation: no Docker, no network. Its clock and its random
// numbers are injected (a time passed to step, a seeded rand.Rand), so a run
// is the same every time and the tests and the docs demo can replay it.

const (
	histLen   = 60  // CPU and memory samples kept per container
	logCap    = 120 // log lines kept per container
	eventCap  = 200 // engine events kept
	warmSteps = 20  // steps run before the first frame, so graphs and logs start filled
	stopGrace = 2   // seconds a restarting container stays down
)

// Container states, as `docker ps` words them.
const (
	stRunning    = "running"
	stExited     = "exited"
	stRestarting = "restarting"
)

type portMap struct {
	host, cont int
	proto      string
}

type envVar struct{ k, v string }

// logLine is one line of a container's log or of the event log. lvl is
// "debug", "info", "warn", "err", or "" for a plain line.
type logLine struct {
	n    int
	text string
	lvl  string
}

type container struct {
	id, name, image, command, network, ip, restart string
	state                                          string
	exit                                           int
	created, started, restartAt                    time.Time
	cpuBase, memBase, memLimit                     float64 // percent of a core, MiB
	cpu, mem                                       float64
	cpuHist, memHist                               []float64 // memory as percent of its limit
	ports                                          []portMap
	env                                            []envVar
	mounts                                         []string // volume names
	kind                                           string   // picks the log templates
	logs                                           []logLine
	nlog                                           int
	crashAt                                        int // sim step at which it dies once (0: never)
}

type image struct {
	repo, tag, id string
	mb            float64
	created       time.Time
}

type volume struct {
	name, driver string
	mb           float64
	created      time.Time
}

type sim struct {
	rng     *rand.Rand
	now     time.Time
	step    int
	cs      []*container
	images  []*image
	volumes []*volume
	events  []logLine
	nev     int
}

// ---- construction ----

func hex(r *rand.Rand, n int) string {
	const digits = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = digits[r.Intn(16)]
	}
	return string(b)
}

func newSim(seed int64, start time.Time) *sim {
	s := &sim{rng: rand.New(rand.NewSource(seed))}
	r := s.rng
	h := func(d float64) time.Time { return start.Add(-time.Duration(d * float64(time.Hour))) }

	type spec struct {
		name, image, command, kind, restart string
		state                               string
		exit                                int
		up                                  float64 // hours up (or since created)
		cpu, mem, limit                     float64
		ports                               []portMap
		mounts                              []string
		env                                 []envVar
		crashAt                             int
	}
	specs := []spec{
		{"edge-proxy", "nginx:1.27-alpine", "nginx -g 'daemon off;'", "nginx", "always", stRunning, 0, 74.5, 3.2, 38, 128,
			[]portMap{{8080, 80, "tcp"}, {8443, 443, "tcp"}}, nil,
			[]envVar{{"NGINX_HOST", "edge.dev.local"}, {"NGINX_PORT", "80"}, {"TZ", "UTC"}}, 0},
		{"api-gateway", "acme/gateway:2.14.0", "node dist/server.js", "api", "always", stRunning, 0, 74.4, 11.5, 212, 512,
			[]portMap{{3000, 3000, "tcp"}}, nil,
			[]envVar{{"NODE_ENV", "production"}, {"PORT", "3000"}, {"UPSTREAM_BILLING", "http://billing-api:3001"},
				{"UPSTREAM_SEARCH", "http://search-api:3002"}, {"LOG_LEVEL", "info"}, {"JWT_SECRET", "********"}}, 0},
		{"billing-api", "acme/billing:1.9.3", "java -jar billing.jar", "api", "always", stRunning, 0, 52.1, 18.4, 610, 1024,
			[]portMap{{3001, 3001, "tcp"}}, nil,
			[]envVar{{"SPRING_PROFILES_ACTIVE", "prod"}, {"DB_URL", "jdbc:postgresql://postgres:5432/billing"}, {"DB_PASSWORD", "********"}}, 0},
		{"search-api", "acme/search:0.8.1", "python -m search.serve", "api", "unless-stopped", stRunning, 0, 26.0, 7.9, 148, 512,
			[]portMap{{3002, 3002, "tcp"}}, nil,
			[]envVar{{"INDEX_DIR", "/var/index"}, {"WORKERS", "4"}}, 0},
		{"worker-email", "acme/worker:3.2.0", "bundle exec sidekiq -q mail", "worker", "on-failure", stRunning, 0, 19.7, 4.1, 96, 256,
			nil, nil,
			[]envVar{{"QUEUE", "mail"}, {"AMQP_URL", "amqp://rabbitmq:5672"}, {"SMTP_PASSWORD", "********"}}, warmSteps + 18},
		{"worker-reports", "acme/worker:3.2.0", "bundle exec sidekiq -q reports", "worker", "no", stExited, 1, 5.2, 0, 0, 256,
			nil, nil,
			[]envVar{{"QUEUE", "reports"}, {"AMQP_URL", "amqp://rabbitmq:5672"}}, 0},
		{"postgres", "postgres:16.3", "postgres -c max_connections=200", "postgres", "always", stRunning, 0, 96.0, 2.6, 184, 1024,
			[]portMap{{5432, 5432, "tcp"}}, []string{"pgdata"},
			[]envVar{{"POSTGRES_USER", "acme"}, {"POSTGRES_DB", "acme"}, {"POSTGRES_PASSWORD", "********"}}, 0},
		{"redis", "redis:7.2-alpine", "redis-server --appendonly yes", "redis", "always", stRunning, 0, 96.0, 1.4, 31, 256,
			[]portMap{{6379, 6379, "tcp"}}, []string{"redisdata"}, nil, 0},
		{"rabbitmq", "rabbitmq:3.13-management", "rabbitmq-server", "rabbit", "always", stRunning, 0, 95.8, 6.8, 168, 512,
			[]portMap{{5672, 5672, "tcp"}, {15672, 15672, "tcp"}}, []string{"rabbitdata"},
			[]envVar{{"RABBITMQ_DEFAULT_USER", "acme"}, {"RABBITMQ_DEFAULT_PASS", "********"}}, 0},
		{"prometheus", "prom/prometheus:v2.53.0", "/bin/prometheus --config.file=/etc/prometheus/prometheus.yml", "prom", "unless-stopped", stRunning, 0, 48.0, 5.3, 224, 512,
			[]portMap{{9090, 9090, "tcp"}}, []string{"promdata"}, nil, 0},
		{"grafana", "grafana/grafana:11.1.0", "/run.sh", "grafana", "unless-stopped", stRunning, 0, 47.9, 3.0, 142, 512,
			[]portMap{{3030, 3000, "tcp"}}, []string{"grafanadata"},
			[]envVar{{"GF_SECURITY_ADMIN_USER", "admin"}, {"GF_SECURITY_ADMIN_PASSWORD", "********"}}, 0},
		{"db-migrate", "acme/migrate:1.4.0", "migrate up", "job", "no", stExited, 0, 52.0, 0, 0, 128,
			nil, nil, []envVar{{"DB_URL", "postgres://acme@postgres:5432/acme"}}, 0},
	}
	nets := []string{"acme_default"}
	for i, sp := range specs {
		c := &container{
			id: hex(r, 12), name: sp.name, image: sp.image, command: sp.command, restart: sp.restart,
			network: nets[0], ip: fmt.Sprintf("172.18.0.%d", i+2),
			state: sp.state, exit: sp.exit, created: h(sp.up + 0.5),
			cpuBase: sp.cpu, memBase: sp.mem, memLimit: sp.limit, kind: sp.kind,
			ports: sp.ports, mounts: sp.mounts, env: sp.env, crashAt: sp.crashAt,
		}
		if sp.state == stRunning {
			c.started = h(sp.up)
			c.cpu, c.mem = sp.cpu, sp.mem
		} else {
			c.started = h(sp.up)
		}
		s.cs = append(s.cs, c)
	}

	// Images: the ones the containers use, plus two stale ones.
	seen := map[string]bool{}
	for i, c := range s.cs {
		if seen[c.image] {
			continue
		}
		seen[c.image] = true
		repo, tag, _ := strings.Cut(c.image, ":")
		sizes := map[string]float64{"nginx": 48, "acme/gateway": 212, "acme/billing": 386, "acme/search": 540,
			"acme/worker": 318, "postgres": 431, "redis": 41, "rabbitmq": 273, "prom/prometheus": 247,
			"grafana/grafana": 452, "acme/migrate": 28}
		s.images = append(s.images, &image{repo: repo, tag: tag, id: hex(r, 12), mb: sizes[repo], created: h(float64(24 * (3 + i)))})
	}
	s.images = append(s.images,
		&image{repo: "acme/gateway", tag: "2.13.4", id: hex(r, 12), mb: 209, created: h(24 * 30)},
		&image{repo: "postgres", tag: "15.7", id: hex(r, 12), mb: 425, created: h(24 * 90)},
	)
	sort.SliceStable(s.images, func(i, j int) bool { return s.images[i].repo < s.images[j].repo })

	for _, v := range []struct {
		name string
		mb   float64
	}{{"pgdata", 1840}, {"redisdata", 12}, {"rabbitdata", 96}, {"promdata", 702}, {"grafanadata", 64}, {"old-backup", 3200}} {
		s.volumes = append(s.volumes, &volume{name: v.name, driver: "local", mb: v.mb, created: h(24 * 40)})
	}

	// Warm up: graphs and logs start filled. The warm-up runs on a clock
	// that ends at `start`.
	for i := 0; i < warmSteps; i++ {
		s.advance(start.Add(-time.Duration(warmSteps-i) * time.Second))
	}
	s.events = s.events[:0] // the engine's history before the app opened is not interesting
	s.nev = 0
	s.event("info", "engine", "docker 27.1.1 ready, %d containers", len(s.cs))
	return s
}

// ---- stepping ----

// advance moves the simulation one second: new CPU and memory samples, log
// lines, scheduled crashes, restarts coming back.
func (s *sim) advance(now time.Time) {
	s.now = now
	s.step++
	for _, c := range s.cs {
		switch c.state {
		case stRunning:
			c.cpu = c.cpuBase * (0.55 + 0.9*s.rng.Float64())
			if s.rng.Intn(25) == 0 {
				c.cpu *= 2.4
			}
			if c.cpu > 100 {
				c.cpu = 100
			}
			c.mem += (s.rng.Float64() - 0.48) * c.memBase * 0.03
			lo, hi := c.memBase*0.85, c.memBase*1.25
			if c.mem < lo {
				c.mem = lo
			}
			if c.mem > hi {
				c.mem = hi
			}
			c.cpuHist = push(c.cpuHist, c.cpu)
			c.memHist = push(c.memHist, 100*c.mem/c.memLimit)
			s.emitLogs(c)
			if c.crashAt != 0 && s.step == c.crashAt {
				s.die(c, 137, "out of memory")
			}
		case stRestarting:
			c.cpu, c.mem = 0, 0
			c.cpuHist = push(c.cpuHist, 0)
			c.memHist = push(c.memHist, 0)
			if !now.Before(c.restartAt) {
				c.state, c.started, c.exit = stRunning, now, 0
				c.cpu, c.mem = c.cpuBase*0.5, c.memBase*0.9
				s.event("info", "container", "start %s", c.name)
				s.log(c, "info", "ready")
			}
		default:
			c.cpu, c.mem = 0, 0
			c.cpuHist = push(c.cpuHist, 0)
			c.memHist = push(c.memHist, 0)
		}
	}
}

func push(h []float64, v float64) []float64 {
	h = append(h, v)
	if len(h) > histLen {
		h = h[len(h)-histLen:]
	}
	return h
}

// die ends a container: exited, or restarting when its policy says so.
func (s *sim) die(c *container, code int, why string) {
	c.exit = code
	c.cpu, c.mem = 0, 0
	s.event("err", "container", "die %s (exit %d, %s)", c.name, code, why)
	s.log(c, "err", "fatal: %s", why)
	if c.restart == "always" || c.restart == "unless-stopped" || (c.restart == "on-failure" && code != 0) {
		c.state = stRestarting
		c.restartAt = s.now.Add(stopGrace * time.Second)
		s.event("warn", "container", "restart %s in %ds (policy %s)", c.name, stopGrace, c.restart)
		return
	}
	c.state = stExited
}

func (s *sim) event(lvl, typ, format string, args ...any) {
	s.nev++
	s.events = append(s.events, logLine{n: s.nev, lvl: lvl,
		text: fmt.Sprintf("%s  %-9s %s", s.now.Format("15:04:05"), typ, fmt.Sprintf(format, args...))})
	if len(s.events) > eventCap {
		s.events = s.events[len(s.events)-eventCap:]
	}
}

func (s *sim) log(c *container, lvl, format string, args ...any) {
	c.nlog++
	c.logs = append(c.logs, logLine{n: c.nlog, lvl: lvl,
		text: s.now.Format("15:04:05") + " " + fmt.Sprintf(format, args...)})
	if len(c.logs) > logCap {
		c.logs = c.logs[len(c.logs)-logCap:]
	}
}

// emitLogs writes the log lines a running container produces this second.
func (s *sim) emitLogs(c *container) {
	r := s.rng
	paths := []string{"/v1/orders", "/v1/users/42", "/v1/search?q=blue", "/healthz", "/v1/invoices", "/static/app.js"}
	switch c.kind {
	case "nginx":
		for i := 0; i < 1+r.Intn(2); i++ {
			code, lvl := 200, "info"
			switch r.Intn(30) {
			case 0:
				code, lvl = 502, "err"
			case 1, 2:
				code, lvl = 304, ""
			}
			s.log(c, lvl, `172.18.0.%d "GET %s" %d %dB`, 2+r.Intn(8), paths[r.Intn(len(paths))], code, 200+r.Intn(9000))
		}
	case "api":
		if r.Intn(3) != 0 {
			ms := 3 + r.Intn(60)
			lvl := "info"
			if r.Intn(18) == 0 {
				ms, lvl = 400+r.Intn(900), "warn"
			}
			s.log(c, lvl, "GET %s 200 %dms", paths[r.Intn(len(paths))], ms)
		}
		if r.Intn(40) == 0 {
			s.log(c, "err", "upstream timeout after 3000ms")
		}
	case "worker":
		if r.Intn(2) == 0 {
			if r.Intn(15) == 0 {
				s.log(c, "warn", "job retry attempt=2 id=%s", hex(r, 6))
			} else {
				s.log(c, "", "job done id=%s in %dms", hex(r, 6), 20+r.Intn(400))
			}
		}
	case "postgres":
		switch r.Intn(8) {
		case 0:
			s.log(c, "", "LOG:  checkpoint complete: wrote %d buffers", 20+r.Intn(200))
		case 1, 2:
			s.log(c, "debug", "LOG:  connection received: host=172.18.0.%d", 2+r.Intn(8))
		}
	case "redis":
		if r.Intn(6) == 0 {
			s.log(c, "", "%d changes in 60 seconds. Saving...", 100+r.Intn(900))
		}
	case "rabbit":
		if r.Intn(5) == 0 {
			s.log(c, "debug", "accepting AMQP connection 172.18.0.%d:%d", 2+r.Intn(8), 40000+r.Intn(9000))
		}
	case "prom":
		if r.Intn(4) == 0 {
			s.log(c, "", `level=info msg="Head GC completed" duration=%dms`, 2+r.Intn(30))
		}
	case "grafana":
		if r.Intn(5) == 0 {
			s.log(c, "", `logger=context path=/api/ds/query status=200 duration=%dms`, 5+r.Intn(90))
		}
	}
}

// ---- actions ----

func (s *sim) byID(id string) *container {
	for _, c := range s.cs {
		if c.id == id {
			return c
		}
	}
	return nil
}

func (s *sim) stop(c *container) bool {
	if c.state == stExited {
		return false
	}
	c.state, c.exit = stExited, 0
	c.cpu, c.mem = 0, 0
	s.event("info", "container", "stop %s", c.name)
	s.log(c, "info", "received SIGTERM, shutting down")
	return true
}

func (s *sim) start(c *container) bool {
	if c.state == stRunning {
		return false
	}
	c.state, c.exit, c.started = stRunning, 0, s.now
	c.cpu, c.mem = c.cpuBase*0.5, c.memBase*0.9
	s.event("info", "container", "start %s", c.name)
	s.log(c, "info", "starting")
	return true
}

func (s *sim) restartC(c *container) bool {
	c.state, c.restartAt = stRestarting, s.now.Add(stopGrace*time.Second)
	c.cpu, c.mem = 0, 0
	s.event("warn", "container", "restart %s", c.name)
	return true
}

func (s *sim) remove(c *container) {
	for i, x := range s.cs {
		if x == c {
			s.cs = append(s.cs[:i], s.cs[i+1:]...)
			break
		}
	}
	s.event("warn", "container", "destroy %s", c.name)
}

// unusedImages and unusedVolumes are what a prune would remove.
func (s *sim) unusedImages() []*image {
	used := map[string]bool{}
	for _, c := range s.cs {
		used[c.image] = true
	}
	var out []*image
	for _, im := range s.images {
		if !used[im.repo+":"+im.tag] {
			out = append(out, im)
		}
	}
	return out
}

func (s *sim) unusedVolumes() []*volume {
	used := map[string]bool{}
	for _, c := range s.cs {
		for _, m := range c.mounts {
			used[m] = true
		}
	}
	var out []*volume
	for _, v := range s.volumes {
		if !used[v.name] {
			out = append(out, v)
		}
	}
	return out
}

func (s *sim) pruneImages() (n int, mb float64) {
	unused := map[*image]bool{}
	for _, im := range s.unusedImages() {
		unused[im] = true
		n++
		mb += im.mb
	}
	kept := s.images[:0:0]
	for _, im := range s.images {
		if unused[im] {
			s.event("warn", "image", "delete %s:%s", im.repo, im.tag)
			continue
		}
		kept = append(kept, im)
	}
	s.images = kept
	return n, mb
}

func (s *sim) pruneVolumes() (n int, mb float64) {
	unused := map[*volume]bool{}
	for _, v := range s.unusedVolumes() {
		unused[v] = true
		n++
		mb += v.mb
	}
	kept := s.volumes[:0:0]
	for _, v := range s.volumes {
		if unused[v] {
			s.event("warn", "volume", "destroy %s", v.name)
			continue
		}
		kept = append(kept, v)
	}
	s.volumes = kept
	return n, mb
}
