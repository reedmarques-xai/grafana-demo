import { css } from '@emotion/css';
import { useEffect, useMemo, useState } from 'react';

import { getAPINamespace } from '@grafana/api-clients';
import { type GrafanaTheme2 } from '@grafana/data';
import { selectors } from '@grafana/e2e-selectors';
import { Trans, t } from '@grafana/i18n';
import { Icon, IconButton, useStyles2 } from '@grafana/ui';

import { dashboardAPIVersionResolver } from '../../../dashboard/api/DashboardAPIVersionResolver';
import { getDashboardsApiVersion } from '../../../dashboard/api/utils';
import { type DashboardScene } from '../DashboardScene';

import { type DashboardApiFlavor, type LoadPathContext, getLoadPathHops } from './getLoadPathHops';

interface Props {
  dashboard: DashboardScene;
}

/** Read the measured duration of the dashboard `/dto` request from the Performance API, if present. */
function readFetchDuration(uid: string): number | undefined {
  if (typeof performance === 'undefined' || typeof performance.getEntriesByType !== 'function') {
    return undefined;
  }
  const needle = `/dashboards/${uid}/dto`;
  const entries = performance.getEntriesByType('resource');
  // Prefer the most recent matching request in case the dashboard was reloaded.
  for (let i = entries.length - 1; i >= 0; i--) {
    if (entries[i].name.includes(needle)) {
      return entries[i].duration;
    }
  }
  return undefined;
}

function resolveApiVersion(flavor: DashboardApiFlavor): string {
  return flavor === 'v2' ? dashboardAPIVersionResolver.getV2() : dashboardAPIVersionResolver.getV1();
}

/**
 * A small chip anchored to the dashboard that expands into the ordered list of hops a dashboard
 * document travelled to reach the browser. It is a thin read-only view over facts Grafana already
 * exposes on the loaded scene (route, selected API client, request URL, storage resource version,
 * folder, and provisioning source) — useful as a tour of the load path while debugging.
 */
export function DashboardLoadPathInspector({ dashboard }: Props) {
  const { uid, meta } = dashboard.useState();
  const styles = useStyles2(getStyles);
  const [expanded, setExpanded] = useState(false);
  const [fetchDurationMs, setFetchDurationMs] = useState<number | undefined>(undefined);

  useEffect(() => {
    if (uid) {
      setFetchDurationMs(readFetchDuration(uid));
    }
  }, [uid]);

  const hops = useMemo(() => {
    if (!uid) {
      return [];
    }
    const apiFlavor = getDashboardsApiVersion();
    const ctx: LoadPathContext = {
      uid,
      meta,
      apiFlavor,
      apiVersion: resolveApiVersion(apiFlavor),
      namespace: getAPINamespace(),
      fetchDurationMs,
    };
    return getLoadPathHops(ctx);
  }, [uid, meta, fetchDurationMs]);

  // Only meaningful for a persisted dashboard that was actually loaded from the backend.
  // A dashboard with no uid is unsaved/new, so there is no load path to show.
  if (!uid || meta.isEmbedded) {
    return null;
  }

  if (!expanded) {
    return (
      <button
        type="button"
        className={styles.chip}
        onClick={() => setExpanded(true)}
        data-testid={selectors.pages.Dashboard.LoadPathInspector.chip}
        aria-label={t('dashboard.load-path-inspector.chip-aria', 'Show dashboard load path')}
      >
        <Icon name="sitemap" size="sm" />
        <Trans i18nKey="dashboard.load-path-inspector.chip-label">Load path</Trans>
        <span className={styles.chipCount}>{hops.length}</span>
      </button>
    );
  }

  return (
    <div className={styles.panel} data-testid={selectors.pages.Dashboard.LoadPathInspector.panel}>
      <div className={styles.header}>
        <span className={styles.headerTitle}>
          <Icon name="sitemap" size="sm" />
          <Trans i18nKey="dashboard.load-path-inspector.panel-title">Dashboard load path</Trans>
        </span>
        <IconButton
          name="times"
          size="sm"
          onClick={() => setExpanded(false)}
          tooltip={t('dashboard.load-path-inspector.collapse', 'Collapse')}
          aria-label={t('dashboard.load-path-inspector.collapse', 'Collapse')}
        />
      </div>
      <ol className={styles.hopList}>
        {hops.map((hop) => (
          <li
            key={hop.key}
            className={styles.hop}
            data-testid={`${selectors.pages.Dashboard.LoadPathInspector.hop}-${hop.key}`}
          >
            <span className={styles.hopIcon}>
              <Icon name={hop.icon} size="sm" />
            </span>
            <span className={styles.hopBody}>
              <span className={styles.hopTitle}>{hop.title}</span>
              <span className={styles.hopSummary}>{hop.summary}</span>
              {hop.detail && <span className={styles.hopDetail}>{hop.detail}</span>}
            </span>
          </li>
        ))}
      </ol>
    </div>
  );
}

