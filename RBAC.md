# RBAC 权限模型文档

## 概述

本系统采用 **RBAC (Role-Based Access Control)** 基于角色的访问控制模型，支持动态角色管理和自定义权限。

> 适用版本：V3（Go 后端 `v3/backend-go`）。权限码的唯一真相源为 `internal/service/role.go` 中的 `PermissionCatalog`，接口侧由 `internal/middleware` 的 `RequirePermission` 中间件在路由上强制校验（详见 [API.md](./API.md) 的接口总览）。

## 核心特性

### 1. 动态角色管理

系统支持管理员在后台创建、编辑、删除自定义角色，每个角色可以配置：

* **权限列表**：从系统预定义的权限集合中选择

* **数据范围**：全部数据 / 本学院数据 / 仅个人数据（`data_scope`：all / college / self）。该字段只约束**个人数据**（评教记录、课表等），不决定**评教任务**的可见范围——任务范围先由 `task:view_all` 决定「是否可见他人的任务」，再由 `AccessibleCollegeIDs` 按学院过滤（见下文说明）

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
| 督导老师  | supervisor          | 22                            | college | 通用督导（向后兼容）；具备 `task:create`，可添加自己及负责学院教师的课程进待评任务（见 `task:create` 说明）                                                          |
| 院级督导  | college\_supervisor | 25                            | college | 仅负责本学院督导                                                                                                                                           |
| 教师    | teacher             | 50                            | self    | `data_scope = self`：仅能查看**自己**的评教记录和课表；**查看评教任务**另有 `task:view_all` 把关，教师默认不持有，只能看到与我相关的任务（我创建的 / 我被评的 / 我评过的，见下方 `task:view_all` 说明）；具备 `task:create` 时可添加**自己及本学院其他教师**的课程进待评任务（见 `task:create` 说明）；具备 `schedule:view_college` 时可查看**本学院**其他教师的课表（见课程表说明）；具备 `evaluation:create`，可提交评教（不能评自己、只能评本学院教师，由后端校验） |

> 说明：`school_admin` 是兼容旧数据的角色编码，显示名同为"学院管理员"，与 `college_admin` 等价处理；具体优先级以 `migration_rbac_roles.sql` 初始化数据为准。

> **学院管理员的数据范围统一为「本学院」（主学院口径）**：评教任务、评教记录、统计报表、教师评教汇总、课表教师范围等一律经 `AccessibleCollegeIDs` 收敛——`college_admin` / `school_admin` **仅本人主学院**（与 `UserManageCollegeIDs` 同口径），不认 `user_college` 里的督导「负责学院」，未绑定学院时为空集（fail-closed），不再沿用旧端「无学院视为全校」的兜底；其余角色（含三类督导）为主学院 + 负责学院，`system_admin` / `school_supervisor` 为全校。

## 权限列表

### 用户管理

* user:view / user:create / user:update / user:delete

> **数据范围（用户管理口径）**：与 `data_scope` 无关，由 `service.UserManageCollegeIDs` 统一给出。系统管理员（`system_admin`）为全校；学院管理员（`college_admin` / `school_admin`）仅限**本人主学院**——归属按用户的 `college_id`（主学院）判定，不认 `user_college` 里的督导「负责学院」；管理类角色**未绑定学院时为空集（fail-closed）**，列表返回空、详情与写操作一律拒绝，不再沿用旧端「无学院视为全校」的兜底。其它角色（含校级督导）不享有用户管理范围。
>
> 该范围同时作用于：列表（`college_id` 筛选只能收窄，与范围取交集）、详情、启用/禁用、批量启停用、新增用户（非系统管理员只能在本学院建号，未指定学院时默认本学院）、编辑/删除/加角色/改学院/改教研室。**同学院管理员账号之间仅系统管理员可互相操作**（防止同级之间改密、禁用、降权），操作自身账号与系统管理员的既有保护不受影响。
>
> **读侧与写侧同权（不可操作即不可读取）**：非系统管理员的用户列表/详情**不返回**其不可操作的管理员账号（`system_admin` / 其他 `college_admin` / `school_admin`，自己除外）——这些账号的写操作被 `checkTargetUser` 拒绝，读接口同样不给信息，堵住「操作不了但还能拿到账号信息」的旁路；系统管理员列表不受限。
>
> **督导负责范围读写同权（仅系统管理员）**：`GET /users/{id}/supervisor-scope` 与 `PUT /users/{id}/supervisor-scope` 同为**仅系统管理员**（`service.SupervisorScope` / `service.UpdateSupervisorScope`），非系统管理员查询返回 403；用户列表/详情里的督导负责范围字段（`colleges` / `research_rooms` / `supervisor_colleges` / `supervisor_research_rooms` 及计数）也只对系统管理员返回。

