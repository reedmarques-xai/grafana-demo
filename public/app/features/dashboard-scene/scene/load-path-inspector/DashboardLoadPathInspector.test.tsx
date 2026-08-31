import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { selectors } from '@grafana/e2e-selectors';
import { SceneTimeRange } from '@grafana/scenes';
import { ManagerKind } from 'app/features/apiserver/types';
import { type DashboardMeta } from 'app/types/dashboard';

import { DashboardScene } from '../DashboardScene';

import { DashboardLoadPathInspector } from './DashboardLoadPathInspector';

jest.mock('@grafana/api-clients', () => ({
  ...jest.requireActual('@grafana/api-clients'),
  getAPINamespace: () => 'default',
}));

function buildScene(uid: string | undefined, meta: DashboardMeta) {
  return new DashboardScene({
    uid,
    meta,
    $timeRange: new SceneTimeRange({ from: 'now-6h', to: 'now' }),
  });
}

const chip = selectors.pages.Dashboard.LoadPathInspector.chip;
const panel = selectors.pages.Dashboard.LoadPathInspector.panel;

describe('DashboardLoadPathInspector', () => {
  it('renders a collapsed chip showing the hop count and no hop list until opened', () => {
    const scene = buildScene('demo-system-metrics', { folderTitle: 'Demo Dashboards', folderUid: 'fld-1' });
    render(<DashboardLoadPathInspector dashboard={scene} />);

    expect(screen.getByTestId(chip)).toHaveTextContent('Load path');
    expect(screen.getByTestId(chip)).toHaveTextContent('6');
    expect(screen.queryByTestId(panel)).not.toBeInTheDocument();
  });

  it('expands to reveal the request URL and folder derived from the loaded scene', async () => {
    const scene = buildScene('demo-system-metrics', { folderTitle: 'Demo Dashboards', folderUid: 'fld-1' });
    render(<DashboardLoadPathInspector dashboard={scene} />);

    await userEvent.click(screen.getByTestId(chip));

    expect(screen.getByTestId(panel)).toBeInTheDocument();
    expect(
      screen.getByText('GET /apis/dashboard.grafana.app/v1beta1/namespaces/default/dashboards/demo-system-metrics/dto')
    ).toBeInTheDocument();
    expect(screen.getByText('Demo Dashboards')).toBeInTheDocument();
  });

  it('surfaces the provisioning manager and source path for a file-provisioned dashboard', async () => {
    const scene = buildScene('demo-provisioned-pipeline', {
      provisioned: true,
      k8s: {
        annotations: {
          'grafana.app/managedBy': ManagerKind.ClassicFP,
          'grafana.app/sourcePath': '/etc/dashboards/ingestion-pipeline.json',
        },
      },
    });
    render(<DashboardLoadPathInspector dashboard={scene} />);

    await userEvent.click(screen.getByTestId(chip));

    expect(screen.getByText('Provisioned · classic-file-provisioning')).toBeInTheDocument();
    expect(screen.getByText('/etc/dashboards/ingestion-pipeline.json')).toBeInTheDocument();
  });

  it('renders nothing for an unsaved dashboard that has no uid', () => {
    const scene = buildScene(undefined, {});
    const { container } = render(<DashboardLoadPathInspector dashboard={scene} />);
    expect(container).toBeEmptyDOMElement();
  });
});
