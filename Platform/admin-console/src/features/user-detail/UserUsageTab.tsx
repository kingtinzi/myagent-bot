import { DataTable } from '../../components/data/DataTable';
import type { ChatUsageRecord } from '../../services/contracts';

type UserUsageTabProps = {
  items: ChatUsageRecord[];
};

export function UserUsageTab({ items }: UserUsageTabProps) {
  return (
    <DataTable
      caption="用户模型用量记录"
      columns={[
        { key: 'model', header: '模型', cell: row => row.model_id },
        { key: 'charged', header: '收费（分）', cell: row => row.charged_fen.toLocaleString('zh-CN') },
        { key: 'tokens', header: 'Token', cell: row => `${row.prompt_tokens ?? 0} / ${row.completion_tokens ?? 0}` },
        { key: 'created', header: '时间', cell: row => new Date(row.created_unix * 1000).toLocaleString('zh-CN') },
      ]}
      getRowKey={row => row.id}
      rows={items}
    />
  );
}
