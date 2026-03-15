create unique index if not exists wallet_transactions_reference_unique_idx
  on wallet_transactions (reference_type, reference_id)
  where reference_type <> '' and reference_id <> '';
