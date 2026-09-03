package dashboards

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/components/simplejson"
)

// Locks the demo-async-processing dashboard that ships on reed-demo.
// A missing file, empty title, colliding UID, wrong testdata UID, or wiped
// queue/lag/cache/throughput thresholds silently empties the local demo.
func TestDemoAsyncProcessingDashboard(t *testing.T) {
	root := asyncProcessingRepoRoot(t)
	providers, err := ReadDashboardConfig(filepath.Join(root, "conf", "provisioning", "dashboards"))
	require.NoError(t, err)

	var demo *config
	for i := range providers {
		if providers[i].Name == "demo-dummy-data" {
			demo = &providers[i].config
			break
		}
	}
	require.NotNil(t, demo, "demo-dummy-data provider missing from conf/provisioning/dashboards")

	jsonDir, ok := demo.Options["path"].(string)
	require.True(t, ok, "provider options.path must be a string")
	require.Equal(t, "conf/provisioning/dashboards/json", jsonDir)

	absJSON := filepath.Join(root, jsonDir)
	raw, err := os.ReadFile(filepath.Join(absJSON, "async-processing.json"))
	require.NoError(t, err, "async-processing.json must live on the demo provider path")

	data, err := simplejson.NewJson(raw)
	require.NoError(t, err, "async-processing.json must be valid JSON")

	dash, err := createDashboardJSON(data, time.Unix(0, 0), demo, 0, demo.FolderUID)
	require.NoError(t, err, "async-processing.json must load as a provisioned dashboard")
	require.Equal(t, "demo-async-processing", dash.Dashboard.UID)
	require.Equal(t, "Demo: Async Processing", dash.Dashboard.Title)
	require.Equal(t, demo.FolderUID, dash.Dashboard.FolderUID)
	require.Equal(t, demo.OrgID, dash.OrgID)

	require.True(t, slices.Contains(data.Get("tags").MustStringArray(), "queue"))
	require.True(t, slices.Contains(data.Get("tags").MustStringArray(), "async"))
	require.Equal(t, "5s", data.Get("refresh").MustString())

	assertUniqueDashboardUID(t, absJSON, "async-processing.json", dash.Dashboard.UID)
	assertTestdataUIDs(t, data)
	assertUniquePanelIDs(t, data)

	panels := indexPanelsByID(t, data)
	require.Len(t, panels, 6)

	assertStatPanel(t, panels[1], panelContract{
		title:      "Queue depth",
		alias:      "queue_depth",
		unit:       "short",
		thresholds: []thresholdStep{{"green", 0}, {"yellow", 500}, {"red", 1200}},
	})
	assertStatPanel(t, panels[2], panelContract{
		title:      "Consumer lag",
		alias:      "consumer_lag",
		unit:       "s",
		thresholds: []thresholdStep{{"green", 0}, {"yellow", 30}, {"red", 120}},
	})
	assertGaugePanel(t, panels[3], panelContract{
		title:      "Cache hit rate",
		alias:      "cache_hit_rate",
		unit:       "percent",
		min:        0,
		max:        100,
		thresholds: []thresholdStep{{"red", 0}, {"yellow", 70}, {"green", 90}},
	})
	assertStatPanel(t, panels[4], panelContract{
		title:      "Message throughput",
		alias:      "throughput",
		unit:       "mps",
		thresholds: []thresholdStep{{"red", 0}, {"yellow", 120}, {"green", 300}},
	})

	groupPanel := panels[5]
	require.Equal(t, "timeseries", groupPanel.Get("type").MustString())
	require.Equal(t, "Queue depth by consumer group", groupPanel.Get("title").MustString())
	require.Equal(t, []string{"orders-consumer", "notifications-consumer", "analytics-consumer"}, targetAliases(groupPanel))
	require.Equal(t, []string{"consumer_group=orders", "consumer_group=notifications", "consumer_group=analytics"}, targetLabels(groupPanel))

	throughputPanel := panels[6]
	require.Equal(t, "timeseries", throughputPanel.Get("type").MustString())
	require.Equal(t, "Message throughput over time", throughputPanel.Get("title").MustString())
	require.Equal(t, "mps", throughputPanel.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []string{"published", "consumed"}, targetAliases(throughputPanel))
	require.Equal(t, []string{"direction=published", "direction=consumed"}, targetLabels(throughputPanel))
}

type thresholdStep struct {
	color string
	value float64
}

type panelContract struct {
	title      string
	alias      string
	unit       string
	min        float64
	max        float64
	thresholds []thresholdStep
}