### 组织架构

* org:view / campus:manage / college:manage / research\_room:manage

> **学院/教研室读写同权（主学院口径）**：学院与教研室的可见、可管理范围一律按「本学院」收敛，不可管理即不可读取。
>
> * 可见范围（`GET /colleges`、`GET /colleges/{id}`、`GET /research-rooms`、`GET /research-rooms/{id}`）：system\_admin 全校；管理类角色（`college_admin` / `school_admin`）**仅本人主学院**（`UserManageCollegeIDs`，不认 `user_college` 里的督导「负责学院」，未绑定学院时为空集 fail-closed）；其余角色（含三类督导、教师自助设置教研室）为主学院 + 督导负责学院。越界详情返回 403，越界列表返回空。
> * 写范围（新建/编辑/删除教研室、编辑/删除学院）：与可见范围同一口径（`checkManageScope`），越界一律拒绝；教研室调整学院时新旧学院都须在范围内。
> * **新建学院仅系统管理员**（校级组织行为，不属于任何人的「本学院」管理范围）；前端 `/admin/colleges` 的「新增学院」按钮仅系统管理员可见。

### 角色管理

* role:manage

### 评教维度

* dimension:manage

### 评教任务

* task:view / task:view\_all / task:create / task:update / task:delete / task:delete\_own / task:restore / task:restore\_own / task:join\_force

> `task:view`：查看评教任务列表/详情/导出（能否进这块功能）。`teacher` / `supervisor` / `college_supervisor` 等内置角色默认分配该权限。历史数据缺失 `task:view` 的角色由迁移 `002_add_task_view_to_builtin_roles.sql` 自动补齐（幂等）。
>
> **`task:view_all`：查看他人评教任务。**未持有该权限者只能看到**与我相关的任务**——我创建的、我被评的、我评过的、我加入的（服务端收敛条件 `relatedToMeCond`；`GET /tasks`、`GET /tasks/:id`、`POST /tasks/export` 同一口径，越权详情返回 403）。持有者再按其学院数据范围查看：管理员=本学院，三类督导=负责学院 / 全校，系统管理员 / 校级督导=全校。该收敛在**服务端查询构造处强制生效**——前端的「我创建的 / 我加入的 / 其他创建的 / 全部任务」只是移动端拼的 `create_by` / `joined_by` / `create_by_not` 参数，去掉也拿不到无关任务。默认授予 `college_admin` / `school_admin` / `school_supervisor` / `college_supervisor` / `supervisor`（迁移 `015_add_task_view_all_to_builtin_roles.sql` 幂等补码），**`teacher` 不授予**——需要放开时由管理员在「角色管理」页勾选「评教任务·查看他人评教任务」，无需改代码。移动端在无该权限时不展示「全部任务 / 其他创建的」两个视图，但「我加入的」恒展示。
>
> **关于** **`data_scope = self`** **与"查看评教任务"的差异**：角色的 `data_scope` 字段只约束**个人数据**的范围，即评教记录、课表这类直接关联到"我"的数据（教师为 `self`，只能看自己的）。**评教任务**不走 `data_scope`，而是两层过滤叠加：`task:view_all` 决定「是否可见他人的任务」，`AccessibleCollegeIDs` + `applyCollegeFilter` 决定「可见哪些学院的任务」。因此教师（无 `task:view_all`）只能看到与自己相关的任务，看不到本学院其他教师的待评计划。

