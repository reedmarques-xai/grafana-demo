package dashboards

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/components/simplejson"
)

func TestGrafanaAlertingMixinDashboardContract(t *testing.T) {
	root := repoRoot(t)
	data := mustJSONFile(t, root, "grafana-mixin", "dashboards", "grafana-alerting.json")

	libsonnet, err := os.ReadFile(filepath.Join(root, "grafana-mixin", "dashboards", "dashboards.libsonnet"))
	require.NoError(t, err)
	require.Contains(t, string(libsonnet), "'grafana-alerting.json': (import 'grafana-alerting.json')",
		"mixin build drops grafana-alerting.json unless dashboards.libsonnet imports it")

	cfg := &config{Name: "grafana-mixin", OrgID: 1, FolderUID: "grafana-mixin"}
	dash, err := createDashboardJSON(data, time.Unix(0, 0), cfg, 0, cfg.FolderUID)
	require.NoError(t, err)
	require.Equal(t, "grafana-alerting", dash.Dashboard.UID)
	require.Equal(t, "Grafana Alerting Operations", dash.Dashboard.Title)
	require.Equal(t, []string{"grafana"}, data.Get("tags").MustStringArray())

	vars := data.Get("templating").Get("list").MustArray()
	require.Len(t, vars, 3)
	require.Equal(t, "datasource", simplejson.NewFromAny(vars[0]).Get("name").MustString())
	require.Equal(t, "datasource", simplejson.NewFromAny(vars[0]).Get("type").MustString())
	require.Equal(t, "prometheus", simplejson.NewFromAny(vars[0]).Get("query").MustString())

	job := simplejson.NewFromAny(vars[1])
	require.Equal(t, "job", job.Get("name").MustString())
	require.Equal(t, ".*", job.Get("allValue").MustString())
	require.Equal(t, "label_values(grafana_alerting_active_alerts, job)", job.Get("definition").MustString())
	require.Equal(t, "label_values(grafana_alerting_active_alerts, job)", job.Get("query").Get("query").MustString())

	instance := simplejson.NewFromAny(vars[2])
	require.Equal(t, "instance", instance.Get("name").MustString())
	require.Equal(t, ".*", instance.Get("allValue").MustString())
	require.Equal(t, "label_values(grafana_alerting_active_alerts, instance)", instance.Get("definition").MustString())
	require.Equal(t, "label_values(grafana_alerting_active_alerts, instance)", instance.Get("query").Get("query").MustString())

	panels := data.Get("panels").MustArray()
	require.Len(t, panels, 9)
	byID := panelsByID(t, data)

	for id, p := range byID {
		require.Equal(t, "$datasource", p.Get("datasource").Get("uid").MustString(), "panel %d datasource", id)
		for _, q := range promQueries(t, p) {
			require.Contains(t, q.Expr, `job=~"$job"`, "panel %d expr dropped job filter", id)
			require.Contains(t, q.Expr, `instance=~"$instance"`, "panel %d expr dropped instance filter", id)
		}
	}

	active := byID[1]
	require.Equal(t, "stat", active.Get("type").MustString())
	require.Equal(t, "Active Alerts", active.Get("title").MustString())
	require.Equal(t, []thresholdStep{
		{Color: "green", Value: 0},
		{Color: "orange", Value: 1},
		{Color: "red", Value: 10},
	}, thresholdSteps(t, active))
	require.Equal(t, []promQuery{{
		Ref:  "A",
		Expr: `sum(grafana_alerting_active_alerts{job=~"$job", instance=~"$instance"})`,
	}}, promQueries(t, active))
	require.True(t, simplejson.NewFromAny(active.Get("targets").MustArray()[0]).Get("instant").MustBool())

	rules := byID[2]
	require.Equal(t, "stat", rules.Get("type").MustString())
	require.Equal(t, "Alert Rules", rules.Get("title").MustString())
	require.Equal(t, []promQuery{{
		Ref:  "A",
		Expr: `max(grafana_stat_totals_alert_rules{job=~"$job", instance=~"$instance"})`,
	}}, promQueries(t, rules))

	groups := byID[3]
	require.Equal(t, "stat", groups.Get("type").MustString())
	require.Equal(t, "Rule Groups", groups.Get("title").MustString())
	require.Equal(t, []promQuery{{
		Ref:  "A",
		Expr: `max(grafana_stat_totals_rule_groups{job=~"$job", instance=~"$instance"})`,
	}}, promQueries(t, groups))

	failureRate := byID[4]
	require.Equal(t, "stat", failureRate.Get("type").MustString())
	require.Equal(t, "Notification Failure Rate", failureRate.Get("title").MustString())
	require.Equal(t, "percentunit", failureRate.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []thresholdStep{
		{Color: "green", Value: 0},
		{Color: "orange", Value: 0.01},
		{Color: "red", Value: 0.05},
	}, thresholdSteps(t, failureRate))
	require.Equal(t, []promQuery{{
		Ref: "A",
		Expr: "sum(rate(grafana_alerting_notification_failed_total{job=~\"$job\", instance=~\"$instance\"}[5m])) / " +
			"clamp_min(sum(rate(grafana_alerting_notification_sent_total{job=~\"$job\", instance=~\"$instance\"}[5m])) + " +
			"sum(rate(grafana_alerting_notification_failed_total{job=~\"$job\", instance=~\"$instance\"}[5m])), 1e-9)",
	}}, promQueries(t, failureRate))

	activeOverTime := byID[5]
	require.Equal(t, "timeseries", activeOverTime.Get("type").MustString())
	require.Equal(t, "Active Alerts", activeOverTime.Get("title").MustString())
	require.Equal(t, []promQuery{{
		Ref:    "A",
		Expr:   `sum by (instance) (grafana_alerting_active_alerts{job=~"$job", instance=~"$instance"})`,
		Legend: "{{instance}}",
	}}, promQueries(t, activeOverTime))

	byState := byID[6]
	require.Equal(t, "timeseries", byState.Get("type").MustString())
	require.Equal(t, "Results by State", byState.Get("title").MustString())
	require.Equal(t, "ops", byState.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, "normal", byState.Get("fieldConfig").Get("defaults").Get("custom").Get("stacking").Get("mode").MustString())
	require.Equal(t, []promQuery{{
		Ref:    "A",
		Expr:   `sum by (state) (rate(grafana_alerting_result_total{job=~"$job", instance=~"$instance"}[$__rate_interval]))`,
		Legend: "{{state}}",
	}}, promQueries(t, byState))

	notifications := byID[7]
	require.Equal(t, "timeseries", notifications.Get("type").MustString())
	require.Equal(t, "Notifications Sent vs Failed", notifications.Get("title").MustString())
	require.Equal(t, []colorOverride{
		{Matcher: "sent.*", Color: "green"},
		{Matcher: "failed.*", Color: "red"},
	}, colorOverrides(t, notifications))
	require.Equal(t, []promQuery{
		{
			Ref:    "A",
			Expr:   `sum by (type) (rate(grafana_alerting_notification_sent_total{job=~"$job", instance=~"$instance"}[$__rate_interval]))`,
			Legend: "sent - {{type}}",
		},
		{
			Ref:    "B",
			Expr:   `sum by (type) (rate(grafana_alerting_notification_failed_total{job=~"$job", instance=~"$instance"}[$__rate_interval]))`,
			Legend: "failed - {{type}}",
		},
	}, promQueries(t, notifications))

	duration := byID[8]
	require.Equal(t, "timeseries", duration.Get("type").MustString())
	require.Equal(t, "Evaluation Duration", duration.Get("title").MustString())
	require.Equal(t, "ms", duration.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []promQuery{
		{
			Ref:    "A",
			Expr:   `avg(grafana_alerting_execution_time_milliseconds{job=~"$job", instance=~"$instance", quantile="0.5"})`,
			Legend: "p50",
		},
		{
			Ref:    "B",
			Expr:   `avg(grafana_alerting_execution_time_milliseconds{job=~"$job", instance=~"$instance", quantile="0.99"})`,
			Legend: "p99",
		},
	}, promQueries(t, duration))

	byNotifier := byID[9]
	require.Equal(t, "table", byNotifier.Get("type").MustString())
	require.Equal(t, "Notification Failure Rate by Notifier", byNotifier.Get("title").MustString())
	require.Equal(t, "percentunit", byNotifier.Get("fieldConfig").Get("defaults").Get("unit").MustString())
	require.Equal(t, []thresholdStep{
		{Color: "green", Value: 0},
		{Color: "orange", Value: 0.01},
		{Color: "red", Value: 0.05},
	}, thresholdSteps(t, byNotifier))
	require.Equal(t, []promQuery{{
		Ref: "A",
		Expr: "sum by (type) (rate(grafana_alerting_notification_failed_total{job=~\"$job\", instance=~\"$instance\"}[5m])) / " +
			"clamp_min(sum by (type) (rate(grafana_alerting_notification_sent_total{job=~\"$job\", instance=~\"$instance\"}[5m])) + " +
			"sum by (type) (rate(grafana_alerting_notification_failed_total{job=~\"$job\", instance=~\"$instance\"}[5m])), 1e-9)",
		Format: "table",
	}}, promQueries(t, byNotifier))
	require.True(t, simplejson.NewFromAny(byNotifier.Get("targets").MustArray()[0]).Get("instant").MustBool())

	transforms := byNotifier.Get("transformations").MustArray()
	require.Len(t, transforms, 2)
	organize := simplejson.NewFromAny(transforms[0])
	require.Equal(t, "organize", organize.Get("id").MustString())
	require.Equal(t, "Failure rate", organize.Get("options").Get("renameByName").Get("Value").MustString())
	require.Equal(t, "Notifier", organize.Get("options").Get("renameByName").Get("type").MustString())
	require.True(t, organize.Get("options").Get("excludeByName").Get("Time").MustBool())
	sortBy := simplejson.NewFromAny(transforms[1])
	require.Equal(t, "sortBy", sortBy.Get("id").MustString())
	sortFields := sortBy.Get("options").Get("sort").MustArray()
	require.Len(t, sortFields, 1)
	require.Equal(t, "Failure rate", simplejson.NewFromAny(sortFields[0]).Get("field").MustString())
	require.True(t, simplejson.NewFromAny(sortFields[0]).Get("desc").MustBool())
}
