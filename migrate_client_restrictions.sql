-- 迁移客户端限制：从 ProviderToken 级别迁移到 ModelRoute 级别
-- 执行方式：sqlite3 gateway-aggregator.db < migrate_client_restrictions.sql

-- 将每个 Token 的客户端限制复制到其对应的所有路由上
UPDATE model_routes
SET
    allow_codex = (
        SELECT COALESCE(pt.allow_codex, 0)
        FROM provider_tokens pt
        WHERE pt.id = model_routes.provider_token_id
    ),
    allow_cc = (
        SELECT COALESCE(pt.allow_cc, 0)
        FROM provider_tokens pt
        WHERE pt.id = model_routes.provider_token_id
    ),
    block_clients = (
        SELECT COALESCE(pt.block_clients, 0)
        FROM provider_tokens pt
        WHERE pt.id = model_routes.provider_token_id
    )
WHERE EXISTS (
    SELECT 1 FROM provider_tokens pt
    WHERE pt.id = model_routes.provider_token_id
);

-- 显示迁移结果统计
SELECT
    '迁移完成' as status,
    COUNT(*) as total_routes,
    SUM(CASE WHEN allow_codex = 1 THEN 1 ELSE 0 END) as allow_codex_count,
    SUM(CASE WHEN allow_cc = 1 THEN 1 ELSE 0 END) as allow_cc_count,
    SUM(CASE WHEN block_clients = 1 THEN 1 ELSE 0 END) as block_clients_count
FROM model_routes;
