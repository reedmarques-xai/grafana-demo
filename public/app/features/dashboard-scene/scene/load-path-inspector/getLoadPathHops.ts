import { type IconName } from '@grafana/data';
import { t } from '@grafana/i18n';
import { AnnoKeyManagerIdentity, AnnoKeyManagerKind, AnnoKeySourcePath } from 'app/features/apiserver/types';
import { type DashboardMeta } from 'app/types/dashboard';

/** Which dashboard API client the frontend selected for this load. */
export type DashboardApiFlavor = 'v1' | 'v2' | 'unified';

/** A single step in the path a dashboard document travelled to reach the browser. */
export interface LoadPathHop {
  /** Stable identifier, also used for the per-hop e2e selector suffix. */
  key: string;
  icon: IconName;
  /** Short name of the layer, e.g. "Dashboard API". */
  title: string;
  /** The primary fact for this hop, e.g. the request URL or resource version. */
  summary: string;
  /** Optional secondary fact shown underneath the summary. */
  detail?: string;
}

/**
 * Everything the inspector needs to describe a load, gathered by the component from the
 * live scene state and runtime services. Kept as plain data so the hop derivation stays
 * pure and unit-testable.
 */
export interface LoadPathContext {
  uid: string;
  meta: DashboardMeta;
  /** Result of getDashboardsApiVersion() — the client family that was used. */
  apiFlavor: DashboardApiFlavor;
  /** Concrete negotiated group version, e.g. "v1beta1". */
  apiVersion: string;
  /** API namespace (org/stack) the request was scoped to. */
  namespace: string;
  /** Measured duration of the `/dto` request in milliseconds, when the Performance API exposed it. */
  fetchDurationMs?: number;
}

const DASHBOARD_API_GROUP = 'dashboard.grafana.app';

/** Build the canonical `/dto` request URL for the load, mirroring ScopedResourceClient. */
export function getDtoRequestUrl(ctx: Pick<LoadPathContext, 'apiVersion' | 'namespace' | 'uid'>): string {
  return `/apis/${DASHBOARD_API_GROUP}/${ctx.apiVersion}/namespaces/${ctx.namespace}/dashboards/${ctx.uid}/dto`;
}

function describeApiFlavor(flavor: DashboardApiFlavor): string {
  switch (flavor) {
    case 'v2':
      return t('dashboard.load-path-inspector.api-detail-v2', 'v2 client (new layouts)');
    case 'v1':
      return t('dashboard.load-path-inspector.api-detail-v1', 'v1 client');
    case 'unified':
    default:
      return t('dashboard.load-path-inspector.api-detail-unified', 'Unified client (v1, falls back to v2)');
  }
}

/**
 * Turn a load context into an ordered list of hops, top (browser) to bottom (storage/source).
 * Every hop is derived from a fact Grafana already exposes about the loaded dashboard, so the
 * output reads as a tour of the request path rather than synthesised data.
 */
export function getLoadPathHops(ctx: LoadPathContext): LoadPathHop[] {
  const { uid, meta, apiFlavor, apiVersion } = ctx;
  const k8s = meta.k8s;
  const annotations = k8s?.annotations ?? {};
  const hops: LoadPathHop[] = [];

  hops.push({
    key: 'route',
    icon: 'compass',
    title: t('dashboard.load-path-inspector.hop-route-title', 'Browser route'),
    summary: `/d/${uid}`,
    detail: t('dashboard.load-path-inspector.hop-route-detail', 'Rendered by DashboardScenePage'),
  });

  hops.push({
    key: 'api',
    icon: 'code-branch',
    title: t('dashboard.load-path-inspector.hop-api-title', 'Dashboard API'),
    summary: `${DASHBOARD_API_GROUP}/${apiVersion}`,
    detail: describeApiFlavor(apiFlavor),
  });

  const fetchDetail =
    ctx.fetchDurationMs != null
      ? t('dashboard.load-path-inspector.hop-fetch-detail-timed', 'Completed in {{ms}} ms', {
          ms: Math.round(ctx.fetchDurationMs),
        })
      : t('dashboard.load-path-inspector.hop-fetch-detail', 'Requested through backendSrv');
  hops.push({
    key: 'fetch',
    icon: 'exchange-alt',
    title: t('dashboard.load-path-inspector.hop-fetch-title', 'Resource fetch'),
    summary: `GET ${getDtoRequestUrl(ctx)}`,
    detail: fetchDetail,
  });

  const resourceVersion = k8s?.resourceVersion;
  const generation = k8s?.generation;
  let storageSummary: string;
  if (resourceVersion) {
    storageSummary = t('dashboard.load-path-inspector.hop-storage-rv', 'resourceVersion {{rv}}', {
      rv: resourceVersion,
    });
  } else if (meta.version != null) {
    storageSummary = t('dashboard.load-path-inspector.hop-storage-version', 'version {{version}}', {
      version: meta.version,
    });
  } else {
    storageSummary = t('dashboard.load-path-inspector.hop-storage-unknown', 'version not reported');
  }
  hops.push({
    key: 'storage',
    icon: 'database',
    title: t('dashboard.load-path-inspector.hop-storage-title', 'Unified storage'),
    summary: storageSummary,
    detail:
      generation != null
        ? t('dashboard.load-path-inspector.hop-storage-generation', 'generation {{generation}}', { generation })
        : undefined,
  });

  const folderUid = meta.folderUid;
  hops.push({
    key: 'folder',
    icon: 'folder',
    title: t('dashboard.load-path-inspector.hop-folder-title', 'Folder'),
    summary:
      meta.folderTitle ||
      (folderUid ? folderUid : t('dashboard.load-path-inspector.hop-folder-root', 'Dashboards (root)')),
    detail: folderUid
      ? t('dashboard.load-path-inspector.hop-folder-uid', 'uid {{folderUid}}', { folderUid })
      : t('dashboard.load-path-inspector.hop-folder-none', 'No parent folder'),
  });

  const managerKind = annotations[AnnoKeyManagerKind];
  const managerIdentity = annotations[AnnoKeyManagerIdentity];
  const sourcePath = annotations[AnnoKeySourcePath] ?? meta.provisionedExternalId;
  let sourceSummary: string;
  let sourceDetail: string | undefined;
  if (managerKind) {
    sourceSummary = t('dashboard.load-path-inspector.hop-source-managed', 'Provisioned · {{manager}}', {
      manager: managerKind,
    });
    sourceDetail = sourcePath || managerIdentity;
  } else if (meta.provisioned) {
    sourceSummary = t('dashboard.load-path-inspector.hop-source-provisioned', 'Provisioned');
    sourceDetail = sourcePath;
  } else {
    sourceSummary = t('dashboard.load-path-inspector.hop-source-db', 'Grafana database');
    sourceDetail = t('dashboard.load-path-inspector.hop-source-db-detail', 'Saved via API / UI');
  }
  hops.push({
    key: 'source',
    icon: 'file-alt',
    title: t('dashboard.load-path-inspector.hop-source-title', 'Source'),
    summary: sourceSummary,
    detail: sourceDetail,
  });

  return hops;
}
