# RBAC 权限模型文档

## 概述

本系统采用 **RBAC (Role-Based Access Control)** 基于角色的访问控制模型，支持动态角色管理和自定义权限。

> 适用版本：V3（Go 后端 `v3/backend-go`）。权限码的唯一真相源为 `internal/service/role.go` 中的 `PermissionCatalog`，接口侧由 `internal/middleware` 的 `RequirePermission` 中间件在路由上强制校验（详见 [API.md](./API.md) 的接口总览）。

## 核心特性

### 1. 动态角色管理

系统支持管理员在后台创建、编辑、删除自定义角色，每个角色可以配置：

* **权限列表**：从系统预定义的权限集合中选择

* **数据范围**：全部数据 / 本学院数据 / 仅个人数据（`data_scope`：all / college / self）。该字段只约束**个人数据**（评教记录、课表等），不决定**评教任务**的可见范围——任务范围由 `task:view` 按学院过滤（见下文说明）

* **优先级**：数字越小权限越高（用于推导主角色）

### 2. 多角色支持

每个用户可以拥有多个角色，角色平等无主次。`role` 字段自动从 `roles` 列表中推导为最高权限角色。

优先级：system\_admin > school\_admin > college\_admin > school\_supervisor > supervisor > college\_supervisor > teacher

### 3. 督导角色区分

| 角色   | 编码                  | 数据范围 | 说明                |
| ---- | ------------------- | ---- | ----------------- |
| 校级督导 | school\_supervisor  | 全部学院 | 可跨学院督导评教，查看所有学院数据 |
| 院级督导 | college\_supervisor | 本学院  | 仅负责本学院的督导评教工作     |
| 督导老师 | supervisor          | 本学院  | 通用督导角色（向后兼容）      |

## 内置角色

| 角色    | 编码                  | 优先级                           | 数据范围    | 说明                                                                                                                                                 |
| ----- | ------------------- | ----------------------------- | ------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| 系统管理员 | system\_admin       | 1                             | all     | 拥有系统所有权限                                                                                                                                           |
| 学院管理员 | college\_admin      | 10                            | college | 管理本学院的用户、评教任务等                                                                                                                                     |
| 学院管理员 | school\_admin       | -（向后兼容别名，与 college\_admin 同义） | college | 兼容旧数据的角色编码                                                                                                                                         |
| 校级督导  | school\_supervisor  | 20                            | all     | 可跨学院督导评教                                                                                                                                           |
| 督导老师  | supervisor          | 22                            | college | 通用督导（向后兼容）                                                                                                                                         |
| 院级督导  | college\_supervisor | 25                            | college | 仅负责本学院督导                                                                                                                                           |
| 教师    | teacher             | 50                            | self    | `data_scope = self`：仅能查看**自己**的评教记录和课表；**查看评教任务**不受此限制，教师可查看本学院全体教师的评教任务（见下方 `task:view` 说明）；具备 `task:create` 时可添加**自己及本学院其他教师**的课程进待评任务（见 `task:create` 说明）；具备 `schedule:view_college` 时可查看**本学院**其他教师的课表（见课程表说明） |

> 说明：`school_admin` 是兼容旧数据的角色编码，显示名同为"学院管理员"，与 `college_admin` 等价处理；具体优先级以 `migration_rbac_roles.sql` 初始化数据为准。

## 权限列表

### 用户管理

* user:view / user:create / user:update / user:delete

### 组织架构

* org:view / campus:manage / college:manage / research\_room:manage

### 角色管理

* role:manage

### 评教维度

* dimension:manage

### 评教任务

* task:view / task:create / task:update / task:delete / task:delete\_own

> `task:view`：查看评教任务列表/详情/导出。`teacher` / `supervisor` / `college_supervisor` 等内置角色默认分配该权限；教师按「被评教师所属学院」查看本学院全体教师的评教任务，督导按「负责学院 / 教研室」查看，管理员按「学院」查看，系统管理员 / 校级督导为全校。历史数据缺失 `task:view` 的角色由迁移 `002_add_task_view_to_builtin_roles.sql` 自动补齐（幂等）。
>
> **关于** **`data_scope = self`** **与"查看评教任务"的差异**：角色的 `data_scope` 字段只约束**个人数据**的范围，即评教记录、课表这类直接关联到"我"的数据（教师为 `self`，只能看自己的）。而**评教任务**的查看范围由 `task:view` 的数据过滤逻辑决定——它不直接用 `data_scope`，而是通过 `AccessibleCollegeIDs` 按「被评教师所属学院」过滤。因此教师即使 `data_scope = self`，查看评教任务时仍能看到**本学院全体教师**的任务。这两者并不冲突：`data_scope = self` 限制的是教师"查看自己评教记录/课表"，`task:view` 则允许教师"查看本学院全体教师的评教任务"。其余角色同理，任务数据范围始终由学院过滤逻辑（`AccessibleCollegeIDs` + `applyCollegeFilter`）而非 `data_scope` 决定。