> `task:create`：创建评教任务。接口 `POST /tasks`（含批量 `POST /tasks/batch`）均需该权限。教师默认分配该权限后，可在移动端课表页将课程加入"待评课表"；`POST /tasks/join`（课表「加入待评任务」）另需 `evaluation:create`——加入即占参与名额并应提交评教，避免占名额却无法评教。后端 `checkTaskTargetScope` 教师分支天然限定**只能为本学院教师（含自己）创建任务**，无法跨学院操作。`supervisor`（督导老师）同样分配 `task:create`，可添加自己及负责学院教师的课程进待评任务（督导分支按负责学院限定）。历史角色缺失 `task:create` 的由迁移 `004_add_task_create_to_teacher.sql`（teacher）、`005_add_task_create_to_supervisor.sql`（supervisor）自动补齐（幂等）。

> `task:delete`：删除任意评教任务；`task:delete_own`：仅能删除自己创建的评教任务。两者可并存，系统管理员恒可删任意。删除任务只软删任务本身（数据保留、可恢复），任务名下已提交的评教记录不受影响——记录的存续、可见性与统计口径不随任务删除变化（见 CONTEXT.md「删除评教任务」）。因此删除任务权限不构成"删除评教记录"的旁路。

> `task:restore`：恢复任意已删除的评教任务；`task:restore_own`：仅能恢复自己创建的已删除任务。两者可并存，系统管理员恒可恢复任意。恢复即清掉软删标记（`is_deleted=0`），只还原任务本身——删除从未改动任务聚合值、任务名下评教记录也不随任务删除变化，故恢复无需回补任何聚合数据（见 `internal/service/task.go` 的 `Restore`）。迁移 `016_add_restore_perms_to_builtin_roles.sql` 只为已持有**完整** `task:delete` 的角色镜像下发 `task:restore`（幂等）；`task:restore_own` **不再自动镜像**——恢复入口在管理端（`AdminGate` 之后），持 `task:delete_own` 的非管理端角色（teacher / 督导）进不了该 UI，自动下发只会造出「能删不能恢复」的死权限，这类角色删除后如需恢复统一联系管理员（与移动端删除文案一致），`task:restore_own` 仍保留在权限目录中供管理员手动分配；该迁移并对历史已下发 `task:restore_own` 的非管理端内置角色做幂等移除（自愈）。恢复若与现存「未删除、未取消」的同教师+同课程+同上课时间任务冲突（唯一索引 `uk_task_dedupe_active`），返回可读的 400 提示而非裸数据库错误。此外，任务列表新增 `deleted=1` 查询参数（只看已删除，供恢复入口使用），**仅对持 `task:restore` 或 `task:restore_own` 者生效**；无权限者即便传参也被服务端强制 `is_deleted=0`，看不到已删数据。

> `task:join_force`：满员后超员加入评教任务。任务参与人数上限来自系统配置 `task_max_participants`（默认 4，含创建人），满员后普通评教人不能再加入；持该权限者可通过 `POST /tasks/:id/join` 继续加入，任务显示 `5/4`，等全部参与人评完才算已评。系统管理员恒可；内置 `college_admin` / `school_admin` 由迁移 `020_add_join_force_to_admin_roles.sql` 默认下发，督导与教师不默认授予。

### 评教记录

* evaluation:view / evaluation:create / evaluation:view\_anonymous / evaluation:view\_all / evaluation:update / evaluation:delete / evaluation:delete\_own / evaluation:restore / evaluation:restore\_own

> `evaluation:create`：提交评教。接口 `POST /evaluations`（含 `POST /evaluations/with-files`）均需该权限。教师默认分配该权限后可提交同行评教；业务约束（不能评自己、只能评本学院教师、任务未取消/未重复提交）由后端 `ValidateSubmit` 校验，与路由权限门分离。历史数据缺失 `evaluation:create` 的 teacher 角色由迁移 `008_add_evaluation_create_to_teacher.sql` 自动补齐（幂等）。

> `evaluation:view_all`：查看他人评教记录（含匿名详情）的权限码。内置管理员角色（`college_admin` / `school_admin`）默认持有（迁移 `018_add_evaluation_view_all_to_admin_roles.sql` 幂等补码），按其学院数据范围查看本学院评教记录/详情/导出，与「匿名评教查看权限」表中「学院管理员可查看本学院完整信息」同口径；其余角色由系统管理员在角色管理中按需分配，分配后按其学院数据范围查看他人评教记录/详情/导出。

