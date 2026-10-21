-- 057: api_logs 敏感字段加密（OPT-SEC-04）
--
-- 存量数据策略说明：
--   ip_address / user_agent 的新写入原在应用层的 ops 统计仓储里经
--   internal/pkg/dbencrypt 加密为 `enc:v1:{base64}` 密文。该写入口在
--   2026-10-21 的零生产消费者清理中已随该仓储文件一并移除（写方法无任何
--   调用点），所以本迁移现在**只做字段长度保护**：存量明文行保留原样
--   （纯 SQL 无法实现 AES-256-GCM —— nonce 随机生成、密钥仅存在于应用侧
--   MASTER_KEY，严禁下发到数据库），读取出口 Decrypt 的透传兼容
--   （非 enc:v1: 前缀原样返回）仍按原逻辑保留。
--
-- 本迁移仅做字段长度保护：密文 = "enc:v1:" + base64(nonce|ciphertext|tag)，
-- 比明文长约 2~4 倍，varchar(45) 无法容纳加密后的 IP，需扩容。全部幂等写法。

-- 1) user_agent 保证为 text（无长度上限）；仅当当前类型非 text 时变更，避免无谓重写
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'api_logs'
          AND column_name = 'user_agent'
          AND data_type <> 'text'
    ) THEN
        ALTER TABLE api_logs ALTER COLUMN user_agent TYPE text;
    END IF;
END $$;

-- 2) ip_address 扩容至 varchar(255) 以容纳 enc:v1: 密文；仅当当前长度不足时变更
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'api_logs'
          AND column_name = 'ip_address'
          AND (character_maximum_length IS NULL OR character_maximum_length < 255)
    ) THEN
        ALTER TABLE api_logs ALTER COLUMN ip_address TYPE varchar(255);
    END IF;
END $$;
