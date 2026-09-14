package dashboards

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/components/simplejson"
)

// Known TestData scenario IDs used by the demo alerting dashboard. A typo
// here means the panel provisions but never returns series.
var alertingOperationsScenarios = map[string]struct{}{
	"random_walk": {},
	"csv_content": {},
}

func TestAlertingOperationsDashboardProvisioning(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "conf", "provisioning", "dashboards", "json", "alerting-operations.json")

	raw, err := os.ReadFile(path)
	require.NoError(t, err, "alerting-operations.json must exist next to the other demo dashboards")

	data, err := simplejson.NewJson(raw)
	require.NoError(t, err)

	demo := demoProvider(t)
	require.Equal(t, "conf/provisioning/dashboards/json", demo.Options["path"])
	require.Equal(t, "demo-dummy", demo.FolderUID)
	require.Equal(t, "Demo", demo.Folder)

	dash, err := createDashboardJSON(data, time.Unix(0, 0), demo, 0, demo.FolderUID)
	require.NoError(t, err)
	require.Equal(t, "demo-alerting-operations", dash.Dashboard.UID)
	require.Equal(t, "Demo: Grafana Alerting Operations", dash.Dashboard.Title)
	require.Equal(t, demo.FolderUID, dash.Dashboard.FolderUID)
	require.Equal(t, demo.OrgID, dash.OrgID)
	require.Equal(t, []string{"demo", "dummy", "alerting"}, data.Get("tags").MustStringArray())

	seenUIDs := map[string]string{}
	entries, err := os.ReadDir(filepath.Join(root, "conf", "provisioning", "dashboards", "json"))
	require.NoError(t, err)
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		sibling := mustJSONFile(t, root, "conf", "provisioning", "dashboards", "json", entry.Name())
		uid := sibling.Get("uid").MustString()
		require.NotEmpty(t, uid, "%s missing uid", entry.Name())
		if prev, ok := seenUIDs[uid]; ok {
			t.Fatalf("duplicate demo dashboard uid %q in %s and %s", uid, prev, entry.Name())
		}
		seenUIDs[uid] = entry.Name()
	}
	require.Equal(t, "alerting-operations.json", seenUIDs["demo-alerting-operations"])

	panels := data.Get("panels").MustArray()
	require.Len(t, panels, 9)

	byID := panelsByID(t, data)
	for id, p := range byID {
		require.Equal(t, "grafana-testdata-datasource", p.Get("datasource").Get("type").MustString(), "panel %d type", id)
		require.Equal(t, "testdata", p.Get("datasource").Get("uid").MustString(), "panel %d uid must match testdata.yaml", id)
		for _, rawTarget := range p.Get("targets").MustArray() {
			tq := simplejson.NewFromAny(rawTarget)
			require.Equal(t, "testdata", tq.Get("datasource").Get("uid").MustString(), "panel %d query uid", id)
			scenario := tq.Get("scenarioId").MustString()
			_, ok := alertingOperationsScenarios[scenario]
			require.True(t, ok, "panel %d uses unknown TestData scenario %q", id, scenario)
		}
	}

	active := byID[1]
	require.NotNil(t, active)
	require.Equal(t, "stat", active.Get("type").MustString())
	require.Equal(t, "Active Alerts", active.Get("title").MustString())
	require.Equal(t, "short", active.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []thresholdStep{
		{Color: "green", Value: 0},
		{Color: "orange", Value: 1},
		{Color: "red", Value: 10},
	}, thresholdSteps(t, active))
	require.Equal(t, []testdataQuery{{Ref: "A", Alias: "active_alerts", Scenario: "random_walk", Min: 2, Max: 18, Series: 1}}, testdataQueries(t, active))

	rules := byID[2]
	require.NotNil(t, rules)
	require.Equal(t, "stat", rules.Get("type").MustString())
	require.Equal(t, "Alert Rules", rules.Get("title").MustString())
	require.Equal(t, []testdataQuery{{Ref: "A", Alias: "alert_rules", Scenario: "random_walk", Min: 186, Max: 214, Series: 1}}, testdataQueries(t, rules))

	groups := byID[3]
	require.NotNil(t, groups)
	require.Equal(t, "stat", groups.Get("type").MustString())
	require.Equal(t, "Rule Groups", groups.Get("title").MustString())
	require.Equal(t, []testdataQuery{{Ref: "A", Alias: "rule_groups", Scenario: "random_walk", Min: 42, Max: 58, Series: 1}}, testdataQueries(t, groups))

	failureRate := byID[4]
	require.NotNil(t, failureRate)
	require.Equal(t, "stat", failureRate.Get("type").MustString())
	require.Equal(t, "Notification Failure Rate", failureRate.Get("title").MustString())
	require.Equal(t, "percentunit", failureRate.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []thresholdStep{
		{Color: "green", Value: 0},
		{Color: "orange", Value: 0.01},
		{Color: "red", Value: 0.05},
	}, thresholdSteps(t, failureRate))
	require.Equal(t, []testdataQuery{{Ref: "A", Alias: "failure_rate", Scenario: "random_walk", Min: 0.004, Max: 0.09, Series: 1}}, testdataQueries(t, failureRate))

	activeOverTime := byID[5]
	require.NotNil(t, activeOverTime)
	require.Equal(t, "timeseries", activeOverTime.Get("type").MustString())
	require.Equal(t, "Active Alerts", activeOverTime.Get("title").MustString())
	require.Equal(t, []testdataQuery{{
		Ref: "A", Alias: "grafana", Scenario: "random_walk",
		Labels: "instance=grafana-$seriesIndex", Min: 1, Max: 14, Series: 3,
	}}, testdataQueries(t, activeOverTime))

	byState := byID[6]
	require.NotNil(t, byState)
	require.Equal(t, "timeseries", byState.Get("type").MustString())
	require.Equal(t, "Results by State", byState.Get("title").MustString())
	require.Equal(t, "ops", byState.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, "normal", byState.Get("fieldConfig").Get("defaults").Get("custom").Get("stacking").Get("mode").MustString())
	require.Equal(t, []testdataQuery{
		{Ref: "A", Alias: "normal", Scenario: "random_walk", Labels: "state=normal", Min: 80, Max: 140, Series: 1},
		{Ref: "B", Alias: "alerting", Scenario: "random_walk", Labels: "state=alerting", Min: 4, Max: 22, Series: 1},
		{Ref: "C", Alias: "pending", Scenario: "random_walk", Labels: "state=pending", Min: 1, Max: 8, Series: 1},
		{Ref: "D", Alias: "nodata", Scenario: "random_walk", Labels: "state=nodata", Min: 0, Max: 4, Series: 1},
		{Ref: "E", Alias: "error", Scenario: "random_walk", Labels: "state=error", Min: 0, Max: 3, Series: 1},
	}, testdataQueries(t, byState))

	notifications := byID[7]
	require.NotNil(t, notifications)
	require.Equal(t, "timeseries", notifications.Get("type").MustString())
	require.Equal(t, "Notifications Sent vs Failed", notifications.Get("title").MustString())
	require.Equal(t, "ops", notifications.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []colorOverride{
		{Matcher: "failed.*", Color: "red"},
		{Matcher: "sent.*", Color: "green"},
	}, colorOverrides(t, notifications))
	require.Equal(t, []testdataQuery{
		{Ref: "A", Alias: "sent - email", Scenario: "random_walk", Labels: "type=email", Min: 18, Max: 46, Series: 1},
		{Ref: "B", Alias: "failed - email", Scenario: "random_walk", Labels: "type=email", Min: 0, Max: 3, Series: 1},
		{Ref: "C", Alias: "sent - slack", Scenario: "random_walk", Labels: "type=slack", Min: 10, Max: 28, Series: 1},
		{Ref: "D", Alias: "failed - slack", Scenario: "random_walk", Labels: "type=slack", Min: 0, Max: 4, Series: 1},
		{Ref: "E", Alias: "sent - pagerduty", Scenario: "random_walk", Labels: "type=pagerduty", Min: 2, Max: 9, Series: 1},
		{Ref: "F", Alias: "failed - pagerduty", Scenario: "random_walk", Labels: "type=pagerduty", Min: 0, Max: 1, Series: 1},
	}, testdataQueries(t, notifications))

	duration := byID[8]
	require.NotNil(t, duration)
	require.Equal(t, "timeseries", duration.Get("type").MustString())
	require.Equal(t, "Evaluation Duration", duration.Get("title").MustString())
	require.Equal(t, "ms", duration.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []testdataQuery{
		{Ref: "A", Alias: "p50", Scenario: "random_walk", Min: 18, Max: 55, Series: 1},
		{Ref: "B", Alias: "p99", Scenario: "random_walk", Min: 80, Max: 240, Series: 1},
	}, testdataQueries(t, duration))

	byNotifier := byID[9]
	require.NotNil(t, byNotifier)
	require.Equal(t, "table", byNotifier.Get("type").MustString())
	require.Equal(t, "Notification Failure Rate by Notifier", byNotifier.Get("title").MustString())
	require.Equal(t, []thresholdStep{
		{Color: "green", Value: 0},
		{Color: "orange", Value: 0.01},
		{Color: "red", Value: 0.05},
	}, namedOverrideThresholds(t, byNotifier, "Failure rate"))
	require.Equal(t, []testdataQuery{{
		Ref:      "A",
		Scenario: "csv_content",
		CSV:      "Notifier,Failure rate,Sent (5m),Failed (5m)\nemail,0.012,420,5\nslack,0.048,180,9\npagerduty,0.002,90,0\nwebhook,0.081,55,5\nopsgenie,0.019,40,1\n",
	}}, testdataQueries(t, byNotifier))
}
