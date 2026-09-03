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

// These files are the demo-repo provisioning contract: if YAML or JSON drifts,
// local Grafana either fails to provision or shows panels with no TestData.
func TestDemoProvisionedDashboards(t *testing.T) {
	root := grafanaRepoRoot(t)
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
	require.Equal(t, int64(1), demo.OrgID)
	require.Equal(t, "Demo", demo.Folder)
	require.Equal(t, "demo-dummy", demo.FolderUID)
	require.Equal(t, "file", demo.Type)
	require.Equal(t, true, demo.AllowUIUpdates)
	require.Equal(t, int64(10), demo.UpdateIntervalSeconds)

	jsonDir, ok := demo.Options["path"].(string)
	require.True(t, ok, "provider options.path must be a string")
	require.Equal(t, "conf/provisioning/dashboards/json", jsonDir)

	absJSON := filepath.Join(root, jsonDir)
	entries, err := os.ReadDir(absJSON)
	require.NoError(t, err)

	want := map[string]string{
		"demo-service-overview": "Demo: Service Overview",
		"demo-infrastructure":   "Demo: Infrastructure",
		"demo-observability":    "Demo: Logs, traces & dependencies",
	}

	gotUIDs := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		raw, err := os.ReadFile(filepath.Join(absJSON, entry.Name()))
		require.NoError(t, err, entry.Name())

		data, err := simplejson.NewJson(raw)
		require.NoError(t, err, "%s must be valid JSON", entry.Name())

		dash, err := createDashboardJSON(data, time.Unix(0, 0), demo, 0, demo.FolderUID)
		require.NoError(t, err, "%s must load as a provisioned dashboard", entry.Name())

		uid := dash.Dashboard.UID
		require.NotEmpty(t, uid, "%s must have a uid", entry.Name())
		if prev, dup := gotUIDs[uid]; dup {
			t.Fatalf("duplicate dashboard uid %q in %s and %s", uid, prev, entry.Name())
		}
		gotUIDs[uid] = entry.Name()

		wantTitle, known := want[uid]
		require.True(t, known, "%s has unexpected uid %q", entry.Name(), uid)
		require.Equal(t, wantTitle, dash.Dashboard.Title)

		require.Equal(t, demo.FolderUID, dash.Dashboard.FolderUID)
		require.Equal(t, demo.OrgID, dash.OrgID)

		testdataUIDs := collectTestdataUIDs(data)
		require.NotEmpty(t, testdataUIDs, "%s has no grafana-testdata-datasource queries", entry.Name())
		for _, dsUID := range testdataUIDs {
			require.Equal(t, "testdata", dsUID, "%s testdata query/panel uid must match the provisioned datasource", entry.Name())
		}

		ids := map[int]struct{}{}
		for i, panel := range data.Get("panels").MustArray() {
			id := simplejson.NewFromAny(panel).Get("id").MustInt(0)
			require.NotZero(t, id, "%s panel[%d] is missing id", entry.Name(), i)
			if _, dup := ids[id]; dup {
				t.Fatalf("%s has duplicate panel id %d", entry.Name(), id)
			}
			ids[id] = struct{}{}
		}
	}

	require.Equal(t, len(want), len(gotUIDs), "expected the three demo dashboards, got %v", gotUIDs)
}

func collectTestdataUIDs(data *simplejson.Json) []string {
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
	return uids
}

func grafanaRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
}
