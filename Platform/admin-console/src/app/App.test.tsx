import { render } from '@testing-library/react';

import App from './App';

describe('App', () => {
  it('renders the admin shell workspace', async () => {
    const view = render(<App />);

    expect(await view.findByRole('heading', { name: '平台仪表盘' })).toBeInTheDocument();
    expect(view.getByTestId('admin-shell-root')).toBeInTheDocument();
    expect(view.getByTestId('query-provider-ready')).toHaveTextContent('ready');
    expect(view.getByText('PinchBot 管理后台')).toBeInTheDocument();
    expect(view.getByRole('button', { name: '刷新后台' })).toBeInTheDocument();
  });
});
