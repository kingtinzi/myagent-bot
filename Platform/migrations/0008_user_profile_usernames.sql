alter table user_profiles
  add column if not exists username text not null default '';

update user_profiles
set username = case
  when coalesce(trim(username), '') <> '' then trim(username)
  when coalesce(trim(email), '') <> '' then split_part(trim(email), '@', 1)
  when coalesce(user_no, 0) > 0 then '用户' || user_no::text
  else '用户'
end
where coalesce(trim(username), '') = '';