> `task:create`：创建评教任务。接口 `POST /tasks`（含批量 `POST /tasks/batch`）均需该权限。教师默认分配该权限后，可在移动端课表页将课程加入"待评课表"；后端 `checkTaskTargetScope` 教师分支天然限定**只能为本学院教师（含自己）创建任务**，无法跨学院操作。历史角色缺失 `task:create` 的由迁移 `004_add_task_create_to_teacher.sql` 自动补齐（幂等）。

> `task:delete`：删除任意评教任务；`task:delete_own`：仅能删除自己创建的评教任务。两者可并存，系统管理员恒可删任意。删除任务时其名下评教记录一并软删（数据保留、可恢复），确保任务删除后记录不再残留于记录列表/统计。

### 评教记录

* evaluation:view / evaluation:create / evaluation:view\_anonymous / evaluation:view\_all / evaluation:delete / evaluation:delete\_own

> `evaluation:view_all`：查看他人评教记录（含匿名详情）的权限码，由系统管理员在角色管理中按需分配；分配后督导等角色可按其学院数据范围查看他人评教记录/详情/导出。

> `evaluation:delete`：删除任意评教记录；`evaluation:delete_own`：仅能删除评教人本人提交的记录。两者可并存，系统管理员恒可删任意。

### 统计报表

* stats:view

### 课程表

* schedule:view

* schedule:view\_college

> `schedule:view`：查看课程表。`/course-schedules` 整组接口（含列表/选教师/查教师课表）均需该权限。
>
> `schedule:view_college`：查看**本学院**其他教师的课表（移动端课表页「选择教师」弹层）。分配给 `teacher` 内置角色后，普通教师即可打开选择器查看本院其他教师的课表；教师可见范围由 `TeacherScopeOf` 天然限定为「教师主学院」。历史角色缺失该权限的由迁移 `003_add_schedule_view_college_to_teacher.sql` 自动补齐（幂等）。管理员（`user:view`）/督导（`schedule:view + 督导角色`）无需该权限亦可查看。

### 数据同步

* sync:execute

## 数据模型

### Role（角色表）

* name: 角色名称

* code: 角色编码（唯一）

* description: 角色描述

* level: 优先级（数字越小越高）

* permissions: JSON 权限列表

* data\_scope: 数据范围 all/college/self

* is\_system: 是否系统内置（不可删除）

* status: 状态 1-启用 0-禁用

### UserRole（用户-角色关联表）

* user\_id / role / assign\_time

### UserCollege（督导负责学院关联表）

* user\_id / college\_id / join\_time

## API 接口

> 完整接口及各路由所需权限码见 [API.md](./API.md)。此处仅列出与权限相关的核心接口：

### 角色管理（接口需 `role:manage`，列表/详情需 `user:view`）

* GET    /roles                  获取角色列表（非 system\_admin 仅见启用角色）

* GET    /roles/permissions      获取权限码目录

* GET    /roles/{id}             获取角色详情

* POST   /roles                  创建角色（role:manage）

* PUT    /roles/{id}             更新角色（role:manage）

* DELETE /roles/{id}             删除角色（role:manage，内置角色不可删）

### 用户管理（接口按 user:view / user:create / user:update / user:delete 校验）

* GET    /users                           用户列表（user:view）

* POST   /users                           创建用户（user:create）

* PUT    /users/{id}                      更新用户（user:update）

* DELETE /users/{id}                      删除用户（user:delete）

* PUT    /users/{id}/status               启用/禁用（user:update）

* POST   /users/batch-status              批量启用/禁用（user:update）

* PUT    /users/me/research-room          教师自助改主教研室（仅登录）

* POST   /users/{id}/roles/{role}         添加角色（user:update）

* DELETE /users/{id}/roles/{role}         移除角色（user:update）

* GET    /users/{id}/supervisor-scope     查询督导负责范围（user:view）

* PUT    /users/{id}/supervisor-scope     覆盖式设置督导负责范围（user:update）

## 前端页面

### 角色管理页面（/admin/roles）

仅系统管理员可见，支持：

* 查看所有角色（含内置标记）

* 新建自定义角色

* 编辑角色权限和数据范围

* 删除自定义角色（有用户关联时不可删）

### 用户管理页面（/admin/users）

角色选择下拉已包含校级督导、院级督导等新增角色。

## 匿名评教查看权限

| 角色                  | 可查看匿名评教详情 | 说明               |
| ------------------- | --------- | ---------------- |
| system\_admin       | 是         | 系统管理员可查看所有完整信息   |
| college\_admin      | 是         | 学院管理员可查看本学院完整信息  |
| school\_supervisor  | 是         | 校级督导可查看所有学院完整信息  |
| college\_supervisor | 否         | 院级督导仅能看到自己评的信息   |
| supervisor          | 否         | 督导仅能看到自己评的信息     |
| teacher             | 否         | 教师仅能看到评给自己的非匿名信息 |

## 迁移

执行 migration\_rbac\_roles.sql 创建角色表并初始化内置角色。
