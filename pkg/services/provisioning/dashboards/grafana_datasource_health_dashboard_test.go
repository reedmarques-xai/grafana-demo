package dashboards

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/components/simplejson"
)

// grafana-datasource-health.json is the dummy Data Source Health board.
// A UID rename breaks /d/demo-datasource-health links; a Prometheus query
// or testdata typo provisions a panel that never returns series; plugin
// aliases that disagree with the CSV tables make the health demo internally
// inconsistent.
const (
	dsHealthFile        = "grafana-datasource-health.json"
	dsHealthUID         = "demo-datasource-health"
	dsHealthTitle       = "Demo: Grafana Data Source Health"
	dsHealthProvider    = "demo-dummy-data"
	dsHealthFolderTitle = "Demo Dashboards"
	dsHealthFolderUID   = "ffyg8julzmscga"
)

var dsHealthScenarios = map[string]struct{}{
	"random_walk": {},
	"csv_content": {},
}

func TestGrafanaDatasourceHealthDashboardProvisioning(t *testing.T) {
	demo := dsHealthDemoConfig(t)
	require.Equal(t, dsHealthFolderTitle, demo.Folder)
	require.Equal(t, dsHealthFolderUID, demo.FolderUID)
	require.Equal(t, "conf/provisioning/dashboards/json", demo.Options["path"])

	data := dsHealthDashboardJSON(t)
	dash, err := createDashboardJSON(data, time.Unix(0, 0), demo, 0, demo.FolderUID)
	require.NoError(t, err)
	require.Equal(t, dsHealthUID, dash.Dashboard.UID)
	require.Equal(t, dsHealthTitle, dash.Dashboard.Title)
	require.Equal(t, dsHealthFolderUID, dash.Dashboard.FolderUID)
	require.Equal(t, demo.OrgID, dash.OrgID)
	require.Equal(t, []string{"demo", "dummy", "datasource"}, data.Get("tags").MustStringArray())
	require.Equal(t, "5s", data.Get("refresh").MustString())
	require.True(t, data.Get("editable").MustBool())

	gotUIDs := map[string]string{}
	jsonDir := filepath.Join(dsHealthRepoRoot(t), "conf", "provisioning", "dashboards", "json")
	entries, err := os.ReadDir(jsonDir)
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		sibling := dsHealthReadJSON(t, filepath.Join(jsonDir, entry.Name()))
		uid := sibling.Get("uid").MustString()
		require.NotEmpty(t, uid, "%s missing uid", entry.Name())
		if prev, dup := gotUIDs[uid]; dup {
			t.Fatalf("duplicate dashboard uid %q in %s and %s", uid, prev, entry.Name())
		}
		gotUIDs[uid] = entry.Name()
	}
	require.Equal(t, dsHealthFile, gotUIDs[dsHealthUID],
		"datasource health dashboard must keep uid %q so /d/demo-datasource-health links keep working", dsHealthUID)

	panels := data.Get("panels").MustArray()
	require.Len(t, panels, 9)
	for _, raw := range panels {
		panel := simplejson.NewFromAny(raw)
		id := panel.Get("id").MustInt()
		require.Equal(t, "grafana-testdata-datasource", panel.Get("datasource").Get("type").MustString(), "panel %d type", id)
		require.Equal(t, "testdata", panel.Get("datasource").Get("uid").MustString(), "panel %d uid", id)
		targets := panel.Get("targets").MustArray()
		require.NotEmpty(t, targets, "panel %d has no queries", id)
		for _, rawTarget := range targets {
			tq := simplejson.NewFromAny(rawTarget)
			require.Equal(t, "testdata", tq.Get("datasource").Get("uid").MustString(), "panel %d query uid", id)
			scenario := tq.Get("scenarioId").MustString()
			_, ok := dsHealthScenarios[scenario]
			require.True(t, ok, "panel %d uses unknown TestData scenario %q (live Prometheus would break the dummy board)", id, scenario)
		}
	}
}

