-- 开始初始化数据 ;
SET IDENTITY_INSERT sys_api ON;
INSERT INTO sys_api (id, handle, title, path, type, "action", created_at, updated_at, deleted_at, create_by, update_by)VALUES 
(5, 'GET /api/v1/sys-login-log/:id', '登录日志通过id获取', '/api/v1/sys-login-log/:id', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(6, 'GET /api/v1/sys-opera-log', '操作日志列表', '/api/v1/sys-opera-log', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(7, 'GET /api/v1/sys-opera-log/:id', '操作日志通过id获取', '/api/v1/sys-opera-log/:id', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(8, 'GET /api/v1/syscategory', '分类列表', '/api/v1/syscategory', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(9, 'GET /api/v1/syscategory/:id', '分类通过id获取', '/api/v1/syscategory/:id', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(10, 'GET /api/v1/syscontent', '内容列表', '/api/v1/syscontent', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(11, 'GET /api/v1/syscontent/:id', '内容通过id获取', '/api/v1/syscontent/:id', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(15, 'GET /api/v1/sysjob', 'job列表', '/api/v1/sysjob', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(16, 'GET /api/v1/sysjob/:id', 'job通过id获取', '/api/v1/sysjob/:id', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(21, 'GET /api/v1/dict/type', '字典类型列表', '/api/v1/dict/type', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(22, 'GET /api/v1/dict/type-option-select', '字典类型查询【代码生成】', '/api/v1/dict/type-option-select', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(23, 'GET /api/v1/dict/type/:id', '字典类型通过id获取', '/api/v1/dict/type/:id', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(24, 'GET /api/v1/dict/data', '字典数据列表', '/api/v1/dict/data', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(25, 'GET /api/v1/dict/data/:dictCode', '字典数据通过code获取', '/api/v1/dict/data/:dictCode', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(26, 'GET /api/v1/dict-data/option-select', '数据字典根据key获取', '/api/v1/dict-data/option-select', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(27, 'GET /api/v1/dept', '部门列表', '/api/v1/dept', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(28, 'GET /api/v1/dept/:id', '部门通过id获取', '/api/v1/dept/:id', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(29, 'GET /api/v1/deptTree', '查询部门下拉树【角色权限-自定权限】', '/api/v1/deptTree', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(30, 'GET /api/v1/db/tables/page', '数据库表列表', '/api/v1/db/tables/page', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(31, 'GET /api/v1/db/columns/page', '数据表列列表', '/api/v1/db/columns/page', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(32, 'GET /api/v1/gen/toproject/:tableId', '数据库表生成到项目', '/api/v1/gen/toproject/:tableId', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(33, 'GET /api/v1/gen/todb/:tableId', '数据库表生成到DB', '/api/v1/gen/todb/:tableId', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(34, 'GET /api/v1/gen/tabletree', '关系表数据【代码生成】', '/api/v1/gen/tabletree', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(35, 'GET /api/v1/gen/preview/:tableId', '生成预览通过id获取', '/api/v1/gen/preview/:tableId', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(36, 'GET /api/v1/gen/apitofile/:tableId', '生成api带文件', '/api/v1/gen/apitofile/:tableId', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(37, 'GET /api/v1/getCaptcha', '验证码获取', '/api/v1/getCaptcha', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(38, 'GET /api/v1/getinfo', '用户信息获取', '/api/v1/getinfo', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(39, 'GET /api/v1/menu', '菜单列表', '/api/v1/menu', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(40, 'GET /api/v1/menuTreeselect', '查询菜单下拉树结构【废弃】', '/api/v1/menuTreeselect', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(41, 'GET /api/v1/menu/:id', '菜单通过id获取', '/api/v1/menu/:id', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(42, 'GET /api/v1/menurole', '角色菜单【顶部左侧菜单】', '/api/v1/menurole', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(43, 'GET /api/v1/menuids', '获取角色对应的菜单id数组【废弃】', '/api/v1/menuids', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(44, 'GET /api/v1/role', '角色列表', '/api/v1/role', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(45, 'GET /api/v1/roleMenuTreeselect/:roleId', '菜单权限列表【角色配菜单使用】', '/api/v1/roleMenuTreeselect/:roleId', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(46, 'GET /api/v1/roleDeptTreeselect/:roleId', '角色部门结构树【自定义数据权限】', '/api/v1/roleDeptTreeselect/:roleId', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(47, 'GET /api/v1/role/:id', '角色通过id获取', '/api/v1/role/:id', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(53, 'GET /api/v1/config', '参数列表', '/api/v1/config', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(54, 'GET /api/v1/config/:id', '参数通过id获取', '/api/v1/config/:id', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(55, 'GET /api/v1/configKey/:configKey', '参数通过键名搜索【基础默认配置】', '/api/v1/configKey/:configKey', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(57, 'GET /api/v1/job/remove/:id', 'job移除', '/api/v1/job/remove/:id', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(58, 'GET /api/v1/job/start/:id', 'job启动', '/api/v1/job/start/:id', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(59, 'GET /api/v1/post', '岗位列表', '/api/v1/post', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(60, 'GET /api/v1/post/:id', '岗位通过id获取', '/api/v1/post/:id', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(62, 'GET /api/v1/app-config', '系统前端参数', '/api/v1/app-config', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(63, 'GET /api/v1/user/profile', '*用户信息获取', '/api/v1/user/profile', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(66, 'github.com/gatework/motor-core/sdk/pkg/ws.(*Manager).WsClient-fm', '链接ws【定时任务log】', '/ws/:id/:channel', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(67, 'github.com/gatework/motor-core/sdk/pkg/ws.(*Manager).UnWsClient-fm', '退出ws【定时任务log】', '/wslogout/:id/:channel', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(68, 'GET /info', '*用户基本信息', '/info', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(72, 'POST /api/v1/syscategory', '分类创建', '/api/v1/syscategory', 'BUS', 'POST', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(73, 'POST /api/v1/syscontent', '内容创建', '/api/v1/syscontent', 'BUS', 'POST', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(76, 'POST /api/v1/sysjob', 'job创建', '/api/v1/sysjob', 'BUS', 'POST', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(80, 'POST /api/v1/dict/data', '字典数据创建', '/api/v1/dict/data', 'BUS', 'POST', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(81, 'POST /api/v1/dict/type', '字典类型创建', '/api/v1/dict/type', 'BUS', 'POST', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(82, 'POST /api/v1/dept', '部门创建', '/api/v1/dept', 'BUS', 'POST', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(85, 'github.com/gatework/motor-core/jwtauth.(*GinJWTMiddleware).LoginHandler-fm', '*登录', '/api/v1/login', 'SYS', 'POST', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(86, 'POST /api/v1/logout', '*退出', '/api/v1/logout', 'SYS', 'POST', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(87, 'POST /api/v1/config', '参数创建', '/api/v1/config', 'BUS', 'POST', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(88, 'POST /api/v1/menu', '菜单创建', '/api/v1/menu', 'BUS', 'POST', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(89, 'POST /api/v1/post', '岗位创建', '/api/v1/post', 'BUS', 'POST', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(90, 'POST /api/v1/role', '角色创建', '/api/v1/role', 'BUS', 'POST', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(91, 'POST /api/v1/user/avatar', '*用户头像编辑', '/api/v1/user/avatar', 'SYS', 'POST', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(92, 'PUT /api/v1/sys-api/:id', '接口编辑', '/api/v1/sys-api/:id', 'BUS', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(95, 'PUT /api/v1/syscategory/:id', '分类编辑', '/api/v1/syscategory/:id', 'BUS', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(96, 'PUT /api/v1/syscontent/:id', '内容编辑', '/api/v1/syscontent/:id', 'BUS', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(97, 'PUT /api/v1/sysjob', 'job编辑', '/api/v1/sysjob', 'BUS', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(101, 'PUT /api/v1/dict/data/:dictCode', '字典数据编辑', '/api/v1/dict/data/:dictCode', 'BUS', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(102, 'PUT /api/v1/dict/type/:id', '字典类型编辑', '/api/v1/dict/type/:id', 'BUS', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(103, 'PUT /api/v1/dept/:id', '部门编辑', '/api/v1/dept/:id', 'BUS', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(104, 'PUT /api/v1/file-dir/:id', '文件夹编辑', '/api/v1/file-dir/:id', 'BUS', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(105, 'PUT /api/v1/file-info/:id', '文件编辑', '/api/v1/file-info/:id', 'BUS', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(106, 'PUT /api/v1/role/:id', '角色编辑', '/api/v1/role/:id', 'BUS', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(107, 'PUT /api/v1/roledatascope', '角色数据权限修改', '/api/v1/roledatascope', 'BUS', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(108, 'PUT /api/v1/config/:id', '参数编辑', '/api/v1/config/:id', 'BUS', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(109, 'PUT /api/v1/menu/:id', '编辑菜单', '/api/v1/menu/:id', 'BUS', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(110, 'PUT /api/v1/post/:id', '岗位编辑', '/api/v1/post/:id', 'BUS', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(111, 'PUT /api/v1/user/pwd', '*用户修改密码', '/api/v1/user/pwd', 'SYS', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(112, 'DELETE /api/v1/syscategory', '分类删除', '/api/v1/syscategory', 'BUS', 'DELETE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(113, 'DELETE /api/v1/syscontent', '内容删除', '/api/v1/syscontent', 'BUS', 'DELETE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(114, 'DELETE /api/v1/sys-login-log', '登录日志删除', '/api/v1/sys-login-log', 'BUS', 'DELETE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(115, 'DELETE /api/v1/sys-opera-log', '操作日志删除', '/api/v1/sys-opera-log', 'BUS', 'DELETE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(116, 'DELETE /api/v1/sysjob', 'job删除', '/api/v1/sysjob', 'BUS', 'DELETE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(117, 'DELETE /api/v1/sys-area-data', '行政区删除', '/api/v1/sys-area-data', 'BUS', 'DELETE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(120, 'DELETE /api/v1/dict/data', '字典数据删除', '/api/v1/dict/data', 'BUS', 'DELETE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(121, 'DELETE /api/v1/dict/type', '字典类型删除', '/api/v1/dict/type', 'BUS', 'DELETE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(122, 'DELETE /api/v1/dept/:id', '部门删除', '/api/v1/dept/:id', 'BUS', 'DELETE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(123, 'DELETE /api/v1/file-dir/:id', '文件夹删除', '/api/v1/file-dir/:id', 'BUS', 'DELETE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(124, 'DELETE /api/v1/file-info/:id', '文件删除', '/api/v1/file-info/:id', 'BUS', 'DELETE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(125, 'DELETE /api/v1/config', '参数删除', '/api/v1/config', 'BUS', 'DELETE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(126, 'DELETE /api/v1/menu', '删除菜单', '/api/v1/menu', 'BUS', 'DELETE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(127, 'DELETE /api/v1/post/:id', '岗位删除', '/api/v1/post/:id', 'BUS', 'DELETE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(128, 'DELETE /api/v1/role', '角色删除', '/api/v1/role', 'BUS', 'DELETE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(131, 'github.com/gatework/motor-core/tools/transfer.Handler.func1', '系统指标', '/api/v1/metrics', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(132, 'GET /api/v1/health', '健康状态', '/api/v1/health', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(133, 'GET /', '项目默认接口', '/', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(134, 'GET /api/v1/server-monitor', '服务器基本状态', '/api/v1/server-monitor', 'SYS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(135, 'GET /api/v1/sys-api', '接口列表', '/api/v1/sys-api', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(136, 'GET /api/v1/sys-api/:id', '接口通过id获取', '/api/v1/sys-api/:id', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(137, 'GET /api/v1/sys-login-log', '登录日志列表', '/api/v1/sys-login-log', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(138, 'POST /api/v1/public/uploadFile', '文件上传', '/api/v1/public/uploadFile', 'SYS', 'POST', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(139, 'PUT /api/v1/set-config', '参数信息修改【参数配置】', '/api/v1/set-config', 'BUS', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(140, 'GET /api/v1/set-config', '参数获取全部【配置使用】', '/api/v1/set-config', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(141, 'GET /api/v1/sys-user', '管理员列表', '/api/v1/sys-user', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(142, 'GET /api/v1/sys-user/:id', '管理员通过id获取', '/api/v1/sys-user/:id', 'BUS', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(143, 'GET /api/v1/sys/tables/info', '', '/api/v1/sys/tables/info', '', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(144, 'GET /api/v1/sys/tables/info/:tableId', '', '/api/v1/sys/tables/info/:tableId', '', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(145, 'GET /api/v1/sys/tables/page', '', '/api/v1/sys/tables/page', '', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(146, 'github.com/gin-gonic/gin.(*RouterGroup).createStaticHandler.func1', '', '/static/*filepath', '', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(147, 'github.com/swaggo/gin-swagger.CustomWrapHandler.func1', '', '/swagger/*any', '', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(148, 'github.com/gin-gonic/gin.(*RouterGroup).createStaticHandler.func1', '', '/form-generator/*filepath', '', 'GET', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(149, 'POST /api/v1/sys/tables/info', '', '/api/v1/sys/tables/info', '', 'POST', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(150, 'POST /api/v1/sys-user', '管理员创建', '/api/v1/sys-user', 'BUS', 'POST', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(151, 'PUT /api/v1/sys-user', '管理员编辑', '/api/v1/sys-user', 'BUS', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(152, 'PUT /api/v1/sys/tables/info', '', '/api/v1/sys/tables/info', '', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(153, 'PUT /api/v1/role-status', '', '/api/v1/role-status', '', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(154, 'PUT /api/v1/user/pwd/reset', '', '/api/v1/user/pwd/reset', '', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(155, 'PUT /api/v1/user/status', '', '/api/v1/user/status', '', 'PUT', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(156, 'DELETE /api/v1/sys-user', '管理员删除', '/api/v1/sys-user', 'BUS', 'DELETE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(157, 'DELETE /api/v1/sys/tables/info/:tableId', '', '/api/v1/sys/tables/info/:tableId', '', 'DELETE', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(158, 'github.com/gin-gonic/gin.(*RouterGroup).createStaticHandler.func1', '', '/static/*filepath', '', 'HEAD', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0),
(159, 'github.com/gin-gonic/gin.(*RouterGroup).createStaticHandler.func1', '', '/form-generator/*filepath', '', 'HEAD', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, 0, 0);
SET IDENTITY_INSERT sys_api OFF;

INSERT INTO sys_config VALUES
(1, '皮肤样式', 'sys_index_skinName', 'skin-green', 'Y', '0', '主框架页-默认皮肤样式名称:蓝色 skin-blue、绿色 skin-green、紫色 skin-purple、红色 skin-red、黄色 skin-yellow', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(2, '初始密码', 'sys_user_initPassword', '123456', 'Y', '0', '用户管理-账号初始密码:123456', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(3, '侧栏主题', 'sys_index_sideTheme', 'theme-dark', 'Y', '0', '主框架页-侧边栏主题:深色主题theme-dark，浅色主题theme-light', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(4, '系统名称', 'sys_app_name', 'motor', 'Y', '1', '', 1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(5, '系统logo', 'sys_app_logo', '', 'Y', '1', '', 1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);

SET IDENTITY_INSERT sys_dept ON;
INSERT INTO sys_dept (dept_id, parent_id, dept_path, dept_name, sort, leader, phone, email, status, create_by, update_by, created_at, updated_at, deleted_at)VALUES
(1, 0, '/0/1/', '默认部门', 0, 'admin', '', '', '2', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
SET IDENTITY_INSERT sys_dept OFF;

INSERT INTO sys_dict_data (dict_code, dict_sort, dict_label, dict_value, dict_type, css_class, list_class, is_default,
                           status, "default", remark, create_by, update_by, created_at, updated_at, deleted_at)VALUES 
(1, 0, '正常', '2', 'sys_normal_disable', '', '', '', '2', '', '系统正常', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (2, 0, '停用', '1', 'sys_normal_disable', '', '', '', '2', '', '系统停用', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (3, 0, '男', '0', 'sys_user_sex', '', '', '', '2', '', '性别男', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (4, 0, '女', '1', 'sys_user_sex', '', '', '', '2', '', '性别女', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (5, 0, '未知', '2', 'sys_user_sex', '', '', '', '2', '', '性别未知', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (6, 0, '显示', '0', 'sys_show_hide', '', '', '', '2', '', '显示菜单', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (7, 0, '隐藏', '1', 'sys_show_hide', '', '', '', '2', '', '隐藏菜单', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (8, 0, '是', 'Y', 'sys_yes_no', '', '', '', '2', '', '系统默认是', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (9, 0, '否', 'N', 'sys_yes_no', '', '', '', '2', '', '系统默认否', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (10, 0, '正常', '2', 'sys_job_status', '', '', '', '2', '', '正常状态', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (11, 0, '停用', '1', 'sys_job_status', '', '', '', '2', '', '停用状态', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (12, 0, '默认', 'DEFAULT', 'sys_job_group', '', '', '', '2', '', '默认分组', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (13, 0, '系统', 'SYSTEM', 'sys_job_group', '', '', '', '2', '', '系统分组', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (14, 0, '通知', '1', 'sys_notice_type', '', '', '', '2', '', '通知', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (15, 0, '公告', '2', 'sys_notice_type', '', '', '', '2', '', '公告', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (16, 0, '正常', '2', 'sys_common_status', '', '', '', '2', '', '正常状态', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (17, 0, '关闭', '1', 'sys_common_status', '', '', '', '2', '', '关闭状态', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (18, 0, '新增', '1', 'sys_oper_type', '', '', '', '2', '', '新增操作', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (19, 0, '修改', '2', 'sys_oper_type', '', '', '', '2', '', '修改操作', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (20, 0, '删除', '3', 'sys_oper_type', '', '', '', '2', '', '删除操作', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (21, 0, '授权', '4', 'sys_oper_type', '', '', '', '2', '', '授权操作', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (22, 0, '导出', '5', 'sys_oper_type', '', '', '', '2', '', '导出操作', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (23, 0, '导入', '6', 'sys_oper_type', '', '', '', '2', '', '导入操作', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (24, 0, '强退', '7', 'sys_oper_type', '', '', '', '2', '', '强退操作', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (25, 0, '生成代码', '8', 'sys_oper_type', '', '', '', '2', '', '生成操作', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (26, 0, '清空数据', '9', 'sys_oper_type', '', '', '', '2', '', '清空操作', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (27, 0, '成功', '0', 'sys_notice_status', '', '', '', '2', '', '成功状态', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (28, 0, '失败', '1', 'sys_notice_status', '', '', '', '2', '', '失败状态', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (29, 0, '登录', '10', 'sys_oper_type', '', '', '', '2', '', '登录操作', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (30, 0, '退出', '11', 'sys_oper_type', '', '', '', '2', '', '', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (31, 0, '获取验证码', '12', 'sys_oper_type', '', '', '', '2', '', '获取验证码', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (32, 0, '正常', '1', 'sys_content_status', '', '', '', '1', '', '', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_data VALUES (33, 1, '禁用', '2', 'sys_content_status', '', '', '', '1', '', '', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);


INSERT INTO sys_dict_type (dict_id, dict_name, dict_type, status, remark, create_by, update_by, created_at, updated_at,
                           deleted_at)VALUES (1, '系统开关', 'sys_normal_disable', '2', '系统开关列表', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_type VALUES (2, '用户性别', 'sys_user_sex', '2', '用户性别列表', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_type VALUES (3, '菜单状态', 'sys_show_hide', '2', '菜单状态列表', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_type VALUES (4, '系统是否', 'sys_yes_no', '2', '系统是否列表', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_type VALUES (5, '任务状态', 'sys_job_status', '2', '任务状态列表', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_type VALUES (6, '任务分组', 'sys_job_group', '2', '任务分组列表', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_type VALUES (7, '通知类型', 'sys_notice_type', '2', '通知类型列表', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_type VALUES (8, '系统状态', 'sys_common_status', '2', '登录状态列表', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_type VALUES (9, '操作类型', 'sys_oper_type', '2', '操作类型列表', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_type VALUES (10, '通知状态', 'sys_notice_status', '2', '通知状态列表', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_dict_type VALUES (11, '内容状态', 'sys_content_status', '2', '', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);



SET IDENTITY_INSERT sys_menu ON;
INSERT INTO sys_menu (menu_id, menu_name, title, icon, path, paths, menu_type, "action", permission, parent_id, no_cache, breadcrumb, component, sort, visible, is_frame, create_by, update_by, created_at, updated_at, deleted_at)VALUES 
(2, 'Admin', '系统管理', 'apps', '/admin', '/0/2', 'M', '无', '', 0, 1, '', 'Layout', 10, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(3, 'SysUserManage', '用户管理', 'user', '/admin/sys-user', '/0/2/3', 'C', '无', 'admin:sysUser:list', 2, 0, '', '/admin/sys-user/index', 10, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(43, '', '新增管理员', 'app-group-fill', '', '/0/2/3/43', 'F', 'POST', 'admin:sysUser:add', 3, 0, '', '', 10, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(44, '', '查询管理员', 'app-group-fill', '', '/0/2/3/44', 'F', 'GET', 'admin:sysUser:query', 3, 0, '', '', 40, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(45, '', '修改管理员', 'app-group-fill', '', '/0/2/3/45', 'F', 'PUT', 'admin:sysUser:edit', 3, 0, '', '', 30, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(46, '', '删除管理员', 'app-group-fill', '', '/0/2/3/46', 'F', 'DELETE', 'admin:sysUser:remove', 3, 0, '', '', 20, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(51, 'SysMenuManage', '菜单管理', 'menu', '/admin/sys-menu', '/0/2/51', 'C', '无', 'admin:sysMenu:list', 2, 1, '', '/admin/sys-menu/index', 30, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(52, 'SysRoleManage', '角色管理', 'safe', '/admin/sys-role', '/0/2/52', 'C', '无', 'admin:sysRole:list', 2, 1, '', '/admin/sys-role/index', 20, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(56, 'SysDeptManage', '部门管理', 'branch', '/admin/sys-dept', '/0/2/56', 'C', '无', 'admin:sysDept:list', 2, 0, '', '/admin/sys-dept/index', 40, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(57, 'SysPostManage', '岗位管理', 'idcard', '/admin/sys-post', '/0/2/57', 'C', '无', 'admin:sysPost:list', 2, 0, '', '/admin/sys-post/index', 50, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(58, 'Dict', '字典管理', 'book', '/admin/dict', '/0/2/58', 'C', '无', 'admin:sysDictType:list', 2, 0, '', '/admin/dict/index', 60, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(59, 'SysDictDataManage', '字典数据', 'unordered-list', '/admin/dict/data/:dictId', '/0/2/59', 'C', '无', 'admin:sysDictData:list', 2, 0, '', '/admin/dict/data', 100, '1', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(60, 'Tools', '开发工具', 'code-square', '/dev-tools', '/0/60', 'M', '无', '', 0, 0, '', 'Layout', 40, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(61, 'Swagger', '系统接口', 'code-block', '/dev-tools/swagger', '/0/60/61', 'C', '无', '', 60, 0, '', '/dev-tools/swagger/index', 1, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(62, 'SysConfigManage', '参数管理', 'storage', '/admin/sys-config', '/0/2/62', 'C', '无', 'admin:sysConfig:list', 2, 0, '', '/admin/sys-config/index', 70, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(211, 'Log', '日志管理', 'folder', '/log', '/0/2/211', 'M', '', '', 2, 0, '', '/log/index', 80, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(212, 'SysLoginLogManage', '登录日志', 'history', '/admin/sys-login-log', '/0/2/211/212', 'C', '', 'admin:sysLoginLog:list', 211, 0, '', '/admin/sys-login-log/index', 1, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(216, 'OperLog', '操作日志', 'list', '/admin/sys-oper-log', '/0/2/211/216', 'C', '', 'admin:sysOperLog:list', 211, 0, '', '/admin/sys-oper-log/index', 1, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(220, '', '新增菜单', 'app-group-fill', '', '/0/2/51/220', 'F', '', 'admin:sysMenu:add', 51, 0, '', '', 1, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(221, '', '修改菜单', 'app-group-fill', '', '/0/2/51/221', 'F', '', 'admin:sysMenu:edit', 51, 0, '', '', 1, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(222, '', '查询菜单', 'app-group-fill', '', '/0/2/51/222', 'F', '', 'admin:sysMenu:query', 51, 0, '', '', 1, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(223, '', '删除菜单', 'app-group-fill', '', '/0/2/51/223', 'F', '', 'admin:sysMenu:remove', 51, 0, '', '', 1, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(224, '', '新增角色', 'app-group-fill', '', '/0/2/52/224', 'F', '', 'admin:sysRole:add', 52, 0, '', '', 1, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(225, '', '查询角色', 'app-group-fill', '', '/0/2/52/225', 'F', '', 'admin:sysRole:query', 52, 0, '', '', 1, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(226, '', '修改角色', 'app-group-fill', '', '/0/2/52/226', 'F', '', 'admin:sysRole:update', 52, 0, '', '', 1, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(227, '', '删除角色', 'app-group-fill', '', '/0/2/52/227', 'F', '', 'admin:sysRole:remove', 52, 0, '', '', 1, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(228, '', '查询部门', 'app-group-fill', '', '/0/2/56/228', 'F', '', 'admin:sysDept:query', 56, 0, '', '', 40, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(229, '', '新增部门', 'app-group-fill', '', '/0/2/56/229', 'F', '', 'admin:sysDept:add', 56, 0, '', '', 10, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(230, '', '修改部门', 'app-group-fill', '', '/0/2/56/230', 'F', '', 'admin:sysDept:edit', 56, 0, '', '', 30, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(231, '', '删除部门', 'app-group-fill', '', '/0/2/56/231', 'F', '', 'admin:sysDept:remove', 56, 0, '', '', 20, '0', '1', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(232, '', '查询岗位', 'app-group-fill', '', '/0/2/57/232', 'F', '', 'admin:sysPost:query', 57, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(233, '', '新增岗位', 'app-group-fill', '', '/0/2/57/233', 'F', '', 'admin:sysPost:add', 57, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(234, '', '修改岗位', 'app-group-fill', '', '/0/2/57/234', 'F', '', 'admin:sysPost:edit', 57, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(235, '', '删除岗位', 'app-group-fill', '', '/0/2/57/235', 'F', '', 'admin:sysPost:remove', 57, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(236, '', '查询字典', 'app-group-fill', '', '/0/2/58/236', 'F', '', 'admin:sysDictType:query', 58, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(237, '', '新增类型', 'app-group-fill', '', '/0/2/58/237', 'F', '', 'admin:sysDictType:add', 58, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(238, '', '修改类型', 'app-group-fill', '', '/0/2/58/238', 'F', '', 'admin:sysDictType:edit', 58, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(239, '', '删除类型', 'app-group-fill', '', '/0/2/58/239', 'F', '', 'system:sysdicttype:remove', 58, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(240, '', '查询数据', 'app-group-fill', '', '/0/2/59/240', 'F', '', 'admin:sysDictData:query', 59, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(241, '', '新增数据', 'app-group-fill', '', '/0/2/59/241', 'F', '', 'admin:sysDictData:add', 59, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(242, '', '修改数据', 'app-group-fill', '', '/0/2/59/242', 'F', '', 'admin:sysDictData:edit', 59, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(243, '', '删除数据', 'app-group-fill', '', '/0/2/59/243', 'F', '', 'admin:sysDictData:remove', 59, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(244, '', '查询参数', 'app-group-fill', '', '/0/2/62/244', 'F', '', 'admin:sysConfig:query', 62, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(245, '', '新增参数', 'app-group-fill', '', '/0/2/62/245', 'F', '', 'admin:sysConfig:add', 62, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(246, '', '修改参数', 'app-group-fill', '', '/0/2/62/246', 'F', '', 'admin:sysConfig:edit', 62, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(247, '', '删除参数', 'app-group-fill', '', '/0/2/62/247', 'F', '', 'admin:sysConfig:remove', 62, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(248, '', '查询登录日志', 'app-group-fill', '', '/0/2/211/212/248', 'F', '', 'admin:sysLoginLog:query', 212, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(249, '', '删除登录日志', 'app-group-fill', '', '/0/2/211/212/249', 'F', '', 'admin:sysLoginLog:remove', 212, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(250, '', '查询操作日志', 'app-group-fill', '', '/0/2/211/216/250', 'F', '', 'admin:sysOperLog:query', 216, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(251, '', '删除操作日志', 'app-group-fill', '', '/0/2/211/216/251', 'F', '', 'admin:sysOperLog:remove', 216, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(261, 'Gen', '代码生成', 'code', '/dev-tools/gen', '/0/60/261', 'C', '', '', 60, 0, '', '/dev-tools/gen/index', 2, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(262, 'EditTable', '代码生成修改', 'edit', '/dev-tools/editTable', '/0/60/262', 'C', '', '', 60, 0, '', '/dev-tools/gen/editTable', 100, '1', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(264, 'Build', '表单构建', 'interaction', '/dev-tools/build', '/0/60/264', 'C', '', '', 60, 0, '', '/dev-tools/build/index', 1, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(269, 'ServerMonitor', '服务监控', 'desktop', '/sys-tools/monitor', '/0/60/269', 'C', '', 'sysTools:serverMonitor:list', 537, 0, '', '/sys-tools/monitor', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(459, 'Schedule', '定时任务', 'calendar-clock', '/schedule', '/0/459', 'M', '无', '', 0, 0, '', 'Layout', 20, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(460, 'ScheduleManage', 'Schedule', 'schedule', '/schedule/manage', '/0/459/460', 'C', '无', 'job:sysJob:list', 459, 0, '', '/schedule/index', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(461, 'sys_job', '分页获取定时任务', 'app-group-fill', '', '/0/459/460/461', 'F', '无', 'job:sysJob:query', 460, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(462, 'sys_job', '创建定时任务', 'app-group-fill', '', '/0/459/460/462', 'F', '无', 'job:sysJob:add', 460, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(463, 'sys_job', '修改定时任务', 'app-group-fill', '', '/0/459/460/463', 'F', '无', 'job:sysJob:edit', 460, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(464, 'sys_job', '删除定时任务', 'app-group-fill', '', '/0/459/460/464', 'F', '无', 'job:sysJob:remove', 460, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(471, 'JobLog', '日志', 'file', '/schedule/log', '/0/459/471', 'C', '', '', 459, 0, '', '/schedule/log', 0, '1', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(528, 'SysApiManage', '接口管理', 'link', '/admin/sys-api', '/0/527/528', 'C', '无', 'admin:sysApi:list', 2, 0, '', '/admin/sys-api/index', 0, '0', '0', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(529, '', '查询接口', 'app-group-fill', '', '/0/527/528/529', 'F', '无', 'admin:sysApi:query', 528, 0, '', '', 40, '0', '0', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(531, '', '修改接口', 'app-group-fill', '', '/0/527/528/531', 'F', '无', 'admin:sysApi:edit', 528, 0, '', '', 30, '0', '0', 0, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(537, 'SysTools', '系统工具', 'tool', '/sys-tools', '', 'M', '', '', 0, 0, '', 'Layout', 30, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(540, 'SysConfigSet', '参数设置', 'settings', '/admin/sys-config/set', '', 'C', '', 'admin:sysConfigSet:list', 2, 0, '', '/admin/sys-config/set', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL),
(542, '', '修改', 'upload', '', '', 'F', '', 'admin:sysConfigSet:update', 540, 0, '', '', 0, '0', '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
SET IDENTITY_INSERT sys_menu OFF;
INSERT INTO sys_menu_api_rule (sys_menu_menu_id, sys_api_id)VALUES (216, 6);
(250, 6),
(58, 21),
(236, 21),
(238, 23),
(59, 24),
(240, 24),
(242, 25),
(58, 26),
(236, 26),
(56, 27),
(228, 27),
(230, 28),
(226, 29),
(51, 39),
(51, 135),
(222, 39),
(221, 41),
(52, 44),
(225, 44),
(226, 45),
(226, 46),
(226, 47),
(62, 53),
(244, 53),
(246, 54),
(57, 59),
(232, 59),
(234, 60),
(241, 80),
(237, 81),
(229, 82),
(245, 87),
(220, 88),
(233, 89),
(224, 90),
(531, 92),
(242, 101),
(238, 102),
(230, 103),
(226, 106),
(226, 107),
(246, 108),
(221, 109),
(234, 110),
(249, 114),
(251, 115),
(243, 120),
(239, 121),
(231, 122),
(247, 125),
(223, 126),
(235, 127),
(227, 128),
(528, 135),
(529, 135),
(531, 136),
(212, 137),
(248, 137),
(542, 139),
(540, 140),
(3, 141),
(44, 141),
(45, 142),
(43, 150),
(45, 151),
(46, 156);
INSERT INTO sys_post (post_id, post_name, post_code, sort, status, remark, create_by, update_by, created_at, updated_at, deleted_at)VALUES 
(1, '管理员', 'admin', 0, '2','默认管理岗位', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_role (role_id, role_name, status, role_key, role_sort, flag, remark, admin, data_scope, create_by, update_by, created_at, updated_at, deleted_at)VALUES 
(1, '系统管理员', '2', 'admin', 1, '', '', 1, '1', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
INSERT INTO sys_user VALUES (1, 'admin', '$2a$10$/Glr4g9Svr6O0kvjsRJCXu3f0W8/dsP3XZyVNi1019ratWpSPMyw.', '管理员', '', 1, '', '', '1', '', 1, 1, '', '2', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL);
-- 数据完成 ;
