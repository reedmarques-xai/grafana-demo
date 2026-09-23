package dashboards

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/components/simplejson"
)

// grafana-auth.json is the dummy Authentication & Identity board. A UID
// rename breaks /d/demo-auth links; a Prometheus query or testdata typo
// provisions a panel that never returns series; role pies that disagree
// with the breakdown table make the identity demo internally inconsistent.
const (
	grafanaAuthFile        = "grafana-auth.json"
	grafanaAuthUID         = "demo-auth"
	grafanaAuthTitle       = "Demo: Grafana Authentication & Identity"
	grafanaAuthProvider    = "demo-dummy-data"
	grafanaAuthFolderTitle = "Demo Dashboards"
	grafanaAuthFolderUID   = "ffyg8julzmscga"
)

var grafanaAuthScenarios = map[string]struct{}{
	"random_walk": {},
	"csv_content": {},
}

func TestGrafanaAuthDashboardProvisioning(t *testing.T) {
	demo := grafanaAuthDemoConfig(t)
	require.Equal(t, grafanaAuthFolderTitle, demo.Folder)
	require.Equal(t, grafanaAuthFolderUID, demo.FolderUID)
	require.Equal(t, "conf/provisioning/dashboards/json", demo.Options["path"])

	data := grafanaAuthDashboardJSON(t)
	dash, err := createDashboardJSON(data, time.Unix(0, 0), demo, 0, demo.FolderUID)
	require.NoError(t, err)
	require.Equal(t, grafanaAuthUID, dash.Dashboard.UID)
	require.Equal(t, grafanaAuthTitle, dash.Dashboard.Title)
	require.Equal(t, grafanaAuthFolderUID, dash.Dashboard.FolderUID)
	require.Equal(t, demo.OrgID, dash.OrgID)
	require.Equal(t, []string{"demo", "dummy", "auth"}, data.Get("tags").MustStringArray())
	require.Equal(t, "5s", data.Get("refresh").MustString())
	require.True(t, data.Get("editable").MustBool())

	gotUIDs := map[string]string{}
	jsonDir := filepath.Join(grafanaAuthRepoRoot(t), "conf", "provisioning", "dashboards", "json")
	entries, err := os.ReadDir(jsonDir)
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		sibling := grafanaAuthReadJSON(t, filepath.Join(jsonDir, entry.Name()))
		uid := sibling.Get("uid").MustString()
		require.NotEmpty(t, uid, "%s missing uid", entry.Name())
		if prev, dup := gotUIDs[uid]; dup {
			t.Fatalf("duplicate dashboard uid %q in %s and %s", uid, prev, entry.Name())
		}
		gotUIDs[uid] = entry.Name()
	}
	require.Equal(t, grafanaAuthFile, gotUIDs[grafanaAuthUID],
		"auth dashboard must keep uid %q so /d/demo-auth links keep working", grafanaAuthUID)

	panels := data.Get("panels").MustArray()
	require.Len(t, panels, 12)
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
			_, ok := grafanaAuthScenarios[scenario]
			require.True(t, ok, "panel %d uses unknown TestData scenario %q (live Prometheus would break the dummy board)", id, scenario)
		}
	}
}