func TestGrafanaDatasourceHealthDashboardProxyContracts(t *testing.T) {
	data := dsHealthDashboardJSON(t)
	byTitle := dsHealthPanelsByTitle(t, data)

	rps := byTitle["Proxy RPS"]
	require.Equal(t, "stat", rps.Get("type").MustString())
	require.Equal(t, "reqps", rps.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []dsHealthQuery{{Ref: "A", Alias: "proxy_rps", Scenario: "random_walk", Min: 80, Max: 260, Series: 1}}, dsHealthQueries(t, rps))

	rate5xx := byTitle["Proxy 5xx Rate"]
	require.Equal(t, "stat", rate5xx.Get("type").MustString())
	require.Equal(t, "percentunit", rate5xx.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []dsHealthThreshold{
		{Color: "green", Value: 0},
		{Color: "orange", Value: 0.01},
		{Color: "red", Value: 0.05},
	}, dsHealthDefaultThresholds(t, rate5xx))
	require.Equal(t, []dsHealthQuery{{Ref: "A", Alias: "proxy_5xx", Scenario: "random_walk", Min: 0.002, Max: 0.08, Series: 1}}, dsHealthQueries(t, rate5xx))

	p99 := byTitle["Proxy Latency p99"]
	require.Equal(t, "stat", p99.Get("type").MustString())
	require.Equal(t, "ms", p99.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []dsHealthThreshold{
		{Color: "green", Value: 0},
		{Color: "orange", Value: 400},
		{Color: "red", Value: 1000},
	}, dsHealthDefaultThresholds(t, p99))
	require.Equal(t, []dsHealthQuery{{Ref: "A", Alias: "proxy_p99", Scenario: "random_walk", Min: 180, Max: 1400, Series: 1}}, dsHealthQueries(t, p99))

	count := byTitle["Datasources"]
	require.Equal(t, "stat", count.Get("type").MustString())
	require.Equal(t, "short", count.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []dsHealthQuery{{Ref: "A", Alias: "datasources", Scenario: "random_walk", Min: 186, Max: 214, Series: 1}}, dsHealthQueries(t, count))

	statusCodes := byTitle["Proxy Status Codes"]
	require.Equal(t, "timeseries", statusCodes.Get("type").MustString())
	require.Equal(t, "reqps", statusCodes.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, "normal", statusCodes.Get("fieldConfig").Get("defaults").Get("custom").Get("stacking").Get("mode").MustString())
	require.Equal(t, []dsHealthColorOverride{
		{Matcher: "2xx", Color: "green"},
		{Matcher: "4xx", Color: "orange"},
		{Matcher: "5xx", Color: "red"},
	}, dsHealthColorOverrides(t, statusCodes))
	require.Equal(t, []dsHealthQuery{
		{Ref: "A", Alias: "2xx", Scenario: "random_walk", Labels: "status=2xx", Min: 140, Max: 280, Series: 1},
		{Ref: "B", Alias: "4xx", Scenario: "random_walk", Labels: "status=4xx", Min: 4, Max: 22, Series: 1},
		{Ref: "C", Alias: "5xx", Scenario: "random_walk", Labels: "status=5xx", Min: 0, Max: 12, Series: 1},
	}, dsHealthQueries(t, statusCodes))

	latency := byTitle["Proxy Latency"]
	require.Equal(t, "timeseries", latency.Get("type").MustString())
	require.Equal(t, "ms", latency.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []dsHealthQuery{
		{Ref: "A", Alias: "p50", Scenario: "random_walk", Min: 40, Max: 140, Series: 1},
		{Ref: "B", Alias: "p99", Scenario: "random_walk", Min: 180, Max: 1400, Series: 1},
	}, dsHealthQueries(t, latency))
}

