import { DataTable } from '../../components/data/DataTable';
import type { WalletSummary, WalletTransaction } from '../../services/contracts';

type UserWalletTabProps = {
  transactions: WalletTransaction[];
  wallet: WalletSummary;
};

export function UserWalletTab({ transactions, wallet }: UserWalletTabProps) {
  return (
    <div className="detail-tab-stack">
      <div className="detail-summary-card">
        <strong>{`${wallet.currency} ${(wallet.balance_fen / 100).toFixed(2)}`}</strong>
        <p>当前钱包余额</p>
      </div>
      <DataTable
        caption="用户钱包流水"
        columns={[
          { key: 'kind', header: '类型', cell: row => row.kind },
          { key: 'amount', header: '金额（分）', cell: row => row.amount_fen.toLocaleString('zh-CN') },
          { key: 'description', header: '说明', cell: row => row.description },
          { key: 'reference', header: '参考', cell: row => row.reference_type || '—' },
        ]}
        getRowKey={row => row.id}
        rows={transactions}
      />
    </div>
  );
}
