# RBAC 权限模型文档

## 概述

本系统采用 **RBAC (Role-Based Access Control)** 基于角色的访问控制模型，支持动态角色管理和自定义权限。

> 适用版本：V3（Go 后端 `v3/backend-go`）。权限码的唯一真相源为 `internal/service/role.go` 中的 `PermissionCatalog`，接口侧由 `internal/middleware` 的 `RequirePermission` 中间件在路由上强制校验（详见 [API.md](./API.md) 的接口总览）。

## 核心特性

### 1. 动态角色管理

系统支持管理员在后台创建、编辑、删除自定义角色，每个角色可以配置：
- **权限列表**：从系统预定义的权限集合中选择
- **数据范围**：全部数据 / 本学院数据 / 仅个人数据
- **优先级**：数字越小权限越高（用于推导主角色）

### 2. 多角色支持

每个用户可以拥有多个角色，角色平等无主次。`role` 字段自动从 `roles` 列表中推导为最高权限角色。

优先级：system_admin > school_admin > college_admin > school_supervisor > supervisor > college_supervisor > teacher

### 3. 督导角色区分

| 角色 | 编码 | 数据范围 | 说明 |
|------|------|---------|------|
| 校级督导 | school_supervisor | 全部学院 | 可跨学院督导评教，查看所有学院数据 |
| 院级督导 | college_supervisor | 本学院 | 仅负责本学院的督导评教工作 |
| 督导老师 | supervisor | 本学院 | 通用督导角色（向后兼容） |

## 内置角色

| 角色 | 编码 | 优先级 | 数据范围 | 说明 |
|------|------|--------|---------|------|
| 系统管理员 | system_admin | 1 | all | 拥有系统所有权限 |
| 学院管理员 | college_admin | 10 | college | 管理本学院的用户、评教任务等 |
| 学院管理员 | school_admin | -（向后兼容别名，与 college_admin 同义） | college | 兼容旧数据的角色编码 |
| 校级督导 | school_supervisor | 20 | all | 可跨学院督导评教 |
| 督导老师 | supervisor | 22 | college | 通用督导（向后兼容） |
| 院级督导 | college_supervisor | 25 | college | 仅负责本学院督导 |
| 教师 | teacher | 50 | self | 查看自己的评教记录和课表 |

> 说明：`school_admin` 是兼容旧数据的角色编码，显示名同为"学院管理员"，与 `college_admin` 等价处理；具体优先级以 `migration_rbac_roles.sql` 初始化数据为准。

## 权限列表

### 用户管理
- user:view / user:create / user:update / user:delete

### 组织架构
- org:view / campus:manage / college:manage / research_room:manage

### 角色管理
- role:manage

### 评教维度
- dimension:manage

### 评教任务
- task:view / task:create / task:update / task:delete

### 评教记录
- evaluation:view / evaluation:create / evaluation:view_anonymous / evaluation:view_all

> `evaluation:view_all`：查看他人评教记录（含匿名详情）的权限码，由系统管理员在角色管理中按需分配；分配后督导等角色可按其学院数据范围查看他人评教记录/详情/导出。

### 统计报表
- stats:view

### 课程表
- schedule:view

### 数据同步
- sync:execute

## 数据模型

### Role（角色表）
- name: 角色名称
- code: 角色编码（唯一）
- description: 角色描述
- level: 优先级（数字越小越高）
- permissions: JSON 权限列表
- data_scope: 数据范围 all/college/self
- is_system: 是否系统内置（不可删除）
- status: 状态 1-启用 0-禁用

### UserRole（用户-角色关联表）
- user_id / role / assign_time

### UserCollege（督导负责学院关联表）
- user_id / college_id / join_time

## API 接口

> 完整接口及各路由所需权限码见 [API.md](./API.md)。此处仅列出与权限相关的核心接口：

### 角色管理（接口需 `role:manage`，列表/详情需 `user:view`）
- GET    /roles                  获取角色列表（非 system_admin 仅见启用角色）
- GET    /roles/permissions      获取权限码目录
- GET    /roles/{id}             获取角色详情
- POST   /roles                  创建角色（role:manage）
- PUT    /roles/{id}             更新角色（role:manage）
- DELETE /roles/{id}             删除角色（role:manage，内置角色不可删）

### 用户管理（接口按 user:view / user:create / user:update / user:delete 校验）
- GET    /users                           用户列表（user:view）
- POST   /users                           创建用户（user:create）
- PUT    /users/{id}                      更新用户（user:update）
- DELETE /users/{id}                      删除用户（user:delete）
- PUT    /users/{id}/status               启用/禁用（user:update）
- POST   /users/batch-status              批量启用/禁用（user:update）
- PUT    /users/me/research-room          教师自助改主教研室（仅登录）
- POST   /users/{id}/roles/{role}         添加角色（user:update）
- DELETE /users/{id}/roles/{role}         移除角色（user:update）
- GET    /users/{id}/supervisor-scope     查询督导负责范围（user:view）
- PUT    /users/{id}/supervisor-scope     覆盖式设置督导负责范围（user:update）

## 前端页面

### 角色管理页面（/admin/roles）
仅系统管理员可见，支持：
- 查看所有角色（含内置标记）
- 新建自定义角色
- 编辑角色权限和数据范围
- 删除自定义角色（有用户关联时不可删）

### 用户管理页面（/admin/users）
角色选择下拉已包含校级督导、院级督导等新增角色。

## 匿名评教查看权限

| 角色 | 可查看匿名评教详情 | 说明 |
|------|-------------------|------|
| system_admin | 是 | 系统管理员可查看所有完整信息 |
| college_admin | 是 | 学院管理员可查看本学院完整信息 |
| school_supervisor | 是 | 校级督导可查看所有学院完整信息 |
| college_supervisor | 否 | 院级督导仅能看到自己评的信息 |
| supervisor | 否 | 督导仅能看到自己评的信息 |
| teacher | 否 | 教师仅能看到评给自己的非匿名信息 |

## 迁移

执行 migration_rbac_roles.sql 创建角色表并初始化内置角色。