> `evaluation:update`：修改评教记录（`PUT /evaluations/:id`）。可改维度作答、匿名标记，以及记录所挂课堂的课程信息（被评教师 / 课程名 / 上课时间 / 教室 / 课表快照）——改的是任务本身（记录 `task_id` 不变），该任务下全部记录同步显示新课程信息，任务评教数 / 督导已评 / 状态不变；课程信息另需 `task:update`，且**已有评教提交的任务仅管理员可编辑**（督导持 `task:update` 也不放行）。评教人 / 评教角色 / 提交时间一律不可改。非系统管理员按被评教师主学院收敛数据范围；保存为单事务（记录侧 + 任务侧同时生效或整体回滚），操作日志记录修改前后值。该码替换了此前借用 `evaluation:delete` 的编辑入口；内置角色中仅 `college_admin` / `school_admin` 默认持有（`system_admin` 恒有全部权限），由迁移 `017_add_evaluation_update_to_admin_roles.sql` 幂等下发。

> `evaluation:delete`：作废任意评教记录；`evaluation:delete_own`：仅能作废评教人本人提交的记录。两者可并存，系统管理员恒可作废任意。作废即软删（数据保留、可追溯），记录退出列表与统计口径，同一评教人可对同一任务重新提交；作废后任务上的评教数、「督导已评」与任务状态在同一事务内回退到与有效记录一致。删除评教任务**不**构成作废记录的旁路（见上条 `task:delete`）。

> `evaluation:restore`：恢复任意已作废的评教记录；`evaluation:restore_own`：仅能恢复评教人本人提交的已作废记录。两者可并存，系统管理员恒可恢复任意。恢复即清掉软删标记（`is_deleted=0`），记录重新入列与统计口径；同一事务内按「整表重算」回补所属任务的聚合值（评教数、督导已评，并在有效记录数 > 0 且任务未删且仍为「待评」时推进回「已评」，取消状态不复活），幂等自愈（见 `internal/service/evaluation.go` 的 `Restore` 与 `syncTaskAggregatesAfterRestore`）。迁移 `016_add_restore_perms_to_builtin_roles.sql` 只为已持有**完整** `evaluation:delete` 的角色镜像下发 `evaluation:restore`；`evaluation:restore_own` 同理不再自动镜像（恢复入口 `Evaluations.tsx` 在管理端），保留在目录中供手动分配，并对历史残留做幂等移除。评教记录列表新增 `deleted=1` 查询参数（只看已作废，供恢复入口），**仅对持 `evaluation:restore` 或 `evaluation:restore_own` 者生效**，无权限者传参被强制忽略；列表项返回 `is_deleted` 与 `can_restore`（服务端按权限与归属计算，供前端决定是否渲染恢复按钮）。恢复若与该评教人在同任务下的现存有效记录冲突（唯一索引 `uk_task_evaluator_active`），返回可读的 400 提示。

### 统计报表

* stats:view

* stats:view\_supervisors

* stats:view\_campus

> `stats:view_supervisors`：查看/导出「督导统计」（`GET /stats/supervisors`、`GET /stats/export/supervisors`，整组仍需 `stats:view`）。督导统计是跨学院/校级口径——名单含「负责本范围的跨学院督导」（表格里的「学院」列是该督导的**主学院**），评教次数与平均分按其全校评教汇总，因此默认**只给系统管理员**（权限中间件直通，无需迁移补码），内置的 `college_admin` / `school_admin` 不持有；需要放开时由管理员在「角色管理」页勾选「统计报表·查看督导统计」，无需改代码。未持有该码时，前端统计报表页不渲染该页签，服务端对应接口返回 403。
>
> `stats:view_campus`：查看「校区统计」（`GET /stats/campus`、`GET /stats/campuses`，整组仍需 `stats:view`）。校区统计是跨学院的校级口径，超出学院管理员「本学院」的数据范围，因此与督导统计同规则——默认**只给系统管理员**，内置的 `college_admin` / `school_admin` 不持有（学院管理员不需要校区统计）；需要放开时由管理员在「角色管理」页勾选「统计报表·查看校区统计」，无需改代码。未持有该码时前端不渲染「校区统计」页签，服务端接口返回 403。

