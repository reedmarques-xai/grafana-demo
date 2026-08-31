import { ManagerKind } from 'app/features/apiserver/types';
import { type DashboardMeta } from 'app/types/dashboard';

import { type LoadPathContext, getLoadPathHops } from './getLoadPathHops';

function buildContext(overrides: Partial<LoadPathContext> = {}): LoadPathContext {
  return {
    uid: 'abc',
    meta: {},
    apiFlavor: 'unified',
    apiVersion: 'v1beta1',
    namespace: 'default',
    ...overrides,
  };
}

/** Reduce hops to the fields we assert on, keyed by hop for readable expectations. */
function byKey(ctx: LoadPathContext) {
  return Object.fromEntries(getLoadPathHops(ctx).map((h) => [h.key, { summary: h.summary, detail: h.detail }]));
}

describe('getLoadPathHops', () => {
  it('emits the six load-path hops in browser-to-storage order', () => {
    const hops = getLoadPathHops(buildContext());
    expect(hops.map((h) => h.key)).toEqual(['route', 'api', 'fetch', 'storage', 'folder', 'source']);
    expect(hops.map((h) => h.icon)).toEqual([
      'compass',
      'code-branch',
      'exchange-alt',
      'database',
      'folder',
      'file-alt',
    ]);
  });

  it('builds the route and /dto request URL from the uid, namespace and negotiated version', () => {
    const hops = byKey(buildContext({ uid: 'demo-system-metrics', namespace: 'org-3', apiVersion: 'v1beta1' }));
    expect(hops.route.summary).toBe('/d/demo-system-metrics');
    expect(hops.fetch.summary).toBe(
      'GET /apis/dashboard.grafana.app/v1beta1/namespaces/org-3/dashboards/demo-system-metrics/dto'
    );
  });

  it.each([
    ['unified', 'v1beta1', 'Unified client (v1, falls back to v2)'],
    ['v1', 'v1beta1', 'v1 client'],
    ['v2', 'v2beta1', 'v2 client (new layouts)'],
  ] as const)('describes the %s API client', (apiFlavor, apiVersion, detail) => {
    const hops = byKey(buildContext({ apiFlavor, apiVersion }));
    expect(hops.api.summary).toBe(`dashboard.grafana.app/${apiVersion}`);
    expect(hops.api.detail).toBe(detail);
  });

  it('reports the measured fetch duration rounded to whole milliseconds when available', () => {
    expect(byKey(buildContext({ fetchDurationMs: 12.7 })).fetch.detail).toBe('Completed in 13 ms');
  });

  it('falls back to a generic fetch note when no timing was captured', () => {
    expect(byKey(buildContext({ fetchDurationMs: undefined })).fetch.detail).toBe('Requested through backendSrv');
  });

  it('prefers the k8s resourceVersion and generation for the storage hop', () => {
    const meta: DashboardMeta = { version: 4, k8s: { resourceVersion: '1788193195579017', generation: 2 } };
    const hops = byKey(buildContext({ meta }));
    expect(hops.storage.summary).toBe('resourceVersion 1788193195579017');
    expect(hops.storage.detail).toBe('generation 2');
  });

  it('falls back to the legacy version and omits generation when k8s metadata is absent', () => {
    const hops = byKey(buildContext({ meta: { version: 4 } }));
    expect(hops.storage.summary).toBe('version 4');
    expect(hops.storage.detail).toBeUndefined();
  });

  it('shows the parent folder title and uid when the dashboard lives in a folder', () => {
    const meta: DashboardMeta = { folderUid: 'fld-1', folderTitle: 'Demo Dashboards' };
    const hops = byKey(buildContext({ meta }));
    expect(hops.folder.summary).toBe('Demo Dashboards');
    expect(hops.folder.detail).toBe('uid fld-1');
  });

  it('marks a dashboard with no folder as living at the root', () => {
    const hops = byKey(buildContext({ meta: {} }));
    expect(hops.folder.summary).toBe('Dashboards (root)');
    expect(hops.folder.detail).toBe('No parent folder');
  });

  it('names the manager and source path for a file-provisioned dashboard', () => {
    const meta: DashboardMeta = {
      provisioned: true,
      provisionedExternalId: 'ingestion-pipeline.json',
      k8s: {
        annotations: {
          'grafana.app/managedBy': ManagerKind.ClassicFP,
          'grafana.app/sourcePath': '/etc/dashboards/ingestion-pipeline.json',
        },
      },
    };
    const hops = byKey(buildContext({ meta }));
    expect(hops.source.summary).toBe('Provisioned · classic-file-provisioning');
    expect(hops.source.detail).toBe('/etc/dashboards/ingestion-pipeline.json');
  });

  it('labels a database-backed dashboard as saved via API / UI', () => {
    const hops = byKey(buildContext({ meta: { provisioned: false } }));
    expect(hops.source.summary).toBe('Grafana database');
    expect(hops.source.detail).toBe('Saved via API / UI');
  });
});
