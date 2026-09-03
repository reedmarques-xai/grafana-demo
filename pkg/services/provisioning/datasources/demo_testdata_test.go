package datasources

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/services/datasources"
	"github.com/grafana/grafana/pkg/services/org"
	"github.com/grafana/grafana/pkg/services/org/orgtest"
)

func TestDemoTestDataDatasourceProvisioning(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))

	orgFake := &orgtest.FakeOrgService{ExpectedOrg: &org.Org{ID: 1}}
	cr := configReader{log: log.New("test-logger"), orgService: orgFake}
	cfgs, err := cr.readConfig(context.Background(), filepath.Join(root, "conf", "provisioning", "datasources"))
	require.NoError(t, err)

	var testdata *upsertDataSourceFromConfig
	for _, cfg := range cfgs {
		for _, ds := range cfg.Datasources {
			if ds != nil && ds.UID == "testdata" {
				testdata = ds
			}
		}
	}
	require.NotNil(t, testdata, "uid testdata missing from conf/provisioning/datasources")
	require.Equal(t, "TestData", testdata.Name)
	require.Equal(t, "grafana-testdata-datasource", testdata.Type)
	require.Equal(t, string(datasources.DS_ACCESS_PROXY), testdata.Access)
	require.True(t, testdata.IsDefault)
	require.Equal(t, int64(1), testdata.OrgID)
}