func TestGrafanaAuthDashboardLoginAndIdentityContracts(t *testing.T) {
	data := grafanaAuthDashboardJSON(t)
	byTitle := grafanaAuthPanelsByTitle(t, data)

	form := byTitle["Form Logins"]
	require.Equal(t, "stat", form.Get("type").MustString())
	require.Equal(t, "ops", form.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []grafanaAuthQuery{{Ref: "A", Alias: "form", Scenario: "random_walk", Min: 0.4, Max: 2.8, Series: 1}}, grafanaAuthQueries(t, form))

	oauth := byTitle["OAuth Logins"]
	require.Equal(t, "stat", oauth.Get("type").MustString())
	require.Equal(t, "ops", oauth.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []grafanaAuthQuery{{Ref: "A", Alias: "oauth", Scenario: "random_walk", Min: 3.2, Max: 9.5, Series: 1}}, grafanaAuthQueries(t, oauth))

	saml := byTitle["SAML Logins"]
	require.Equal(t, "stat", saml.Get("type").MustString())
	require.Equal(t, "ops", saml.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []grafanaAuthQuery{{Ref: "A", Alias: "saml", Scenario: "random_walk", Min: 8.0, Max: 22.0, Series: 1}}, grafanaAuthQueries(t, saml))

	ldapP99 := byTitle["LDAP Sync p99"]
	require.Equal(t, "stat", ldapP99.Get("type").MustString())
	require.Equal(t, "ms", ldapP99.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []grafanaAuthThreshold{
		{Color: "green", Value: 0},
		{Color: "orange", Value: 800},
		{Color: "red", Value: 2000},
	}, grafanaAuthDefaultThresholds(t, ldapP99))
	require.Equal(t, []grafanaAuthQuery{{Ref: "A", Alias: "ldap_p99", Scenario: "random_walk", Min: 180, Max: 1400, Series: 1}}, grafanaAuthQueries(t, ldapP99))

	accessEval := byTitle["Access Eval Rate"]
	require.Equal(t, "stat", accessEval.Get("type").MustString())
	require.Equal(t, "ops", accessEval.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []grafanaAuthQuery{{Ref: "A", Alias: "access_eval", Scenario: "random_walk", Min: 120, Max: 480, Series: 1}}, grafanaAuthQueries(t, accessEval))

	byMethod := byTitle["Login Rates by Method"]
	require.Equal(t, "timeseries", byMethod.Get("type").MustString())
	require.Equal(t, "ops", byMethod.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []grafanaAuthQuery{
		{Ref: "A", Alias: "form", Scenario: "random_walk", Labels: "method=form", Min: 0.3, Max: 3.1, Series: 1},
		{Ref: "B", Alias: "oauth", Scenario: "random_walk", Labels: "method=oauth", Min: 3.0, Max: 10.0, Series: 1},
		{Ref: "C", Alias: "saml", Scenario: "random_walk", Labels: "method=saml", Min: 7.5, Max: 24.0, Series: 1},
	}, grafanaAuthQueries(t, byMethod))

	ldapDuration := byTitle["LDAP Sync Duration"]
	require.Equal(t, "timeseries", ldapDuration.Get("type").MustString())
	require.Equal(t, "ms", ldapDuration.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []grafanaAuthQuery{
		{Ref: "A", Alias: "p50", Scenario: "random_walk", Min: 80, Max: 260, Series: 1},
		{Ref: "B", Alias: "p99", Scenario: "random_walk", Min: 220, Max: 1500, Series: 1},
	}, grafanaAuthQueries(t, ldapDuration))

	evalDuration := byTitle["Access Evaluation & Permissions Duration"]
	require.Equal(t, "timeseries", evalDuration.Get("type").MustString())
	require.Equal(t, "ms", evalDuration.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []grafanaAuthQuery{
		{Ref: "A", Alias: "eval p50", Scenario: "random_walk", Min: 0.2, Max: 1.8, Series: 1},
		{Ref: "B", Alias: "eval p99", Scenario: "random_walk", Min: 2.0, Max: 12.0, Series: 1},
		{Ref: "C", Alias: "permissions p50", Scenario: "random_walk", Min: 0.4, Max: 3.2, Series: 1},
		{Ref: "D", Alias: "permissions p99", Scenario: "random_walk", Min: 4.0, Max: 18.0, Series: 1},
	}, grafanaAuthQueries(t, evalDuration))

	cache := byTitle["Permissions Cache Hit vs Miss"]
	require.Equal(t, "timeseries", cache.Get("type").MustString())
	require.Equal(t, "ops", cache.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []grafanaAuthColorOverride{
		{Matcher: "hit.*", Color: "green"},
		{Matcher: "miss.*", Color: "orange"},
	}, grafanaAuthColorOverrides(t, cache))
	require.Equal(t, []grafanaAuthQuery{
		{Ref: "A", Alias: "hit", Scenario: "random_walk", Labels: "status=hit", Min: 180, Max: 420, Series: 1},
		{Ref: "B", Alias: "miss", Scenario: "random_walk", Labels: "status=miss", Min: 8, Max: 46, Series: 1},
	}, grafanaAuthQueries(t, cache))
}

func TestGrafanaAuthDashboardRoleBreakdownConsistency(t *testing.T) {
	data := grafanaAuthDashboardJSON(t)
	byTitle := grafanaAuthPanelsByTitle(t, data)

	totalPie := byTitle["Roles (Total)"]
	require.Equal(t, "piechart", totalPie.Get("type").MustString())
	totalRows := grafanaAuthCSVRows(t, totalPie)
	require.Equal(t, [][]string{
		{"Role", "Count"},
		{"Viewer", "1842"},
		{"Editor", "416"},
		{"Admin", "37"},
	}, totalRows)

	activePie := byTitle["Roles (Active)"]
	require.Equal(t, "piechart", activePie.Get("type").MustString())
	activeRows := grafanaAuthCSVRows(t, activePie)
	require.Equal(t, [][]string{
		{"Role", "Count"},
		{"Viewer", "612"},
		{"Editor", "188"},
		{"Admin", "21"},
	}, activeRows)

	table := byTitle["Role Breakdown"]
	require.Equal(t, "table", table.Get("type").MustString())
	require.Equal(t, []grafanaAuthThreshold{
		{Color: "orange", Value: 0},
		{Color: "green", Value: 0.25},
	}, grafanaAuthNamedOverrideThresholds(t, table, "Active ratio"))
	tableRows := grafanaAuthCSVRows(t, table)
	require.Equal(t, [][]string{
		{"Role", "Total", "Active", "Active ratio"},
		{"Viewer", "1842", "612", "0.332"},
		{"Editor", "416", "188", "0.452"},
		{"Admin", "37", "21", "0.568"},
	}, tableRows)

	totals := grafanaAuthRoleCounts(t, totalRows)
	actives := grafanaAuthRoleCounts(t, activeRows)
	for _, row := range tableRows[1:] {
		require.Equal(t, totals[row[0]], row[1], "Role Breakdown Total for %s must match Roles (Total)", row[0])
		require.Equal(t, actives[row[0]], row[2], "Role Breakdown Active for %s must match Roles (Active)", row[0])
	}
}

