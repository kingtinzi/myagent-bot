import { render } from '@testing-library/react';

import App from './App';

describe('App', () => {
  it('renders the initial admin shell scaffold', async () => {
    const view = render(<App />);

    expect(await view.findByRole('heading', { name: 'PinchBot 管理后台（重构中）' })).toBeInTheDocument();
    expect(view.getByTestId('admin-shell-root')).toBeInTheDocument();
    expect(view.getByTestId('query-provider-ready')).toHaveTextContent('ready');
    expect(view.getByText('Wave 1：前端工程骨架搭建中。后续会在这个壳层中接入用户、钱包、订单、模型与治理模块。')).toBeInTheDocument();
  });
});
