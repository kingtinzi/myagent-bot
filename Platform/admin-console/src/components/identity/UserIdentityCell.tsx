type UserIdentityCellProps = {
  email?: string;
  userNo?: number;
  username?: string;
};

export function UserIdentityCell({ email, userNo, username }: UserIdentityCellProps) {
  const primary = username?.trim() || email?.trim() || '未命名用户';
  const secondary = userNo ? `#${userNo}` : '未分配编号';
  const tertiary = email?.trim() || '暂无邮箱';

  return (
    <div className="user-identity-cell">
      <strong>{primary}</strong>
      <span>{secondary}</span>
      <small>{tertiary}</small>
    </div>
  );
}
