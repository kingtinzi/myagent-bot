import { useEffect } from 'react';

import { useUIStore } from '../../stores/uiStore';

export function ConfirmDialog() {
  const dialog = useUIStore(state => state.confirmDialog);
  const setConfirmInput = useUIStore(state => state.setConfirmInput);
  const resolveConfirmation = useUIStore(state => state.resolveConfirmation);

  useEffect(() => {
    if (!dialog.open) {
      return undefined;
    }

    const handleKeydown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        resolveConfirmation(false);
      }
    };

    globalThis.addEventListener('keydown', handleKeydown);
    return () => {
      globalThis.removeEventListener('keydown', handleKeydown);
    };
  }, [dialog.open, resolveConfirmation]);

  if (!dialog.open) {
    return null;
  }

  const requireText = dialog.requireText?.trim() ?? '';
  const confirmDisabled = requireText !== '' && dialog.inputValue.trim() !== requireText;
  const toneClassName = dialog.tone ? `is-${dialog.tone}` : 'is-warning';

  return (
    <div className="confirm-dialog-backdrop" onClick={() => resolveConfirmation(false)} role="presentation">
      <div
        aria-describedby="confirm-dialog-message"
        aria-labelledby="confirm-dialog-title"
        aria-modal="true"
        className={['confirm-dialog', toneClassName].join(' ')}
        onClick={event => event.stopPropagation()}
        role="dialog"
      >
        <div className="confirm-dialog__header">
          <h2 id="confirm-dialog-title">{dialog.title}</h2>
          {dialog.hint ? <p>{dialog.hint}</p> : null}
        </div>

        <p className="confirm-dialog__message" id="confirm-dialog-message">
          {dialog.message}
        </p>

        {requireText ? (
          <label className="confirm-dialog__field">
            请输入 <strong>{requireText}</strong> 以继续
            <input onChange={event => setConfirmInput(event.target.value)} value={dialog.inputValue} />
          </label>
        ) : null}

        <div className="confirm-dialog__actions">
          <button className="button button--ghost" onClick={() => resolveConfirmation(false)} type="button">
            {dialog.cancelLabel || '取消'}
          </button>
          <button
            className="button button--primary"
            disabled={confirmDisabled}
            onClick={() => resolveConfirmation(true)}
            type="button"
          >
            {dialog.confirmLabel || '确认'}
          </button>
        </div>
      </div>
    </div>
  );
}