### 课程表

* schedule:view

* schedule:view\_college

> `schedule:view`：查看课程表。`/course-schedules` 整组接口（含列表/选教师/查教师课表）均需该权限。
>
> `schedule:view_college`：查看**本学院**其他教师的课表（移动端课表页「选择教师」弹层）。分配给 `teacher` 内置角色后，普通教师即可打开选择器查看本院其他教师的课表；教师可见范围由 `TeacherScopeOf` 天然限定为「教师主学院」。历史角色缺失该权限的由迁移 `003_add_schedule_view_college_to_teacher.sql` 自动补齐（幂等）。管理员（`user:view`）/督导（`schedule:view + 督导角色`）无需该权限亦可查看。
>
> **学期配置（校级配置，仅系统管理员可维护）**：`POST/PUT/DELETE /course-schedules/semester-configs` 仅系统管理员可操作（`handler.requireSystemAdminSemesterConfig`，非系统管理员一律 403），其余角色（含学院管理员）只读查询——学院管理员不需要学期配置管理。前端 `/admin/course-schedule` 的学期配置编辑/删除/新增入口仅系统管理员可见，其他角色只展示查询结果。

### 数据同步

* sync:execute

### 系统设置

* setting:manage

> `setting:manage`：管理全局系统配置（当前为单个评教任务最多参与人数 `task_max_participants`，默认 4，含创建人）。`GET/PUT /settings` 均需该权限；默认仅系统管理员持有（权限中间件直通，无迁移补码），需要放开时由系统管理员在「角色管理」页勾选。该配置的数值本身不是权限码——权限只表达「能不能改」，人数上限是配置值，改人数不需要发版。

### 异步队列（系统运维）

* 无独立权限码：队列状态查看 / 死信重放接口由 `role:manage`（系统管理员）保护，非系统管理员不可访问

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

> **读侧收敛**：`GET /roles`、`GET /roles/{id}` 对无 `role:manage` 的调用者（如学院管理员）只返回基本字段（名称/编码/描述/状态/内置/使用人数）与 `assignable` 标记，**不返回 `permissions` / `data_scope` / `level` 等角色配置**——不可管理的配置同样不可读取。`assignable` 与写侧 `checkRoleAssignable`（`service.AssignableRoles`）同一口径：系统管理员全部可分配，`school_admin` 可分配三类督导与教师，`college_admin` 仅教师；前端「新增/编辑用户」的角色下拉据此只展示可分配角色，不可分配的角色既不可选也不出现在下拉里。

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

* GET    /users/{id}/supervisor-scope     查询督导负责范围（user:view + 仅系统管理员）

* PUT    /users/{id}/supervisor-scope     覆盖式设置督导负责范围（user:update）

### 异步队列管理（接口需 `role:manage`，系统管理员）

* GET    /queue/status            查看评价异步队列状态（待处理数 / 死信数）
* POST   /queue/replay-dead       将死信流消息重放回主队列，由消费者重新落库

## 前端页面

### 管理端菜单与页签（按权限码渲染）

管理端导航（`views/admin/Layout.tsx` 的 `MENU`）、概览页功能卡片（复用同一份 `MENU`）以及统计报表页签（`views/admin/Stats.tsx` 的 `statsTabItems`）都按权限码渲染，**不做角色硬编码**。判定统一走 `stores/auth.ts` 的 `hasPermission`：显式 `permissions` 清单（`/auth/login`、`/auth/refresh`、`/auth/me` 均返回）非空时以清单为准，清单里没有的权限码一律不放行；只有在清单缺失/为空（旧版本 localStorage 会话、角色未配置权限）时才退回按管理员角色放行。因此角色权限调整后刷新页面即生效（启动时会调 `/auth/me` 刷新当前用户），无需改代码。

隐藏菜单/页签只是展示层收敛：**服务端仍按权限码与数据范围强制校验**，直接请求接口会返回 403（`middleware/permission.go`）。

### 角色管理页面（/admin/roles）

仅系统管理员可见，支持：

* 查看所有角色（含内置标记）

* 新建自定义角色

* 编辑角色权限和数据范围

* 删除自定义角色（有用户关联时不可删）

