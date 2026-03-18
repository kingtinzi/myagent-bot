import type { ReactNode } from 'react';

type DataTableColumn<T> = {
  key: string;
  header: ReactNode;
  cell: (row: T) => ReactNode;
  className?: string;
};

type DataTableProps<T> = {
  caption?: string;
  columns: DataTableColumn<T>[];
  emptyMessage?: string;
  getRowKey: (row: T) => string;
  onRowClick?: (row: T) => void;
  rows: T[];
};

export function DataTable<T>({
  caption,
  columns,
  emptyMessage = '当前没有数据。',
  getRowKey,
  onRowClick,
  rows,
}: DataTableProps<T>) {
  return (
    <div className="data-table-shell">
      <div className="data-table-scroll">
        <table className="data-table">
          {caption ? <caption className="sr-only">{caption}</caption> : null}
          <thead>
            <tr>
              {columns.map(column => (
                <th className={column.className} key={column.key} scope="col">
                  {column.header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.length === 0 ? (
              <tr>
                <td className="data-table__empty" colSpan={columns.length}>
                  {emptyMessage}
                </td>
              </tr>
            ) : (
              rows.map(row => (
                <tr
                  className={onRowClick ? 'is-clickable' : ''}
                  key={getRowKey(row)}
                  onClick={onRowClick ? () => onRowClick(row) : undefined}
                >
                  {columns.map(column => (
                    <td className={column.className} key={column.key}>
                      {column.cell(row)}
                    </td>
                  ))}
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