func assertStatPanel(t *testing.T, panel *simplejson.Json, want panelContract) {
	t.Helper()
	require.Equal(t, "stat", panel.Get("type").MustString(), want.title)
	require.Equal(t, want.title, panel.Get("title").MustString())
	require.Equal(t, []string{want.alias}, targetAliases(panel), want.title)
	defaults := panel.Get("fieldConfig").Get("defaults")
	require.Equal(t, want.unit, defaults.Get("unit").MustString(), want.title)
	assertThresholds(t, defaults, want.thresholds, want.title)
}

func assertGaugePanel(t *testing.T, panel *simplejson.Json, want panelContract) {
	t.Helper()
	require.Equal(t, "gauge", panel.Get("type").MustString(), want.title)
	require.Equal(t, want.title, panel.Get("title").MustString())
	require.Equal(t, []string{want.alias}, targetAliases(panel), want.title)
	defaults := panel.Get("fieldConfig").Get("defaults")
	require.Equal(t, want.unit, defaults.Get("unit").MustString(), want.title)
	require.Equal(t, want.min, defaults.Get("min").MustFloat64(), want.title)
	require.Equal(t, want.max, defaults.Get("max").MustFloat64(), want.title)
	assertThresholds(t, defaults, want.thresholds, want.title)
}

func assertThresholds(t *testing.T, defaults *simplejson.Json, want []thresholdStep, panelTitle string) {
	t.Helper()
	steps := defaults.Get("thresholds").Get("steps").MustArray()
	require.Len(t, steps, len(want), panelTitle)
	for i, expected := range want {
		step := simplejson.NewFromAny(steps[i])
		require.Equal(t, expected.color, step.Get("color").MustString(), "%s threshold[%d] color", panelTitle, i)
		require.Equal(t, expected.value, step.Get("value").MustFloat64(), "%s threshold[%d] value", panelTitle, i)
	}
}

func assertTestdataUIDs(t *testing.T, data *simplejson.Json) {
	t.Helper()
	var uids []string
	for _, panel := range data.Get("panels").MustArray() {
		p := simplejson.NewFromAny(panel)
		if p.Get("datasource").Get("type").MustString() == "grafana-testdata-datasource" {
			uids = append(uids, p.Get("datasource").Get("uid").MustString())
		}
		for _, target := range p.Get("targets").MustArray() {
			tq := simplejson.NewFromAny(target)
			if tq.Get("datasource").Get("type").MustString() == "grafana-testdata-datasource" {
				uids = append(uids, tq.Get("datasource").Get("uid").MustString())
			}
		}
	}
	require.NotEmpty(t, uids, "async-processing.json has no grafana-testdata-datasource queries")
	for _, uid := range uids {
		require.Equal(t, "testdata", uid, "testdata query/panel uid must match the provisioned datasource")
	}
}

func assertUniquePanelIDs(t *testing.T, data *simplejson.Json) {
	t.Helper()
	ids := map[int]struct{}{}
	for i, panel := range data.Get("panels").MustArray() {
		id := simplejson.NewFromAny(panel).Get("id").MustInt(0)
		require.NotZero(t, id, "panel[%d] is missing id", i)
		if _, dup := ids[id]; dup {
			t.Fatalf("duplicate panel id %d", id)
		}
		ids[id] = struct{}{}
	}
}

func assertUniqueDashboardUID(t *testing.T, jsonDir, filename, uid string) {
	t.Helper()
	entries, err := os.ReadDir(jsonDir)
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" || entry.Name() == filename {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(jsonDir, entry.Name()))
		require.NoError(t, err, entry.Name())
		other, err := simplejson.NewJson(raw)
		require.NoError(t, err, entry.Name())
		require.NotEqual(t, uid, other.Get("uid").MustString(), "dashboard uid %q collides with %s", uid, entry.Name())
	}
}

func indexPanelsByID(t *testing.T, data *simplejson.Json) map[int]*simplejson.Json {
	t.Helper()
	out := map[int]*simplejson.Json{}
	for _, panel := range data.Get("panels").MustArray() {
		p := simplejson.NewFromAny(panel)
		out[p.Get("id").MustInt(0)] = p
	}
	return out
}

func targetAliases(panel *simplejson.Json) []string {
	var aliases []string
	for _, target := range panel.Get("targets").MustArray() {
		aliases = append(aliases, simplejson.NewFromAny(target).Get("alias").MustString())
	}
	return aliases
}

func targetLabels(panel *simplejson.Json) []string {
	var labels []string
	for _, target := range panel.Get("targets").MustArray() {
		labels = append(labels, simplejson.NewFromAny(target).Get("labels").MustString())
	}
	return labels
}

func asyncProcessingRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
}
