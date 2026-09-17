package dashboards

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/components/simplejson"
)

// Known TestData scenario IDs used by this dashboard. A typo here means the
// panel provisions but never returns series.
var asyncProcessingScenarios = map[string]struct{}{
	"random_walk":       {},
	"predictable_pulse": {},
}

func TestAsyncProcessingDashboardProvisioning(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "conf", "provisioning", "dashboards", "json", "async-processing.json")

	raw, err := os.ReadFile(path)
	require.NoError(t, err, "async-processing.json must exist next to the other demo dashboards")

	data, err := simplejson.NewJson(raw)
	require.NoError(t, err)

	providers, err := ReadDashboardConfig(filepath.Join(root, "conf", "provisioning", "dashboards"))
	require.NoError(t, err)

	var demo *config
	for i := range providers {
		if providers[i].Name == "demo-dummy-data" {
			demo = &providers[i].config
			break
		}
	}
	require.NotNil(t, demo, "demo-dummy-data provider must still pick up json/async-processing.json")
	require.Equal(t, "conf/provisioning/dashboards/json", demo.Options["path"])

	dash, err := createDashboardJSON(data, time.Unix(0, 0), demo, 0, demo.FolderUID)
	require.NoError(t, err)
	require.Equal(t, "demo-async-processing", dash.Dashboard.UID)
	require.Equal(t, "Demo: Async Processing", dash.Dashboard.Title)
	require.Equal(t, demo.FolderUID, dash.Dashboard.FolderUID)
	require.Equal(t, demo.OrgID, dash.OrgID)
	require.Equal(t, []string{"demo", "dummy", "queue", "async"}, data.Get("tags").MustStringArray())

	panels := data.Get("panels").MustArray()
	require.Len(t, panels, 6)

	byID := map[int]*simplejson.Json{}
	for i, rawPanel := range panels {
		p := simplejson.NewFromAny(rawPanel)
		id := p.Get("id").MustInt(0)
		require.NotZero(t, id, "panel[%d] missing id", i)
		_, dup := byID[id]
		require.False(t, dup, "duplicate panel id %d", id)
		byID[id] = p

		require.Equal(t, "grafana-testdata-datasource", p.Get("datasource").Get("type").MustString(), "panel %d type", id)
		require.Equal(t, "testdata", p.Get("datasource").Get("uid").MustString(), "panel %d uid must match testdata.yaml", id)

		for _, rawTarget := range p.Get("targets").MustArray() {
			tq := simplejson.NewFromAny(rawTarget)
			require.Equal(t, "testdata", tq.Get("datasource").Get("uid").MustString(), "panel %d query uid", id)
			scenario := tq.Get("scenarioId").MustString()
			_, ok := asyncProcessingScenarios[scenario]
			require.True(t, ok, "panel %d uses unknown TestData scenario %q", id, scenario)
		}
	}

	queue := byID[1]
	require.NotNil(t, queue)
	require.Equal(t, "stat", queue.Get("type").MustString())
	require.Equal(t, "Queue depth", queue.Get("title").MustString())
	require.Equal(t, "short", queue.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []thresholdStep{
		{Color: "green", Value: 0},
		{Color: "yellow", Value: 500},
		{Color: "red", Value: 1200},
	}, thresholdSteps(t, queue))
	require.Equal(t, []queryContract{{Ref: "A", Alias: "queue_depth", Scenario: "random_walk", Min: 80, Max: 1800}}, panelQueries(t, queue))

	lag := byID[2]
	require.NotNil(t, lag)
	require.Equal(t, "stat", lag.Get("type").MustString())
	require.Equal(t, "Consumer lag", lag.Get("title").MustString())
	require.Equal(t, "s", lag.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []thresholdStep{
		{Color: "green", Value: 0},
		{Color: "yellow", Value: 30},
		{Color: "red", Value: 120},
	}, thresholdSteps(t, lag))
	require.Equal(t, []queryContract{{Ref: "A", Alias: "consumer_lag", Scenario: "predictable_pulse"}}, panelQueries(t, lag))

	cache := byID[3]
	require.NotNil(t, cache)
	require.Equal(t, "gauge", cache.Get("type").MustString())
	require.Equal(t, "Cache hit rate", cache.Get("title").MustString())
	require.Equal(t, "percent", cache.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, float64(0), cache.Get("fieldConfig").Get("defaults").Get("min").MustFloat64())
	require.Equal(t, float64(100), cache.Get("fieldConfig").Get("defaults").Get("max").MustFloat64())
	require.Equal(t, []thresholdStep{
		{Color: "red", Value: 0},
		{Color: "yellow", Value: 70},
		{Color: "green", Value: 90},
	}, thresholdSteps(t, cache))
	require.Equal(t, []queryContract{{Ref: "A", Alias: "cache_hit_rate", Scenario: "random_walk", Min: 62, Max: 98}}, panelQueries(t, cache))

	throughput := byID[4]
	require.NotNil(t, throughput)
	require.Equal(t, "stat", throughput.Get("type").MustString())
	require.Equal(t, "Message throughput", throughput.Get("title").MustString())
	require.Equal(t, "mps", throughput.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []thresholdStep{
		{Color: "red", Value: 0},
		{Color: "yellow", Value: 120},
		{Color: "green", Value: 300},
	}, thresholdSteps(t, throughput))
	require.Equal(t, []queryContract{{Ref: "A", Alias: "throughput", Scenario: "random_walk", Min: 90, Max: 520}}, panelQueries(t, throughput))

	byGroup := byID[5]
	require.NotNil(t, byGroup)
	require.Equal(t, "timeseries", byGroup.Get("type").MustString())
	require.Equal(t, "Queue depth by consumer group", byGroup.Get("title").MustString())
	require.Equal(t, []queryContract{
		{Ref: "A", Alias: "orders-consumer", Scenario: "random_walk", Labels: "consumer_group=orders", Min: 40, Max: 900},
		{Ref: "B", Alias: "notifications-consumer", Scenario: "random_walk", Labels: "consumer_group=notifications", Min: 20, Max: 450},
		{Ref: "C", Alias: "analytics-consumer", Scenario: "random_walk", Labels: "consumer_group=analytics", Min: 60, Max: 1200},
	}, panelQueries(t, byGroup))

	overTime := byID[6]
	require.NotNil(t, overTime)
	require.Equal(t, "timeseries", overTime.Get("type").MustString())
	require.Equal(t, "Message throughput over time", overTime.Get("title").MustString())
	require.Equal(t, "mps", overTime.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []queryContract{
		{Ref: "A", Alias: "published", Scenario: "random_walk", Labels: "direction=published", Min: 150, Max: 480},
		{Ref: "B", Alias: "consumed", Scenario: "random_walk", Labels: "direction=consumed", Min: 120, Max: 460},
	}, panelQueries(t, overTime))
}

type thresholdStep struct {
	Color string
	Value float64
}

type queryContract struct {
	Ref      string
	Alias    string
	Scenario string
	Labels   string
	Min      float64
	Max      float64
}

func thresholdSteps(t *testing.T, panel *simplejson.Json) []thresholdStep {
	t.Helper()
	raw := panel.Get("fieldConfig").Get("defaults").Get("thresholds").Get("steps").MustArray()
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

func panelQueries(t *testing.T, panel *simplejson.Json) []queryContract {
	t.Helper()
	raw := panel.Get("targets").MustArray()
	require.NotEmpty(t, raw)
	out := make([]queryContract, 0, len(raw))
	for _, target := range raw {
		tq := simplejson.NewFromAny(target)
		out = append(out, queryContract{
			Ref:      tq.Get("refId").MustString(),
			Alias:    tq.Get("alias").MustString(),
			Scenario: tq.Get("scenarioId").MustString(),
			Labels:   tq.Get("labels").MustString(),
			Min:      tq.Get("min").MustFloat64(),
			Max:      tq.Get("max").MustFloat64(),
		})
	}
	return out
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
}
