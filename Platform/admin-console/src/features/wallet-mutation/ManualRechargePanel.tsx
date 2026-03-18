import { useEffect, useState } from 'react';

import { manualRechargeFormSchema } from '../../schemas/walletMutation';

type ManualRechargePanelProps = {
  onSubmit: (payload: { userId: string; amountFen: number; description: string }) => Promise<unknown>;
  presetUserId?: string;
};

export function ManualRechargePanel({ onSubmit, presetUserId = '' }: ManualRechargePanelProps) {
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
          <h2>管理员手动充值</h2>
          <p>直接为指定用户增加余额，并写入独立的管理员充值流水。</p>
        </div>
      </div>
      <form
        className="form-stack"
        onSubmit={async event => {
          event.preventDefault();
          const result = manualRechargeFormSchema.safeParse({
            userId,
            amountFen,
            description,
          });

          if (!result.success) {
            setErrorMessage(result.error.issues[0]?.message ?? '充值表单校验失败。');
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
            <span>金额（分）</span>
            <input onChange={event => setAmountFen(event.target.value)} value={amountFen} />
          </label>
        </div>
        <label className="filter-field">
          <span>说明</span>
          <input onChange={event => setDescription(event.target.value)} value={description} />
        </label>
        {errorMessage ? <InlineFormError message={errorMessage} /> : null}
        <div className="form-actions">
          <button className="button button--primary" type="submit">
            确认充值
          </button>
        </div>
      </form>
    </section>
  );
}

function InlineFormError({ message }: { message: string }) {
  return <p className="form-error">{message}</p>;
}
