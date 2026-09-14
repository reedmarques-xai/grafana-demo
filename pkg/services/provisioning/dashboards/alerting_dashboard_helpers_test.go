package dashboards

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/components/simplejson"
)

type thresholdStep struct {
	Color string
	Value float64
}

type testdataQuery struct {
	Ref      string
	Alias    string
	Scenario string
	Labels   string
	Min      float64
	Max      float64
	Series   int
	CSV      string
}

type promQuery struct {
	Ref    string
	Expr   string
	Legend string
	Format string
}

type colorOverride struct {
	Matcher string
	Color   string
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
}

func mustJSONFile(t *testing.T, parts ...string) *simplejson.Json {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(parts...))
	require.NoError(t, err)
	data, err := simplejson.NewJson(raw)
	require.NoError(t, err)
	return data
}

func demoProvider(t *testing.T) *config {
	t.Helper()
	providers, err := ReadDashboardConfig(filepath.Join(repoRoot(t), "conf", "provisioning", "dashboards"))
	require.NoError(t, err)
	for i := range providers {
		if providers[i].Name == "demo-dummy-data" {
			return &providers[i].config
		}
	}
	t.Fatal("demo-dummy-data provider missing")
	return nil
}

func panelsByID(t *testing.T, data *simplejson.Json) map[int]*simplejson.Json {
	t.Helper()
	raw := data.Get("panels").MustArray()
	require.NotEmpty(t, raw)
	byID := make(map[int]*simplejson.Json, len(raw))
	for i, rawPanel := range raw {
		p := simplejson.NewFromAny(rawPanel)
		id := p.Get("id").MustInt(0)
		require.NotZero(t, id, "panel[%d] missing id", i)
		_, dup := byID[id]
		require.False(t, dup, "duplicate panel id %d", id)
		byID[id] = p
	}
	return byID
}

func thresholdSteps(t *testing.T, panel *simplejson.Json) []thresholdStep {
	t.Helper()
	return decodeThresholdSteps(t, panel.Get("fieldConfig").Get("defaults").Get("thresholds").Get("steps").MustArray())
}

func decodeThresholdSteps(t *testing.T, raw []any) []thresholdStep {
	t.Helper()
	require.NotEmpty(t, raw)
	out := make([]thresholdStep, 0, len(raw))
	for _, step := range raw {
		s := simplejson.NewFromAny(step)
		out = append(out, thresholdStep{
			Color: s.Get("color").MustString(),
			Value: s.Get("value").MustFloat64(),
		})
	}
	return out
}

func testdataQueries(t *testing.T, panel *simplejson.Json) []testdataQuery {
	t.Helper()
	raw := panel.Get("targets").MustArray()
	require.NotEmpty(t, raw)
	out := make([]testdataQuery, 0, len(raw))
	for _, target := range raw {
		tq := simplejson.NewFromAny(target)
		out = append(out, testdataQuery{
			Ref:      tq.Get("refId").MustString(),
			Alias:    tq.Get("alias").MustString(),
			Scenario: tq.Get("scenarioId").MustString(),
			Labels:   tq.Get("labels").MustString(),
			Min:      tq.Get("min").MustFloat64(),
			Max:      tq.Get("max").MustFloat64(),
			Series:   tq.Get("seriesCount").MustInt(),
			CSV:      tq.Get("csvContent").MustString(),
		})
	}
	return out
}

func promQueries(t *testing.T, panel *simplejson.Json) []promQuery {
	t.Helper()
	raw := panel.Get("targets").MustArray()
	require.NotEmpty(t, raw)
	out := make([]promQuery, 0, len(raw))
	for _, target := range raw {
		tq := simplejson.NewFromAny(target)
		out = append(out, promQuery{
			Ref:    tq.Get("refId").MustString(),
			Expr:   tq.Get("expr").MustString(),
			Legend: tq.Get("legendFormat").MustString(),
			Format: tq.Get("format").MustString(),
		})
	}
	return out
}

func colorOverrides(t *testing.T, panel *simplejson.Json) []colorOverride {
	t.Helper()
	raw := panel.Get("fieldConfig").Get("overrides").MustArray()
	out := make([]colorOverride, 0)
	for _, item := range raw {
		ov := simplejson.NewFromAny(item)
		if ov.Get("matcher").Get("id").MustString() != "byRegexp" {
			continue
		}
		for _, propRaw := range ov.Get("properties").MustArray() {
			prop := simplejson.NewFromAny(propRaw)
			if prop.Get("id").MustString() != "color" {
				continue
			}
			out = append(out, colorOverride{
				Matcher: ov.Get("matcher").Get("options").MustString(),
				Color:   prop.Get("value").Get("fixedColor").MustString(),
			})
		}
	}
	return out
}

func namedOverrideThresholds(t *testing.T, panel *simplejson.Json, fieldName string) []thresholdStep {
	t.Helper()
	for _, item := range panel.Get("fieldConfig").Get("overrides").MustArray() {
		ov := simplejson.NewFromAny(item)
		matcher := ov.Get("matcher")
		if matcher.Get("id").MustString() != "byName" || matcher.Get("options").MustString() != fieldName {
			continue
		}
		for _, propRaw := range ov.Get("properties").MustArray() {
			prop := simplejson.NewFromAny(propRaw)
			if prop.Get("id").MustString() == "thresholds" {
				return decodeThresholdSteps(t, prop.Get("value").Get("steps").MustArray())
			}
		}
	}
	t.Fatalf("no byName thresholds override for %q", fieldName)
	return nil
}