func TestGrafanaDatasourceHealthDashboardPluginAndPublicQueryConsistency(t *testing.T) {
	data := dsHealthDashboardJSON(t)
	byTitle := dsHealthPanelsByTitle(t, data)

	byPlugin := byTitle["Query by ID Rate"]
	require.Equal(t, "timeseries", byPlugin.Get("type").MustString())
	require.Equal(t, "reqps", byPlugin.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []dsHealthQuery{
		{Ref: "A", Alias: "prometheus", Scenario: "random_walk", Labels: "plugin_id=prometheus", Min: 40, Max: 120, Series: 1},
		{Ref: "B", Alias: "loki", Scenario: "random_walk", Labels: "plugin_id=loki", Min: 18, Max: 70, Series: 1},
		{Ref: "C", Alias: "tempo", Scenario: "random_walk", Labels: "plugin_id=tempo", Min: 6, Max: 28, Series: 1},
		{Ref: "D", Alias: "elasticsearch", Scenario: "random_walk", Labels: "plugin_id=elasticsearch", Min: 4, Max: 22, Series: 1},
		{Ref: "E", Alias: "cloudwatch", Scenario: "random_walk", Labels: "plugin_id=cloudwatch", Min: 2, Max: 14, Series: 1},
	}, dsHealthQueries(t, byPlugin))

	pluginBars := byTitle["Datasources by Plugin"]
	require.Equal(t, "barchart", pluginBars.Get("type").MustString())
	pluginRows := dsHealthCSVRows(t, pluginBars)
	require.Equal(t, [][]string{
		{"Plugin", "Count"},
		{"prometheus", "48"},
		{"loki", "31"},
		{"tempo", "18"},
		{"elasticsearch", "22"},
		{"cloudwatch", "14"},
		{"mysql", "9"},
		{"postgres", "11"},
	}, pluginRows)

	publicQueries := byTitle["Public Dashboard Query Success by Type"]
	require.Equal(t, "table", publicQueries.Get("type").MustString())
	require.Equal(t, []dsHealthThreshold{
		{Color: "green", Value: 0},
		{Color: "orange", Value: 0.01},
		{Color: "red", Value: 0.05},
	}, dsHealthNamedOverrideThresholds(t, publicQueries, "Fail rate"))
	publicRows := dsHealthCSVRows(t, publicQueries)
	require.Equal(t, [][]string{
		{"Datasource", "Success", "Failed", "Fail rate"},
		{"prometheus", "18420", "92", "0.005"},
		{"loki", "6320", "410", "0.061"},
		{"tempo", "2110", "18", "0.008"},
		{"elasticsearch", "1540", "126", "0.076"},
		{"cloudwatch", "880", "41", "0.045"},
	}, publicRows)

	pluginCounts := dsHealthNamedCounts(t, pluginRows)
	queryPlugins := map[string]struct{}{}
	for _, q := range dsHealthQueries(t, byPlugin) {
		queryPlugins[q.Alias] = struct{}{}
		require.Equal(t, "plugin_id="+q.Alias, q.Labels, "query-by-id label must match alias %s", q.Alias)
		_, ok := pluginCounts[q.Alias]
		require.True(t, ok, "query-by-id plugin %q missing from Datasources by Plugin", q.Alias)
	}

	for _, row := range publicRows[1:] {
		require.Contains(t, queryPlugins, row[0], "public-dashboard type %q missing from Query by ID Rate", row[0])
		success, err := strconv.ParseFloat(row[1], 64)
		require.NoError(t, err)
		failed, err := strconv.ParseFloat(row[2], 64)
		require.NoError(t, err)
		failRate, err := strconv.ParseFloat(row[3], 64)
		require.NoError(t, err)
		require.InDelta(t, failed/(success+failed), failRate, 0.0006,
			"Fail rate for %s must be Failed/(Success+Failed)", row[0])
	}
}

type dsHealthQuery struct {
	Ref      string
	Alias    string
	Scenario string
	Labels   string
	Min      float64
	Max      float64
	Series   int
	CSV      string
}

type dsHealthThreshold struct {
	Color string
	Value float64
}

type dsHealthColorOverride struct {
	Matcher string
	Color   string
}

func dsHealthDemoConfig(t *testing.T) *config {
	t.Helper()

	providers, err := ReadDashboardConfig(filepath.Join(dsHealthRepoRoot(t), "conf", "provisioning", "dashboards"))
	require.NoError(t, err)

	var demo *config
	sameFolderUID := 0
	for i := range providers {
		if providers[i].FolderUID == dsHealthFolderUID {
			sameFolderUID++
		}
		if providers[i].Name == dsHealthProvider {
			demo = &providers[i].config
		}
	}
	require.NotNil(t, demo, "%s provider missing from conf/provisioning/dashboards", dsHealthProvider)
	require.Equal(t, 1, sameFolderUID, "folderUid %q must be unique across dashboard providers", dsHealthFolderUID)
	return demo
}

func dsHealthDashboardJSON(t *testing.T) *simplejson.Json {
	t.Helper()
	return dsHealthReadJSON(t, filepath.Join(dsHealthRepoRoot(t), "conf", "provisioning", "dashboards", "json", dsHealthFile))
}

func dsHealthReadJSON(t *testing.T, path string) *simplejson.Json {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err, path)
	data, err := simplejson.NewJson(raw)
	require.NoError(t, err, "%s must be valid JSON", path)
	return data
}