权限勾选项由 `GET /roles/permissions` 动态渲染（分组来自 `internal/service/role.go` 的 `PermissionCatalog`），因此在角色目录中新增权限码后，管理员**无需等待前端发版**即可在页面上勾选分配（`task:view_all` 即按此方式下发）。

### 用户管理页面（/admin/users）

角色选择下拉已包含校级督导、院级督导等新增角色。

页面数据范围由服务端按调用者收敛（见上文「用户管理」），前端不再依赖「学院」下拉来限定范围：学院管理员的 `GET /colleges` 只返回本学院，即便手工改请求参数也拿不到院外用户。

「督导范围」按钮仅系统管理员可见（服务端读写同权，非系统管理员查询 403）；「新增/编辑用户」的角色下拉按 `GET /roles` 返回的 `assignable` 只展示当前账号可分配的角色。

### 移动端首页（/mobile）

任务视图切换「我创建的 / 其他创建的 / 全部任务」在持有 `task:view_all` 时才展示；未持有时只显示「我创建的」，空态文案改为引导联系管理员分配权限。

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

执行 migration\_rbac\_roles.sql 创建角色表并初始化内置角色。此后每次需要的内置角色默认权限调整，都以 `internal/database/migrations/NNN_*.sql` 增量补码，服务启动时经 `//go:embed` + `Migrate()` 自动执行（幂等，可重复启动）：

* `002_add_task_view_to_builtin_roles.sql`：teacher / supervisor / college\_supervisor 补 `task:view`
* `003_add_schedule_view_college_to_teacher.sql`：teacher 补 `schedule:view_college`
* `004_add_task_create_to_teacher.sql` / `005_add_task_create_to_supervisor.sql`：补 `task:create`
* `008_add_evaluation_create_to_teacher.sql`：teacher 补 `evaluation:create`
* `015_add_task_view_all_to_builtin_roles.sql`：college\_admin / school\_admin / school\_supervisor / college\_supervisor / supervisor 补 `task:view_all`（teacher 不补）
* `016_add_restore_perms_to_builtin_roles.sql`：为已持有**完整** delete 码的内置角色镜像下发对应 restore 码——持 `task:delete`→补 `task:restore`，持 `evaluation:delete`→补 `evaluation:restore`（幂等）。`*_own` 恢复码**不再自动镜像**：恢复入口在管理端（`AdminGate` 之后），持 `*_own` 删除码的非管理端角色（teacher / 督导）用不到，自动下发只会产生死权限；这两个码保留在目录中供管理员手动分配。迁移同时对历史已下发 `task:restore_own` / `evaluation:restore_own` 的非管理端内置角色做幂等移除（自愈）
* `017_add_evaluation_update_to_admin_roles.sql`：给内置管理员角色（`college_admin` / `school_admin`；`system_admin` 恒有全部权限）补 `evaluation:update`（幂等）
* `018_add_evaluation_view_all_to_admin_roles.sql`：给内置管理员角色（`college_admin` / `school_admin`）补 `evaluation:view_all`（幂等）——否则学院管理员在评教记录页落入「仅自己评的 + 评给自己的」分支，看不到本学院任何记录，其持有的 `evaluation:view_anonymous` / `evaluation:update` 也无从生效；三类督导**不**默认下发（评教记录含匿名身份，督导需要时由系统管理员在角色管理页勾选）
* `019_task_participant_and_settings.sql`：新建任务参与人表 `evaluation_task_participant` 与系统配置表 `system_setting`，初始化 `task_max_participants = 4`，并按历史评教记录回填参与人（含已作废记录，不包含未评教的历史创建人）
* `020_add_join_force_to_admin_roles.sql`：给内置管理员角色（`college_admin` / `school_admin`）补 `task:join_force`（幂等）；`system_admin` 恒有全部权限

> `stats:view_supervisors`（统计报表·查看督导统计）与 `stats:view_campus`（统计报表·查看校区统计）**没有配套迁移**：系统管理员在权限中间件与 `/auth/me` 处恒有全部权限码，其它内置角色按产品口径**不默认授予**（学院管理员不需要这两类校级口径统计），新增迁移只会产生没人用的死权限。需要时由管理员在角色管理页勾选。
