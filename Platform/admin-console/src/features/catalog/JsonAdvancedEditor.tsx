type JsonAdvancedEditorProps = {
  disabled?: boolean;
  label?: string;
  onChange: (value: string) => void;
  value: string;
};

export function JsonAdvancedEditor({
  disabled = false,
  label = '高级 JSON 配置',
  onChange,
  value,
}: JsonAdvancedEditorProps) {
  return (
    <label className="json-editor">
      <span>{label}</span>
      <textarea
        className="json-editor__textarea"
        disabled={disabled}
        onChange={event => onChange(event.target.value)}
        spellCheck={false}
        value={value}
      />
    </label>
  );
}