type grafanaAuthQuery struct {
	Ref      string
	Alias    string
	Scenario string
	Labels   string
	Min      float64
	Max      float64
	Series   int
	CSV      string
}

type grafanaAuthThreshold struct {
	Color string
	Value float64
}

type grafanaAuthColorOverride struct {
	Matcher string
	Color   string
}

func grafanaAuthDemoConfig(t *testing.T) *config {
	t.Helper()

	providers, err := ReadDashboardConfig(filepath.Join(grafanaAuthRepoRoot(t), "conf", "provisioning", "dashboards"))
	require.NoError(t, err)

	var demo *config
	sameFolderUID := 0
	for i := range providers {
		if providers[i].FolderUID == grafanaAuthFolderUID {
			sameFolderUID++
		}
		if providers[i].Name == grafanaAuthProvider {
			demo = &providers[i].config
		}
	}
	require.NotNil(t, demo, "%s provider missing from conf/provisioning/dashboards", grafanaAuthProvider)
	require.Equal(t, 1, sameFolderUID, "folderUid %q must be unique across dashboard providers", grafanaAuthFolderUID)
	return demo
}

func grafanaAuthDashboardJSON(t *testing.T) *simplejson.Json {
	t.Helper()
	return grafanaAuthReadJSON(t, filepath.Join(grafanaAuthRepoRoot(t), "conf", "provisioning", "dashboards", "json", grafanaAuthFile))
}

func grafanaAuthReadJSON(t *testing.T, path string) *simplejson.Json {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err, path)
	data, err := simplejson.NewJson(raw)
	require.NoError(t, err, "%s must be valid JSON", path)
	return data
}

func grafanaAuthPanelsByTitle(t *testing.T, data *simplejson.Json) map[string]*simplejson.Json {
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

func grafanaAuthQueries(t *testing.T, panel *simplejson.Json) []grafanaAuthQuery {
	t.Helper()
	var out []grafanaAuthQuery
	for _, raw := range panel.Get("targets").MustArray() {
		tq := simplejson.NewFromAny(raw)
		q := grafanaAuthQuery{
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

func grafanaAuthDefaultThresholds(t *testing.T, panel *simplejson.Json) []grafanaAuthThreshold {
	t.Helper()
	return grafanaAuthThresholdSteps(t, panel.Get("fieldConfig").Get("defaults").Get("thresholds").Get("steps").MustArray())
}

func grafanaAuthNamedOverrideThresholds(t *testing.T, panel *simplejson.Json, fieldName string) []grafanaAuthThreshold {
	t.Helper()
	for _, raw := range panel.Get("fieldConfig").Get("overrides").MustArray() {
		override := simplejson.NewFromAny(raw)
		if override.Get("matcher").Get("options").MustString() != fieldName {
			continue
		}
		for _, rawProp := range override.Get("properties").MustArray() {
			prop := simplejson.NewFromAny(rawProp)
			if prop.Get("id").MustString() == "thresholds" {
				return grafanaAuthThresholdSteps(t, prop.Get("value").Get("steps").MustArray())
			}
		}
	}
	t.Fatalf("no threshold override for field %q", fieldName)
	return nil
}

func grafanaAuthThresholdSteps(t *testing.T, steps []any) []grafanaAuthThreshold {
	t.Helper()
	out := make([]grafanaAuthThreshold, 0, len(steps))
	for _, raw := range steps {
		step := simplejson.NewFromAny(raw)
		out = append(out, grafanaAuthThreshold{
			Color: step.Get("color").MustString(),
			Value: step.Get("value").MustFloat64(),
		})
	}
	return out
}

func grafanaAuthColorOverrides(t *testing.T, panel *simplejson.Json) []grafanaAuthColorOverride {
	t.Helper()
	var out []grafanaAuthColorOverride
	for _, raw := range panel.Get("fieldConfig").Get("overrides").MustArray() {
		override := simplejson.NewFromAny(raw)
		matcher := override.Get("matcher").Get("options").MustString()
		for _, rawProp := range override.Get("properties").MustArray() {
			prop := simplejson.NewFromAny(rawProp)
			if prop.Get("id").MustString() != "color" {
				continue
			}
			out = append(out, grafanaAuthColorOverride{
				Matcher: matcher,
				Color:   prop.Get("value").Get("fixedColor").MustString(),
			})
		}
	}
	return out
}

func grafanaAuthCSVRows(t *testing.T, panel *simplejson.Json) [][]string {
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

func grafanaAuthRoleCounts(t *testing.T, rows [][]string) map[string]string {
	t.Helper()
	require.GreaterOrEqual(t, len(rows), 2)
	out := map[string]string{}
	for _, row := range rows[1:] {
		require.Len(t, row, 2)
		out[row[0]] = row[1]
	}
	return out
}

func grafanaAuthRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
}
