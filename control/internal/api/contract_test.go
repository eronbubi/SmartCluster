package api

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/eronbubi/smartcluster/control/internal/agentgw"
	"github.com/eronbubi/smartcluster/pkg/proto"
)

// The app's TypeScript types are generated from docs/api/openapi.yaml. These
// tests make sure the Go side sends exactly what the document describes.

type spec struct {
	Paths      map[string]any `yaml:"paths"`
	Components struct {
		Schemas map[string]struct {
			Required   []string       `yaml:"required"`
			Properties map[string]any `yaml:"properties"`
		} `yaml:"schemas"`
	} `yaml:"components"`
}

func loadSpec(t *testing.T) spec {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "api", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var s spec
	if err := yaml.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// jsonFields returns all JSON field names of a struct and those that are
// always present (no omitempty).
func jsonFields(typ reflect.Type) (all, always []string) {
	for i := range typ.NumField() {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		all = append(all, name)
		if !strings.Contains(opts, "omitempty") {
			always = append(always, name)
		}
	}
	slices.Sort(all)
	slices.Sort(always)
	return all, always
}

func TestContractSchemas(t *testing.T) {
	s := loadSpec(t)
	types := map[string]reflect.Type{
		"Node":            reflect.TypeFor[Node](),
		"StatsResponse":   reflect.TypeFor[StatsResponse](),
		"Point":           reflect.TypeFor[Point](),
		"Cluster":         reflect.TypeFor[Cluster](),
		"Error":           reflect.TypeFor[ErrorBody](),
		"OSInfo":          reflect.TypeFor[proto.OSInfo](),
		"Hardware":        reflect.TypeFor[proto.Hardware](),
		"CPUInfo":         reflect.TypeFor[proto.CPUInfo](),
		"GPUInfo":         reflect.TypeFor[proto.GPUInfo](),
		"NICInfo":         reflect.TypeFor[proto.NICInfo](),
		"Stats":           reflect.TypeFor[proto.Stats](),
		"CPUStats":        reflect.TypeFor[proto.CPUStats](),
		"MemStats":        reflect.TypeFor[proto.MemStats](),
		"DiskStats":       reflect.TypeFor[proto.DiskStats](),
		"NetStats":        reflect.TypeFor[proto.NetStats](),
		"GPUStats":        reflect.TypeFor[proto.GPUStats](),
		"TempStat":        reflect.TypeFor[proto.TempStat](),
		"Battery":         reflect.TypeFor[proto.Battery](),
		"NodeStatusEvent": reflect.TypeFor[agentgw.NodeStatus](),
		"NodeStatsEvent":  reflect.TypeFor[agentgw.NodeStats](),
	}
	for name, typ := range types {
		sch, ok := s.Components.Schemas[name]
		if !ok {
			t.Errorf("schema %s missing in openapi.yaml", name)
			continue
		}
		var props []string
		for p := range sch.Properties {
			props = append(props, p)
		}
		slices.Sort(props)
		required := slices.Clone(sch.Required)
		slices.Sort(required)

		all, always := jsonFields(typ)
		if !slices.Equal(props, all) {
			t.Errorf("%s: openapi properties %v, Go fields %v", name, props, all)
		}
		if !slices.Equal(required, always) {
			t.Errorf("%s: openapi required %v, Go fields without omitempty %v", name, required, always)
		}
	}
}

func TestContractPaths(t *testing.T) {
	s := loadSpec(t)
	var documented, served []string
	for p := range s.Paths {
		documented = append(documented, p)
	}
	for _, r := range (&API{}).routes() {
		served = append(served, r.path)
	}
	slices.Sort(documented)
	slices.Sort(served)
	if !slices.Equal(documented, served) {
		t.Errorf("openapi paths %v\nserved routes %v", documented, served)
	}
}
