import { DataTable } from '../../components/data/DataTable';
import type { AgreementAcceptance } from '../../services/contracts';

type UserAgreementsTabProps = {
  agreements: AgreementAcceptance[];
};

export function UserAgreementsTab({ agreements }: UserAgreementsTabProps) {
  return (
    <DataTable
      caption="用户协议签署记录"
      columns={[
        { key: 'key', header: '协议', cell: row => row.agreement_key },
        { key: 'version', header: '版本', cell: row => row.version },
        { key: 'accepted', header: '签署时间', cell: row => new Date(row.accepted_unix * 1000).toLocaleString('zh-CN') },
        { key: 'device', header: '设备', cell: row => row.device_summary || '—' },
      ]}
      getRowKey={row => `${row.agreement_key}-${row.version}-${row.accepted_unix}`}
      rows={agreements}
    />
  );
}
