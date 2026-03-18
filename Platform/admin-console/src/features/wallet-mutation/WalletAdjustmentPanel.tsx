import { useEffect, useState } from 'react';

import { walletAdjustmentFormSchema } from '../../schemas/walletMutation';

type WalletAdjustmentPanelProps = {
  onSubmit: (payload: { userId: string; amountFen: number; description: string }) => Promise<unknown>;
  presetUserId?: string;
};

export function WalletAdjustmentPanel({ onSubmit, presetUserId = '' }: WalletAdjustmentPanelProps) {
  const [userId, setUserId] = useState(presetUserId);
  const [amountFen, setAmountFen] = useState('');
  const [description, setDescription] = useState('');
  const [errorMessage, setErrorMessage] = useState('');

  useEffect(() => {
    setUserId(presetUserId);
  }, [presetUserId]);

  return (
    <section className="panel">
      <div className="panel__header">
        <div>
          <h2>钱包调账</h2>
          <p>支持正负金额调账，所有操作都会进入统一审计日志。</p>
        </div>
      </div>
      <form
        className="form-stack"
        onSubmit={async event => {
          event.preventDefault();
          const result = walletAdjustmentFormSchema.safeParse({
            userId,
            amountFen,
            description,
          });

          if (!result.success) {
            setErrorMessage(result.error.issues[0]?.message ?? '调账表单校验失败。');
            return;
          }

          setErrorMessage('');
          await onSubmit(result.data);
          setAmountFen('');
          setDescription('');
        }}
      >
        <div className="form-grid">
          <label className="filter-field">
            <span>用户 ID</span>
            <input onChange={event => setUserId(event.target.value)} value={userId} />
          </label>
          <label className="filter-field">
            <span>金额（分，可负）</span>
            <input onChange={event => setAmountFen(event.target.value)} value={amountFen} />
          </label>
        </div>
        <label className="filter-field">
          <span>说明</span>
          <input onChange={event => setDescription(event.target.value)} value={description} />
        </label>
        {errorMessage ? <InlineFormError message={errorMessage} /> : null}
        <div className="form-actions">
          <button className="button button--ghost" type="submit">
            提交调账
          </button>
        </div>
      </form>
    </section>
  );
}

function InlineFormError({ message }: { message: string }) {
  return <p className="form-error">{message}</p>;
}