func dsHealthPanelsByTitle(t *testing.T, data *simplejson.Json) map[string]*simplejson.Json {
	t.Helper()
	out := map[string]*simplejson.Json{}
	for _, raw := range data.Get("panels").MustArray() {
		panel := simplejson.NewFromAny(raw)
		title := panel.Get("title").MustString()
		require.NotEmpty(t, title, "panel missing title")
		if _, dup := out[title]; dup {
			t.Fatalf("duplicate panel title %q", title)
		}
		out[title] = panel
	}
	return out
}

func dsHealthQueries(t *testing.T, panel *simplejson.Json) []dsHealthQuery {
	t.Helper()
	var out []dsHealthQuery
	for _, raw := range panel.Get("targets").MustArray() {
		tq := simplejson.NewFromAny(raw)
		q := dsHealthQuery{
			Ref:      tq.Get("refId").MustString(),
			Alias:    tq.Get("alias").MustString(),
			Scenario: tq.Get("scenarioId").MustString(),
			Labels:   tq.Get("labels").MustString(),
			CSV:      tq.Get("csvContent").MustString(),
		}
		if q.Scenario == "random_walk" {
			q.Min = tq.Get("min").MustFloat64()
			q.Max = tq.Get("max").MustFloat64()
			q.Series = tq.Get("seriesCount").MustInt()
		}
		out = append(out, q)
	}
	return out
}

func dsHealthDefaultThresholds(t *testing.T, panel *simplejson.Json) []dsHealthThreshold {
	t.Helper()
	return dsHealthThresholdSteps(t, panel.Get("fieldConfig").Get("defaults").Get("thresholds").Get("steps").MustArray())
}

func dsHealthNamedOverrideThresholds(t *testing.T, panel *simplejson.Json, fieldName string) []dsHealthThreshold {
	t.Helper()
	for _, raw := range panel.Get("fieldConfig").Get("overrides").MustArray() {
		override := simplejson.NewFromAny(raw)
		if override.Get("matcher").Get("options").MustString() != fieldName {
			continue
		}
		for _, rawProp := range override.Get("properties").MustArray() {
			prop := simplejson.NewFromAny(rawProp)
			if prop.Get("id").MustString() == "thresholds" {
				return dsHealthThresholdSteps(t, prop.Get("value").Get("steps").MustArray())
			}
		}
	}
	t.Fatalf("no threshold override for field %q", fieldName)
	return nil
}

func dsHealthThresholdSteps(t *testing.T, steps []any) []dsHealthThreshold {
	t.Helper()
	out := make([]dsHealthThreshold, 0, len(steps))
	for _, raw := range steps {
		step := simplejson.NewFromAny(raw)
		out = append(out, dsHealthThreshold{
			Color: step.Get("color").MustString(),
			Value: step.Get("value").MustFloat64(),
		})
	}
	return out
}

func dsHealthColorOverrides(t *testing.T, panel *simplejson.Json) []dsHealthColorOverride {
	t.Helper()
	var out []dsHealthColorOverride
	for _, raw := range panel.Get("fieldConfig").Get("overrides").MustArray() {
		override := simplejson.NewFromAny(raw)
		matcher := override.Get("matcher").Get("options").MustString()
		for _, rawProp := range override.Get("properties").MustArray() {
			prop := simplejson.NewFromAny(rawProp)
			if prop.Get("id").MustString() != "color" {
				continue
			}
			out = append(out, dsHealthColorOverride{
				Matcher: matcher,
				Color:   prop.Get("value").Get("fixedColor").MustString(),
			})
		}
	}
	return out
}

func dsHealthCSVRows(t *testing.T, panel *simplejson.Json) [][]string {
	t.Helper()
	var csvContent string
	for _, raw := range panel.Get("targets").MustArray() {
		target := simplejson.NewFromAny(raw)
		require.Equal(t, "testdata", target.Get("datasource").Get("uid").MustString())
		require.Equal(t, "csv_content", target.Get("scenarioId").MustString())
		csvContent = target.Get("csvContent").MustString()
	}
	require.NotEmpty(t, csvContent, "panel %q missing csvContent", panel.Get("title").MustString())
	rows, err := csv.NewReader(strings.NewReader(csvContent)).ReadAll()
	require.NoError(t, err)
	return rows
}

func dsHealthNamedCounts(t *testing.T, rows [][]string) map[string]string {
	t.Helper()
	require.GreaterOrEqual(t, len(rows), 2)
	out := map[string]string{}
	for _, row := range rows[1:] {
		require.Len(t, row, 2)
		out[row[0]] = row[1]
	}
	return out
}

func dsHealthRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
}