function getStyles(theme: GrafanaTheme2) {
  return {
    chip: css({
      position: 'fixed',
      bottom: theme.spacing(2),
      right: theme.spacing(2),
      zIndex: theme.zIndex.tooltip,
      display: 'inline-flex',
      alignItems: 'center',
      gap: theme.spacing(0.75),
      padding: theme.spacing(0.5, 1),
      border: `1px solid ${theme.colors.border.weak}`,
      borderRadius: theme.shape.radius.pill,
      background: theme.colors.background.secondary,
      color: theme.colors.text.secondary,
      boxShadow: theme.shadows.z1,
      fontSize: theme.typography.bodySmall.fontSize,
      cursor: 'pointer',
      '&:hover': {
        color: theme.colors.text.primary,
        borderColor: theme.colors.border.medium,
      },
    }),
    chipCount: css({
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      minWidth: theme.spacing(2),
      height: theme.spacing(2),
      padding: theme.spacing(0, 0.5),
      borderRadius: theme.shape.radius.pill,
      background: theme.colors.background.canvas,
      color: theme.colors.text.secondary,
      fontSize: theme.typography.pxToRem(11),
    }),
    panel: css({
      position: 'fixed',
      bottom: theme.spacing(2),
      right: theme.spacing(2),
      zIndex: theme.zIndex.tooltip,
      width: 360,
      maxWidth: `calc(100vw - ${theme.spacing(4)})`,
      maxHeight: `calc(100vh - ${theme.spacing(12)})`,
      overflowY: 'auto',
      border: `1px solid ${theme.colors.border.weak}`,
      borderRadius: theme.shape.radius.default,
      background: theme.colors.background.primary,
      boxShadow: theme.shadows.z3,
    }),
    header: css({
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      padding: theme.spacing(1, 1.5),
      borderBottom: `1px solid ${theme.colors.border.weak}`,
    }),
    headerTitle: css({
      display: 'inline-flex',
      alignItems: 'center',
      gap: theme.spacing(1),
      fontWeight: theme.typography.fontWeightMedium,
    }),
    hopList: css({
      listStyle: 'none',
      margin: 0,
      padding: theme.spacing(1, 0),
    }),
    hop: css({
      display: 'flex',
      gap: theme.spacing(1.5),
      padding: theme.spacing(1, 1.5),
      position: 'relative',
      // Connector line between consecutive hop icons.
      '&:not(:last-child)::before': {
        content: '""',
        position: 'absolute',
        left: theme.spacing(2.25),
        top: theme.spacing(3),
        bottom: 0,
        width: 1,
        background: theme.colors.border.weak,
      },
    }),
    hopIcon: css({
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      flexShrink: 0,
      width: theme.spacing(3),
      height: theme.spacing(3),
      borderRadius: theme.shape.radius.circle,
      border: `1px solid ${theme.colors.border.weak}`,
      background: theme.colors.background.secondary,
      color: theme.colors.text.secondary,
      zIndex: 1,
    }),
    hopBody: css({
      display: 'flex',
      flexDirection: 'column',
      minWidth: 0,
    }),
    hopTitle: css({
      fontSize: theme.typography.bodySmall.fontSize,
      color: theme.colors.text.secondary,
    }),
    hopSummary: css({
      fontFamily: theme.typography.fontFamilyMonospace,
      fontSize: theme.typography.bodySmall.fontSize,
      color: theme.colors.text.primary,
      overflowWrap: 'anywhere',
    }),
    hopDetail: css({
      fontSize: theme.typography.pxToRem(11),
      color: theme.colors.text.secondary,
      overflowWrap: 'anywhere',
    }),
  };
}
