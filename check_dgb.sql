SELECT 
    p.id as provider_id,
    p.name as provider_name,
    p.balance as provider_balance,
    pt.id as token_id,
    pt.name as token_name,
    pt.remain_quota,
    pt.unlimited_quota,
    pt.used_quota
FROM providers p
LEFT JOIN provider_tokens pt ON pt.provider_id = p.id
WHERE p.name LIKE '%DGB%' OR p.name LIKE '%公益%';
