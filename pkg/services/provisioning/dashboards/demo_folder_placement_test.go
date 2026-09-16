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

// The dummy dashboards are only useful if they land in the shared Demo
// Dashboards folder. FileReader.getOrCreateFolder uses both the folder title
// and folderUid; drifting either one creates a second folder and hides every
// provisioned demo dashboard from the UI the demos are advertised under.
const (
	demoProviderName          = "demo-dummy-data"
	demoFolderTitle           = "Demo Dashboards"
	demoFolderUID             = "ffyg8julzmscga"
	alertingOperationsFile    = "alerting-operations.json"
	alertingOperationsUID     = "demo-alerting"
	alertingOperationsTitle   = "Demo: Grafana Alerting Operations"
	alertingNotifierTableName = "Notification Failure Rate by Notifier"
)

func TestDemoDashboardsFolderPlacement(t *testing.T) {
	demo := demoDummyDataConfig(t)

	require.Equal(t, int64(1), demo.OrgID)
	require.Equal(t, demoFolderTitle, demo.Folder)
	require.Equal(t, demoFolderUID, demo.FolderUID)
	require.Equal(t, "file", demo.Type)
	require.Equal(t, "conf/provisioning/dashboards/json", demo.Options["path"])

	jsonDir := filepath.Join(demoFolderRepoRoot(t), "conf", "provisioning", "dashboards", "json")
	entries, err := os.ReadDir(jsonDir)
	require.NoError(t, err)

	gotUIDs := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data := mustReadDashboardJSON(t, filepath.Join(jsonDir, entry.Name()))
		dash, err := createDashboardJSON(data, time.Unix(0, 0), demo, 0, demo.FolderUID)
		require.NoError(t, err, "%s must provision into folder %q", entry.Name(), demoFolderUID)
		require.Equal(t, demoFolderUID, dash.Dashboard.FolderUID, "%s FolderUID", entry.Name())
		require.Equal(t, demo.OrgID, dash.OrgID, "%s OrgID", entry.Name())
		require.NotEmpty(t, dash.Dashboard.UID, "%s missing uid", entry.Name())
		require.NotEmpty(t, dash.Dashboard.Title, "%s missing title", entry.Name())

		if prev, dup := gotUIDs[dash.Dashboard.UID]; dup {
			t.Fatalf("duplicate dashboard uid %q in %s and %s", dash.Dashboard.UID, prev, entry.Name())
		}
		gotUIDs[dash.Dashboard.UID] = entry.Name()
	}

	require.Equal(t, alertingOperationsFile, gotUIDs[alertingOperationsUID],
		"alerting operations dashboard must keep uid %q so existing Demo Dashboards links keep working", alertingOperationsUID)
	require.GreaterOrEqual(t, len(gotUIDs), 5, "expected the full set of dummy dashboards, got %v", gotUIDs)
}

func TestAlertingOperationsUIDAndNotifierTable(t *testing.T) {
	demo := demoDummyDataConfig(t)
	path := filepath.Join(demoFolderRepoRoot(t), "conf", "provisioning", "dashboards", "json", alertingOperationsFile)

	data := mustReadDashboardJSON(t, path)
	dash, err := createDashboardJSON(data, time.Unix(0, 0), demo, 0, demo.FolderUID)
	require.NoError(t, err)
	require.Equal(t, alertingOperationsUID, dash.Dashboard.UID)
	require.Equal(t, alertingOperationsTitle, dash.Dashboard.Title)
	require.Equal(t, demoFolderUID, dash.Dashboard.FolderUID)
	require.Equal(t, []string{"demo", "dummy", "alerting"}, data.Get("tags").MustStringArray())

	table := panelByTitle(t, data, alertingNotifierTableName)
	require.Equal(t, "table", table.Get("type").MustString())

	var csvContent string
	for _, raw := range table.Get("targets").MustArray() {
		target := simplejson.NewFromAny(raw)
		require.Equal(t, "testdata", target.Get("datasource").Get("uid").MustString())
		require.Equal(t, "csv_content", target.Get("scenarioId").MustString())
		csvContent = target.Get("csvContent").MustString()
	}
	require.NotEmpty(t, csvContent, "notifier table is missing csvContent")

	rows, err := csv.NewReader(strings.NewReader(csvContent)).ReadAll()
	require.NoError(t, err)
	require.Equal(t, [][]string{
		{"Notifier", "Failure rate", "Sent (5m)", "Failed (5m)"},
		{"email", "0.012", "420", "5"},
		{"slack", "0.048", "180", "9"},
		{"pagerduty", "0.002", "90", "0"},
		{"webhook", "0.081", "55", "5"},
	}, rows)
}

func demoDummyDataConfig(t *testing.T) *config {
	t.Helper()

	providers, err := ReadDashboardConfig(filepath.Join(demoFolderRepoRoot(t), "conf", "provisioning", "dashboards"))
	require.NoError(t, err)

	var demo *config
	sameFolderUID := 0
	for i := range providers {
		if providers[i].FolderUID == demoFolderUID {
			sameFolderUID++
		}
		if providers[i].Name == demoProviderName {
			demo = &providers[i].config
		}
	}
	require.NotNil(t, demo, "%s provider missing from conf/provisioning/dashboards", demoProviderName)
	require.Equal(t, 1, sameFolderUID, "folderUid %q must be unique across dashboard providers", demoFolderUID)
	return demo
}

func mustReadDashboardJSON(t *testing.T, path string) *simplejson.Json {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err, path)
	data, err := simplejson.NewJson(raw)
	require.NoError(t, err, "%s must be valid JSON", path)
	return data
}

func panelByTitle(t *testing.T, data *simplejson.Json, title string) *simplejson.Json {
	t.Helper()
	for _, raw := range data.Get("panels").MustArray() {
		panel := simplejson.NewFromAny(raw)
		if panel.Get("title").MustString() == title {
			return panel
		}
	}
	t.Fatalf("panel %q not found", title)
	return nil
}

func demoFolderRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
}
