// Command gen reads controlplane/gen/endpoints.yaml (the machine-readable form
// of spec/models/d2/endpoint-queries.d2) and emits one Echo handler file per
// endpoint group into controlplane/endpoints/<group>_gen.go.
//
// Groups carrying a schema_deps list (crons, workers) are skipped until their
// tables land in postgres.d2 — generating handlers that reference non-existent
// tables would produce code that can't compile against the schema.
//
//	go run ./controlplane/gen
package main

import (
	_ "embed"
	"fmt"
	"go/format"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

//go:embed templates/group.go.tmpl
var groupTmpl string

type manifest struct {
	Groups []group `yaml:"groups"`
}

type group struct {
	Name       string     `yaml:"name"`
	DB         string     `yaml:"db"`
	SchemaDeps []string   `yaml:"schema_deps"`
	Endpoints  []endpoint `yaml:"endpoints"`
}

type endpoint struct {
	Name     string   `yaml:"name"`
	Method   string   `yaml:"method"`
	Path     string   `yaml:"path"`
	Kind     string   `yaml:"kind"`
	Outbox   bool     `yaml:"outbox"`
	Events   []string `yaml:"events"`
	Steps    []string `yaml:"steps"`
	Request  string   `yaml:"request"`
	Response string   `yaml:"response"`
	Custom   bool     `yaml:"custom"` // hand-written body in <group>.go; generate route only
	Impl     *impl    `yaml:"impl"`
}

// impl is the real-body spec for an endpoint (absent ⇒ scaffold the handler).
type impl struct {
	Mode       string   `yaml:"mode"`        // write_returning | read_one | update | delete | read_list | count | hard_delete
	Aggregate  string   `yaml:"aggregate"`   // event aggregate type
	Row        string   `yaml:"row"`         // db row struct to scan into
	Query      string   `yaml:"query"`       // SQL (raw, $1.. params)
	Args       []string `yaml:"args"`        // Go expressions bound as query args
	PathParam  string   `yaml:"path_param"`  // path param parsed as uuid into pathID
	PathParam2 string   `yaml:"path_param2"` // second path param parsed as uuid into pathID2
}

// view types passed to the template (with computed fields).
type groupView struct {
	Name        string
	Pascal      string
	PoolExpr    string
	NeedsPgx    bool
	NeedsUUID   bool
	NeedsErrors bool
	NeedsHTTP   bool
	Endpoints   []endpointView
}

type endpointView struct {
	endpoint
	Handler         string
	Pascal          string
	EchoVerb        string
	EchoPath        string
	IsWrite         bool
	IsRead          bool
	ProjectionSteps []string
}

func pascal(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '_' || r == '-' || r == '.' })
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

func aggFor(eventType string) string {
	prefix := eventType
	if i := strings.IndexByte(eventType, '.'); i >= 0 {
		prefix = eventType[:i]
	}
	switch prefix {
	case "execution", "interrupt":
		return "Run"
	default:
		return pascal(prefix)
	}
}

// echoPath converts manifest {param} segments to Echo :param style.
func echoPath(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		if p[i] == '{' {
			j := strings.IndexByte(p[i:], '}')
			if j > 0 {
				b.WriteByte(':')
				b.WriteString(p[i+1 : i+j])
				i += j
				continue
			}
		}
		b.WriteByte(p[i])
	}
	return b.String()
}

func main() {
	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	manifestPath := filepath.Join(root, "controlplane", "gen", "endpoints.yaml")
	outDir := filepath.Join(root, "controlplane", "endpoints")

	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		log.Fatalf("read manifest: %v (run from the duragraph module root)", err)
	}
	var m manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		log.Fatalf("parse manifest: %v", err)
	}

	tmpl := template.Must(template.New("group").Funcs(template.FuncMap{
		"aggFor": aggFor,
		"join":   strings.Join,
	}).Parse(groupTmpl))

	var generated, skipped []string
	for _, g := range m.Groups {
		if len(g.SchemaDeps) > 0 {
			skipped = append(skipped, fmt.Sprintf("%s (schema_deps: %s)", g.Name, strings.Join(g.SchemaDeps, ", ")))
			continue
		}
		gv := toView(g)
		var buf strings.Builder
		if err := tmpl.Execute(&buf, gv); err != nil {
			log.Fatalf("template %s: %v", g.Name, err)
		}
		src, err := format.Source([]byte(buf.String()))
		if err != nil {
			log.Fatalf("gofmt %s: %v\n--- source ---\n%s", g.Name, err, buf.String())
		}
		outFile := filepath.Join(outDir, g.Name+"_gen.go")
		if err := os.WriteFile(outFile, src, 0o644); err != nil {
			log.Fatalf("write %s: %v", outFile, err)
		}
		generated = append(generated, fmt.Sprintf("%s_gen.go (%d endpoints)", g.Name, len(g.Endpoints)))
	}

	fmt.Printf("generated %d files:\n", len(generated))
	for _, s := range generated {
		fmt.Printf("  ✓ %s\n", s)
	}
	if len(skipped) > 0 {
		fmt.Printf("skipped %d groups (schema gap — pending postgres.d2):\n", len(skipped))
		for _, s := range skipped {
			fmt.Printf("  ⏸ %s\n", s)
		}
	}
}

func toView(g group) groupView {
	poolExpr := "nil"
	switch g.DB {
	case "tenant":
		poolExpr = "s.Tenant"
	case "platform":
		poolExpr = "s.Platform"
	}
	gv := groupView{
		Name:     g.Name,
		Pascal:   pascal(g.Name),
		PoolExpr: poolExpr,
	}
	for _, e := range g.Endpoints {
		ev := endpointView{
			endpoint: e,
			Handler:  pascal(g.Name) + pascal(e.Name),
			Pascal:   pascal(g.Name) + pascal(e.Name),
			EchoVerb: strings.ToUpper(e.Method),
			EchoPath: echoPath(e.Path),
			IsWrite:  e.Kind == "write",
			IsRead:   e.Kind == "read",
		}
		if !e.Custom {
			gv.NeedsHTTP = true // generated body uses http.Status*
		}
		if ev.IsWrite && e.Outbox {
			gv.NeedsPgx = true
			gv.NeedsUUID = true
			ev.ProjectionSteps = e.Steps
		}
		if e.Impl != nil {
			gv.NeedsPgx = true // all impl bodies use pgx (CollectOneRow/CollectRows)
			switch e.Impl.Mode {
			case "write_returning", "read_one", "update", "delete":
				gv.NeedsUUID = true // uuid.New() or uuid.Parse()
			case "hard_delete", "read_list", "count":
				if e.Impl.PathParam != "" {
					gv.NeedsUUID = true
				}
			}
			if e.Impl.Mode == "read_one" || e.Impl.Mode == "update" || e.Impl.Mode == "hard_delete" || e.Impl.Mode == "write_plain_returning" {
				gv.NeedsErrors = true
			}
		}
		gv.Endpoints = append(gv.Endpoints, ev)
	}
	return gv
}
