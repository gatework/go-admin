-- Destructive local-development reset. Back up the database before explicit use.
BEGIN;
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM sys_user WHERE user_id = 1 AND username = 'admin' AND deleted_at = 0)
    OR NOT EXISTS (SELECT 1 FROM sys_role WHERE role_id = 1 AND role_key = 'admin' AND deleted_at = 0)
    OR NOT EXISTS (SELECT 1 FROM sys_dept WHERE dept_id = 1 AND deleted_at = 0)
    OR NOT EXISTS (SELECT 1 FROM sys_post WHERE post_id = 1 AND deleted_at = 0) THEN
    RAISE EXCEPTION 'Expected built-in admin, role, department and post are missing';
  END IF;
END $$;
DELETE FROM sys_role_menu WHERE role_id <> 1;
DELETE FROM sys_role_dept;
DELETE FROM casbin_rule WHERE v0 <> 'admin';
DELETE FROM sys_user WHERE user_id <> 1;
DELETE FROM sys_role WHERE role_id <> 1;
UPDATE sys_user SET dept_id = 1, role_id = 1, post_id = 1,
  nick_name = CASE WHEN nick_name = 'zhangwj' THEN '管理员' ELSE nick_name END,
  phone = CASE WHEN phone = '13818888888' THEN '' ELSE phone END,
  email = CASE WHEN email = '1@qq.com' THEN '' ELSE email END
WHERE user_id = 1;
DELETE FROM sys_dept WHERE dept_id <> 1;
DELETE FROM sys_post WHERE post_id <> 1;
UPDATE sys_dept SET parent_id = 0, dept_path = '/0/1/', dept_name = '默认部门', leader = 'admin', phone = '', email = '' WHERE dept_id = 1;
UPDATE sys_post SET post_name = '管理员', post_code = 'admin', remark = '默认管理岗位' WHERE post_id = 1;
UPDATE sys_config SET config_value = '' WHERE config_key = 'sys_app_logo' AND config_value IN ('https://doc-image.zhangwj.com/img/motor.png', 'https://gitee.com/mydearzwj/image/raw/master/img/motor.png');
INSERT INTO sys_job_lease (name, owner, acquired_at_ms, expires_at_ms)
VALUES ('scheduler', '', 0, 0) ON CONFLICT (name) DO NOTHING;
DELETE FROM sys_job;
DELETE FROM demo_product;
DELETE FROM tb_demo;
DELETE FROM sys_columns;
DELETE FROM sys_tables;
DELETE FROM sys_login_log;
DELETE FROM sys_opera_log;
UPDATE sys_api SET created_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP;
UPDATE sys_menu SET created_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP;
UPDATE sys_config SET created_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP;
UPDATE sys_dict_data SET created_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP;
UPDATE sys_dict_type SET created_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP;
UPDATE sys_dept SET created_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP;
UPDATE sys_post SET created_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP;
UPDATE sys_user SET created_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP;
UPDATE sys_role SET created_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP;
COMMIT;
