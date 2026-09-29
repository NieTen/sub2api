-- 宿主扩展能力由管理员按插件单独开启，已有安装默认关闭，不改变插件配置和出站绑定。
-- 读写均通过安装记录主键定位，复用主键索引，无需为布尔开关新增索引。
ALTER TABLE sub2api_plugin_installations
    ADD COLUMN IF NOT EXISTS host_adaptation_enabled BOOLEAN NOT NULL DEFAULT FALSE;
