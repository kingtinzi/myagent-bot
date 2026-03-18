type ReconcileActionBarProps = {
  onReconcile: () => Promise<void>;
};

export function ReconcileActionBar({ onReconcile }: ReconcileActionBarProps) {
  return (
    <div className="form-actions">
      <button className="button button--primary" onClick={() => void onReconcile()} type="button">
        立即对账
      </button>
    </div>
  );
}
