# 教学评价系统 V3 - API接口文档

本文档与 Go 后端 `v3/backend-go/internal/router/router.go` 中的路由逐条对应（旧 FastAPI 端路径已兼容，前端共用）。

## 基础信息

* **Base URL**: `http://localhost:8000/api/v1`

* **认证方式**: JWT Token（`Authorization: Bearer {token}`）或 HttpOnly Cookie（`token`，path=`/api/v1`），Cookie 优先

* **Content-Type**: `application/json`（上传接口为 `multipart/form-data`）

* **通用响应格式**:

  ```json
  { "code": 200, "message": "success", "data": {} }
  ```

* **分页结构**（`data.list` 数组）:

  ```json
  { "list": [...], "total": 100, "page": 1, "page_size": 20 }
  ```

* **时间格式**: ISO8601 `2006-01-02T15:04:05`（微秒非零时带 6 位小数，空值为 `null`）

* **权限控制**: 除登录等公开接口外均需登录；管理类接口按 [RBAC 权限码](./RBAC.md) 校验（见下）或角色校验

## 权限码速查

| 分组   | 权限码                                                                                                                                           |
| ---- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| 用户管理 | `user:view` / `user:create` / `user:update` / `user:delete`                                                                                   |
| 组织架构 | `org:view` / `campus:manage` / `college:manage` / `research_room:manage`                                                                      |
| 角色管理 | `role:manage`                                                                                                                                 |
| 评教维度 | `dimension:manage`                                                                                                                            |
| 评教任务 | `task:view` / `task:create` / `task:update` / `task:delete` / `task:delete_own`                                                               |
| 评教记录 | `evaluation:view` / `evaluation:create` / `evaluation:view_anonymous` / `evaluation:view_all` / `evaluation:delete` / `evaluation:delete_own` |
| 统计报表 | `stats:view`                                                                                                                                  |
| 课程表  | `schedule:view` / `schedule:view_college`                                                                                                    |
| 数据同步 | `sync:execute`                                                                                                                                |

## 接口总览

> 权限列：`-` = 仅需登录；权限码见上表。`/course-schedules` 整组需 `schedule:view`，`/stats` 整组需 `stats:view`。

| 方法     | 路径                                                | 权限                                                   |
| ------ | ------------------------------------------------- | ---------------------------------------------------- |
| POST   | /auth/login                                       | 公开                                                   |
| POST   | /auth/logout                                      | -                                                    |
| GET    | /auth/me                                          | -                                                    |
| POST   | /auth/password                                    | -                                                    |
| GET    | /users                                            | user:view                                            |
| GET    | /users/:id                                        | user:view                                            |
| POST   | /users                                            | user:create                                          |
| PUT    | /users/:id                                        | user:update                                          |
| DELETE | /users/:id                                        | user:delete                                          |
| PUT    | /users/:id/status                                 | user:update                                          |
| POST   | /users/:id/roles/:role                            | user:update                                          |
| DELETE | /users/:id/roles/:role                            | user:update                                          |
| POST   | /users/:id/colleges/:college\_id                  | user:update                                          |
| DELETE | /users/:id/colleges/:college\_id                  | user:update                                          |
| POST   | /users/:id/research-rooms/:room\_id               | user:update                                          |
| DELETE | /users/:id/research-rooms/:room\_id               | user:update                                          |
| GET    | /users/:id/supervisor-scope                       | user:view                                            |
| PUT    | /users/:id/supervisor-scope                       | user:update                                          |
| POST   | /users/batch-status                               | user:update                                          |
| PUT    | /users/me/research-room                           | -                                                    |
| GET    | /roles/permissions                                | role:manage                                          |
| GET    | /roles                                            | user:view                                            |
| GET    | /roles/:id                                        | user:view                                            |
| POST   | /roles                                            | role:manage                                          |
| PUT    | /roles/:id                                        | role:manage                                          |
| DELETE | /roles/:id                                        | role:manage                                          |
| GET    | /campuses                                         | org:view                                             |
| POST   | /campuses                                         | campus:manage                                        |
| GET    | /campuses/:id                                     | org:view                                             |
| PUT    | /campuses/:id                                     | campus:manage                                        |
| DELETE | /campuses/:id                                     | campus:manage                                        |
| GET    | /colleges                                         | org:view                                             |
| POST   | /colleges                                         | college:manage                                       |
| GET    | /colleges/:id                                     | org:view                                             |
| PUT    | /colleges/:id                                     | college:manage                                       |
| DELETE | /colleges/:id                                     | college:manage                                       |
| POST   | /colleges/sync-from-jwxt                          | role:manage（仅 system\_admin）                         |
| GET    | /research-rooms                                   | org:view                                             |
| POST   | /research-rooms                                   | research\_room:manage                                |
| GET    | /research-rooms/:id                               | org:view                                             |
| PUT    | /research-rooms/:id                               | research\_room:manage                                |
| DELETE | /research-rooms/:id                               | research\_room:manage                                |
| GET    | /dimensions/groups                                | -                                                    |
| POST   | /dimensions/groups                                | dimension:manage                                     |
| PUT    | /dimensions/groups/sort                           | dimension:manage                                     |
| PUT    | /dimensions/groups/:id                            | dimension:manage                                     |
| DELETE | /dimensions/groups/:id                            | dimension:manage                                     |
| GET    | /dimensions                                       | -                                                    |
| GET    | /dimensions/active                                | -                                                    |
| GET    | /dimensions/:id                                   | -                                                    |
| PUT    | /dimensions/sort                                  | dimension:manage                                     |
| POST   | /dimensions                                       | dimension:manage                                     |
| PUT    | /dimensions/:id                                   | dimension:manage                                     |
| DELETE | /dimensions/:id                                   | dimension:manage                                     |
| GET    | /tasks                                            | task:view                                            |
| GET    | /tasks/:id                                        | task:view                                            |
| POST   | /tasks                                            | task:create                                          |
| POST   | /tasks/batch                                      | task:create                                          |
| POST   | /tasks/export                                     | task:view                                            |
| PUT    | /tasks/:id                                        | task:update                                          |
| POST   | /tasks/:id/cancel                                 | task:update                                          |
| DELETE | /tasks/:id                                        | task:delete（任意）或 task:delete\_own（仅自己创建）             |
| GET    | /evaluations                                      | evaluation:view                                      |
| POST   | /evaluations                                      | evaluation:create                                    |
| POST   | /evaluations/with-files                           | evaluation:create                                    |
| GET    | /evaluations/:id                                  | evaluation:view                                      |
| GET    | /evaluations/:id/export                           | evaluation:view                                      |
| DELETE | /evaluations/:id                                  | evaluation:delete（任意）或 evaluation:delete\_own（仅自己提交） |
| POST   | /upload                                           | 学院管理员及以上                                             |
| POST   | /upload/evaluation/:task\_id/:dim\_code           | -                                                    |
| DELETE | /upload/evaluation/:task\_id/:dim\_code/:filename | 管理员或提交者                                              |
| GET    | /upload/\*filepath、/files/\*filepath              | -（附件下载）                                              |
| GET    | /course-schedules/...                             | schedule:view（整组）                                    |
| POST   | /teachers/sync-from-jwxt                          | sync:execute                                         |
| GET    | /teachers/sync-status                             | -                                                    |
| POST   | /course-schedules/crawl                           | sync:execute                                         |
| POST   | /course-schedules/parse-html                      | sync:execute                                         |
| POST   | /course-schedules/sync-from-jwxt                  | sync:execute                                         |
| POST   | /sync/all                                         | sync:execute                                         |
| POST   | /sync/course-schedule                             | sync:execute                                         |
| POST   | /sync/units-and-teachers                          | sync:execute                                         |
| POST   | /sync/llsykb                                      | sync:execute                                         |
| POST   | /sync/llsykb/preview                              | role:manage                                          |
| POST   | /sync/llsykb/batch                                | sync:execute                                         |
| GET    | /sync/llsykb/progress/:taskId                     | sync:execute                                         |
| GET    | /sync/scan-invalid-users                          | sync:execute                                         |
| DELETE | /sync/cleanup-user/:userId                        | sync:execute                                         |
| POST   | /crawl/timetable（/preview、/async）                 | 管理员                                                  |
| POST   | /crawl/llsykb                                     | 管理员                                                  |
| GET    | /stats/...                                        | stats:view（整组）                                       |

***

## 认证接口 (`/api/v1/auth`)

### 用户登录

```http
POST /auth/login
```

**请求体**:

| 参数       | 类型     | 必填 | 说明 |
| -------- | ------ | -- | -- |
| user\_no | string | 是  | 工号 |
| password | string | 是  | 密码 |

登录成功后将 JWT 写入 HttpOnly Cookie（`token`，path=`/api/v1`，SameSite=Lax，浏览器自动携带），响应体不含 access\_token：

**响应示例**:

```json
{
  "code": 200,
  "message": "登录成功",
  "data": {
    "token_type": "bearer",
    "user": {
      "id": 1,
      "user_no": "admin",
      "username": "系统管理员",
      "role": "system_admin",
      "roles": ["system_admin"],
      "permissions": ["user:view", "user:create", "user:update", "user:delete", "org:view", "role:manage", "stats:view", "schedule:view", "sync:execute"],
      "college_id": null,
      "college_name": null,
      "research_room_id": null,
      "research_room_name": null,
      "status": 1,
      "must_change_password": false,
      "supervisor_colleges": [],
      "last_login_time": null,
      "create_time": "2024-01-01T00:00:00",
      "update_time": "2024-01-01T00:00:00"
    }
  }
}
```

### 退出登录

```http
POST /auth/logout
```

清除登录 Cookie 并返回成功。

### 获取当前用户信息

```http
GET /auth/me
Authorization: Bearer {token}
```

**响应**: 同登录响应的 `user` 字段。

### 修改密码

```http
POST /auth/password
```

**请求体**:

| 参数            | 类型     | 必填 | 说明  |
| ------------- | ------ | -- | --- |
| old\_password | string | 是  | 旧密码 |
| new\_password | string | 是  | 新密码 |

**响应示例**:

```json
{ "code": 200, "message": "密码修改成功", "data": null }
```

***

## 用户管理接口 (`/api/v1/users`)

### 获取用户列表

```http
GET /users?page=1&page_size=20&keyword=张&role=teacher&college_id=1,2&research_room_id=1&status=1&no_college=false&no_research_room=false
```

**查询参数**:

| 参数                 | 类型     | 必填 | 说明               |
| ------------------ | ------ | -- | ---------------- |
| page               | int    | 否  | 页码，默认1           |
| page\_size         | int    | 否  | 每页数量，默认20        |
| keyword            | string | 否  | 搜索关键词（工号/姓名）     |
| role               | string | 否  | 角色筛选（用户拥有该角色即匹配） |
| college\_id        | string | 否  | 学院筛选，支持逗号分隔多个 ID |
| research\_room\_id | int    | 否  | 教研室筛选            |
| status             | int    | 否  | 状态筛选（0/1）        |
| no\_college        | bool   | 否  | 只看无学院用户          |
| no\_research\_room | bool   | 否  | 只看无教研室用户         |

**响应示例**:

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "list": [
      {
        "id": 1,
        "user_no": "TD001",
        "username": "张督导",
        "role": "supervisor",
        "role_name": "督导老师",
        "roles": ["supervisor", "teacher"],
        "user_roles": [
          {"role": "supervisor", "role_name": "督导老师", "assign_time": "2024-01-01T00:00:00"},
          {"role": "teacher", "role_name": "教师", "assign_time": "2024-01-01T00:00:00"}
        ],
        "college_id": 1,
        "college_name": "计算机学院",
        "research_room_id": 1,
        "research_room_name": "软件工程教研室",
        "supervisor_college_count": 3,
        "supervisor_research_room_count": 2,
        "colleges": [{"college_id": 1, "college_name": "计算机学院", "college_code": "CS", "join_time": "2024-01-01T00:00:00"}],
        "research_rooms": [],
        "status": 1,
        "last_login_time": "2024-01-15T10:00:00",
        "create_time": "2024-01-01T00:00:00"
      }
    ],
    "total": 10,
    "page": 1,
    "page_size": 20
  }
}
```

### 获取用户详情

```http
GET /users/{user_id}
```

**响应**: 用户详情（含 `supervisor_colleges` / `supervisor_research_rooms` 明细数组，无 `college_name` 等冗余字段）。

### 创建用户

```http
POST /users
```

**请求体**:

| 参数                              | 类型             | 必填 | 说明                                                                          |
| ------------------------------- | -------------- | -- | --------------------------------------------------------------------------- |
| user\_no                        | string         | 是  | 工号（唯一）                                                                      |
| username                        | string         | 是  | 姓名                                                                          |
| password                        | string         | 是  | 密码                                                                          |
| role                            | string         | 否  | 主角色（向后兼容，自动从 roles 推导）：system\_admin, college\_admin, supervisor, teacher 等 |
| roles                           | array\[string] | 否  | 角色列表（多角色无主次，`role` 自动设为最高权限角色）                                              |
| college\_id                     | int            | 否  | 所属学院ID（单选）                                                                  |
| research\_room\_id              | int            | 否  | 所属教研室ID（单选）                                                                 |
| supervisor\_college\_ids        | array\[int]    | 否  | 督导负责的学院ID列表                                                                 |
| supervisor\_research\_room\_ids | array\[int]    | 否  | 督导负责的教研室ID列表                                                                |
| status                          | int            | 否  | 状态，默认1                                                                      |

**权限**: 学院/系统管理员；仅 system\_admin 可设置 system\_admin 角色。

### 更新用户

```http
PUT /users/{user_id}
```

与创建同字段；权限同上。

### 删除用户

```http
DELETE /users/{user_id}
```

**权限**: 学院/系统管理员。

### 启用/禁用用户

```http
PUT /users/{user_id}/status?status=0
```

`status` 必须为 0（禁用）或 1（启用）。

### 批量启用/禁用

```http
POST /users/batch-status
```

**请求体**:

```json
{ "ids": [1, 2, 3], "status": 0 }
```

**响应**:

```json
{ "code": 200, "message": "success", "data": { "success": 2, "failed": 0, "total": 2 } }
```

### 教师自助修改主教研室

```http
PUT /users/me/research-room
```

**请求体**: `{ "research_room_id": 1 }`

**响应**: `{ "research_room_id": 1, "research_room_name": "软件工程教研室" }`

### 用户-角色关联管理

```http
POST /users/{user_id}/roles/{role}      # 添加角色
DELETE /users/{user_id}/roles/{role}    # 移除角色
```

`role` 为角色编码（如 `teacher`）。

### 督导负责范围

```http
GET /users/{user_id}/supervisor-scope   # 查询
PUT /users/{user_id}/supervisor-scope   # 覆盖式更新
```

**更新请求体**:

```json
{ "college_ids": [1, 2, 3], "research_room_ids": [1, 2] }
```

### 督导负责学院/教研室（增量）

```http
POST   /users/{user_id}/colleges/{college_id}          # 加入督导学院
DELETE /users/{user_id}/colleges/{college_id}          # 移出督导学院
POST   /users/{user_id}/research-rooms/{room_id}       # 加入督导教研室
DELETE /users/{user_id}/research-rooms/{room_id}       # 移出督导教研室
```

***

## 角色管理接口 (`/api/v1/roles`)

### 权限码目录

```http
GET /roles/permissions
```

返回按分组的全部权限码（来源 `PermissionCatalog`，见 RBAC.md），供角色编辑界面勾选。

### 获取角色列表

```http
GET /roles?page=1&page_size=100
```

列表项含 `user_count`（角色使用人数）；非 system\_admin 仅能看到启用角色。

### 获取角色详情

```http
GET /roles/{role_id}
```

### 创建/更新角色

```http
POST /roles
PUT /roles/{role_id}
```

**请求体**:

| 参数          | 类型             | 必填 | 说明                        |
| ----------- | -------------- | -- | ------------------------- |
| name        | string         | 是  | 角色名称                      |
| code        | string         | 是  | 角色编码（唯一）                  |
| description | string         | 否  | 描述                        |
| level       | int            | 否  | 优先级（数字越小越高，用于推导主角色）       |
| permissions | array\[string] | 否  | 权限码列表                     |
| data\_scope | string         | 否  | 数据范围：all / college / self |
| status      | int16          | 否  | 状态 1-启用 0-禁用              |

### 删除角色

```http
DELETE /roles/{role_id}
```

系统内置角色（is\_system）不可删除，被引用的角色不可删除。

***

## 组织架构接口

### 校区管理 (`/api/v1/campuses`)

```http
GET    /campuses?page=1&page_size=100&status=1   # 列表（org:view）
POST   /campuses                                 # 新建（campus:manage）
GET    /campuses/{campus_id}                     # 详情（org:view）
PUT    /campuses/{campus_id}                     # 更新（campus:manage）
DELETE /campuses/{campus_id}                     # 删除（campus:manage）
```

校区字段：`name`（必填）、`sort_order`、`status`。列表项含 `college_count`。

**列表响应示例**:

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "list": [{ "id": 1, "name": "主校区", "sort_order": 1, "status": 1, "college_count": 5, "create_time": "2024-01-01T00:00:00" }],
    "total": 1, "page": 1, "page_size": 100
  }
}
```

### 学院管理 (`/api/v1/colleges`)

```http
GET    /colleges?page=1&page_size=100&campus_id=1&status=1   # 列表（org:view）
POST   /colleges                                             # 新建（college:manage）
GET    /colleges/{college_id}                                # 详情（org:view）
PUT    /colleges/{college_id}                                # 更新（college:manage）
DELETE /colleges/{college_id}                                # 删除（college:manage）
POST   /colleges/sync-from-jwxt                              # 从教务系统同步学院/教研室（仅 system_admin）
```

学院字段：`name`、`code`（编码，唯一，必填）、`campus_id`、`sort_order`、`status`。列表项含 `campus_name`、`research_room_count`、`user_count`。

### 教研室管理 (`/api/v1/research-rooms`)

```http
GET    /research-rooms?page=1&page_size=100&college_id=1&status=1   # 列表（org:view）
POST   /research-rooms                                              # 新建（research_room:manage）
GET    /research-rooms/{room_id}                                    # 详情（org:view）
PUT    /research-rooms/{room_id}                                    # 更新（research_room:manage）
DELETE /research-rooms/{room_id}                                    # 删除（research_room:manage）
```

教研室字段：`name`、`code`（编码，必填）、`college_id`（必填）、`status`。

***

## 评教维度接口 (`/api/v1/dimensions`)

查询类接口（GET）仅需登录；写操作需 `dimension:manage`。

### 维度分组

```http
GET    /dimensions/groups?page=1&page_size=100&status=1   # 分组列表
POST   /dimensions/groups                                 # 新建
PUT    /dimensions/groups/sort                            # 批量排序
PUT    /dimensions/groups/{group_id}                      # 更新
DELETE /dimensions/groups/{group_id}                      # 删除
```

分组字段：`code`（必填）、`name`（必填）、`sort_order`、`status`。

**批量排序请求体**（分组与维度通用）:

```json
[ { "id": 1, "sort_order": 1 }, { "id": 2, "sort_order": 2 } ]
```

### 评教维度

```http
GET    /dimensions?page=1&page_size=100&keyword=&group_id=1&field_type=score&status=1   # 维度列表
GET    /dimensions/active                                                                # 启用维度列表（评教表单用）
GET    /dimensions/{dim_id}                                                              # 详情
PUT    /dimensions/sort                                                                  # 批量排序
POST   /dimensions                                                                       # 新建
PUT    /dimensions/{dim_id}                                                              # 更新
DELETE /dimensions/{dim_id}                                                              # 删除
```

**创建/更新请求体**:

| 参数            | 类型     | 必填 | 说明        |
| ------------- | ------ | -- | --------- |
| group\_id     | int    | 否  | 所属分组ID    |
| code          | string | 是  | 维度编码（唯一）  |
| name          | string | 是  | 维度名称      |
| field\_type   | string | 是  | 字段类型（见下表） |
| field\_config | object | 否  | 字段配置      |
| description   | string | 否  | 描述        |
| sort\_order   | int    | 否  | 排序号       |
| is\_required  | bool   | 否  | 是否必填      |
| status        | int16  | 否  | 状态        |

**字段类型说明**:

| 类型               | 说明       | field\_config 配置             |
| ---------------- | -------- | ---------------------------- |
| score            | 评分       | min\_score, max\_score, step |
| single\_choice   | 单选       | options: \[{label, value}]   |
| multiple\_choice | 多选       | options: \[{label, value}]   |
| text             | 文本       | placeholder, max\_length     |
| number           | 数字       | min\_value, max\_value, step |
| date             | 日期       | -                            |
| datetime         | 日期时间     | -                            |
| rich\_text       | 富文本      | placeholder                  |
| image            | 图片（投票附件） | max\_count（默认 9）             |
| file             | 文件附件     | max\_count（默认 5）             |

***

## 评教任务接口 (`/api/v1/tasks`)

### 获取任务列表

```http
GET /tasks?page=1&page_size=20&keyword=高等&status=1&teacher_id=1&college_id=1&has_supervisor_eval=false&create_by=1&create_by_not=2&start_date=2026-08-31&end_date=2027-01-18
```

**查询参数**:

| 参数                    | 类型     | 必填 | 说明                                              |
| --------------------- | ------ | -- | ----------------------------------------------- |
| page                  | int    | 否  | 页码，默认1                                          |
| page\_size            | int    | 否  | 每页数量，默认20                                       |
| keyword               | string | 否  | 关键词搜索（课程名称）                                     |
| status                | int    | 否  | 状态筛选：1待评，2已评，3取消                                |
| teacher\_id           | int    | 否  | 教师筛选                                            |
| college\_id           | string | 否  | 学院筛选（逗号分隔多个）                                    |
| has\_supervisor\_eval | bool   | 否  | 按督导已评筛选                                         |
| create\_by            | int    | 否  | 按创建者筛选                                          |
| create\_by\_not       | int    | 否  | 排除创建者                                           |
| start\_date           | string | 否  | 开始日期 `YYYY-MM-DD`，按上课时间（class\_time）筛选          |
| end\_date             | string | 否  | 结束日期 `YYYY-MM-DD`（含当天），按上课时间筛选；须不早于 start\_date |

**响应示例**:

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "list": [
      {
        "id": 1,
        "teacher_id": 1,
        "teacher_name": "张老师",
        "course_name": "高等数学",
        "class_time": "2024-01-15T08:00:00",
        "classroom": "A101",
        "status": 1,
        "status_name": "待评",
        "start_time": "2024-01-15T08:00:00",
        "end_time": "2024-01-15T09:40:00",
        "evaluation_count": 5,
        "has_supervisor_eval": false,
        "create_by": 1,
        "create_by_name": "张督导",
        "create_time": "2024-01-10T00:00:00",
        "current_user_evaluated": false,
        "evaluation_records": [
          { "evaluator_id": 1, "evaluator_name": "张督导", "evaluator_role": "supervisor", "is_anonymous": false, "submit_time": "2024-01-11T10:00:00", "total_score": 88 }
        ]
      }
    ],
    "total": 3, "page": 1, "page_size": 20
  }
}
```

> 注意：`evaluation_records` 汇总按评教可见性过滤（督导默认看不到其他人的评教记录），`current_user_evaluated` 不受影响。

### 获取任务详情

```http
GET /tasks/{task_id}
```

返回字段在列表基础上增加 `teacher_college_id` / `teacher_college_name`、`update_time`。

### 创建评教任务

```http
POST /tasks
```

**请求体**:

| 参数            | 类型       | 必填 | 说明              |
| ------------- | -------- | -- | --------------- |
| teacher\_id   | int      | 是  | 教师ID            |
| teacher\_name | string   | 否  | 教师姓名（缺省按 ID 查询） |
| course\_name  | string   | 是  | 课程名称            |
| class\_time   | datetime | 否  | 上课时间            |
| classroom     | string   | 否  | 教室（可为 null）     |
| start\_time   | datetime | 否  | 开始时间            |
| end\_time     | datetime | 否  | 结束时间            |

**权限**: task:create。教师默认分配该权限后可将自己及本学院其他教师的课程加入"待评课表"（后端按教师所属学院校验，无法跨学院操作）。

### 批量创建评教任务

```http
POST /tasks/batch
```

**请求体**:

```json
{ "tasks": [ { "teacher_id": 1, "course_name": "高等数学", "class_time": "2024-01-15T08:00:00" } ] }
```

重复任务自动跳过。**响应**: `{ "created": [...], "skipped": [...], "created_count": n, "skipped_count": n }`

### 导出评教任务

```http
POST /tasks/export
```

**请求体**:

```json
{ "status": 1, "college_id": 1, "keyword": "高等", "start_date": "2026-08-31", "end_date": "2027-01-18", "fields": ["task_id", "teacher_name", "course_name", "class_time", "classroom", "college_name", "status_name", "evaluation_count", "has_supervisor_eval", "create_time"] }
```

`start_date` / `end_date` 与列表接口一致，按上课时间筛选；格式 `YYYY-MM-DD`。返回 XLSX 附件（`fields` 为空时使用默认列）。

### 更新/取消/删除

```http
PUT    /tasks/{task_id}          # 更新（task:update，仅创建者或管理员）
POST   /tasks/{task_id}/cancel   # 取消（task:update，返回 cancelled_records_count 级联取消记录数）
DELETE /tasks/{task_id}          # 删除（task:delete 删任意；task:delete_own 仅删自己创建，软删除）
```

***

## 评教记录接口 (`/api/v1/evaluations`)

### 获取评教记录列表

```http
GET /evaluations?page=1&page_size=20&task_id=1&teacher_id=1&evaluator_id=1&evaluator_name=张&evaluator_role=supervisor&college_id=1&teacher_name=&type=&keyword=&start_date=&end_date=
```

**查询参数**: `task_id`、`teacher_id`、`evaluator_id`、`evaluator_name`、`evaluator_role`、`college_id`、`teacher_name`、`type`、`keyword`、`start_date`、`end_date`、`page`、`page_size`（`start_date` / `end_date` 格式 `YYYY-MM-DD`，按提交时间过滤）。

**可见性**: 教师角色只能查看自己的评教记录；督导默认只能查看自己评教的记录，需 `evaluation:view_all` 权限码（在角色管理中分配）后按学院范围查看他人记录。

### 提交评教

```http
POST /evaluations
```

**请求体**:

| 参数                | 类型     | 必填 | 说明           |
| ----------------- | ------ | -- | ------------ |
| task\_id          | int    | 是  | 任务ID         |
| dimension\_values | object | 是  | 维度值，键为维度编码   |
| is\_anonymous     | bool   | 否  | 是否匿名，默认false |

**请求示例**:

```json
{
  "task_id": 1,
  "dimension_values": {
    "attitude_responsible": 18,
    "effect_overall": "excellent",
    "suggestions": "教学内容充实，讲解清晰"
  },
  "is_anonymous": false
}
```

**响应**: `{ "id": 1, "task_id": 1, "submit_time": "...", "total_score": 88 }`

**权限**: evaluation:create。后端校验：任务存在且未取消、评教人本人不能评教自己、任务下未重复提交。

### 提交评教（含文件附件，multipart）

```http
POST /evaluations/with-files
```

| 表单字段              | 说明                                                             |
| ----------------- | -------------------------------------------------------------- |
| task\_id          | 必填                                                             |
| dimension\_values | 必填，JSON 字符串                                                    |
| is\_anonymous     | "true" / "false"                                               |
| files             | 文件数组；文件名必须以 `{维度编码}_` 开头，如 `evidence_1.png`，文件按维度编码分组存入对应文件类维度 |

文件类型限制：图片（jpg/jpeg/png/gif/webp，≤10MB，默认最多9个）、文档（pdf/zip/rar/doc/xls/ppt/docx/xlsx/pptx，≤20MB，默认最多5个）。
上传成功后 `dimension_values` 中对应维度值被回写为文件 URL 数组，响应含 `files` 字段。

### 获取评教详情

```http
GET /evaluations/{record_id}
```

匿名评教时教师端评教人显示为"匿名"；按可见性规则校验（404/403）。

### 导出单条记录

```http
GET /evaluations/{record_id}/export?format=pdf
```

* `format=pdf`：返回 PDF 附件（`application/pdf`，文件名 `evaluation_{id}_{课程名}.pdf`，布局对齐旧端紧凑模板）。

* 不传 `format`：返回可打印 HTML（浏览器可直接另存/打印）。

### 删除评教记录

```http
DELETE /evaluations/{record_id}
```

**权限**: 被分配 `evaluation:delete` 权限可删除任意记录；仅有 `evaluation:delete_own` 权限仅能删除评教人本人提交的记录。

***

## 文件上传接口 (`/api/v1/upload`)

### 上传临时文件

```http
POST /upload
```

multipart：`file`（文件）、`file_type`（`image` 或 `file`，默认 `file`）。
**权限**: 学院/系统管理员。
**响应**: `{ "url": "/api/v1/files/evaluations/temp/xxx.png", "filename": "...", "size": 123 }`

### 评教文件上传（按任务+维度）

```http
POST /upload/evaluation/{task_id}/{dim_code}
```

多维度的文件类维度上传，multipart 字段 `files`（数组）。**响应**: `{ "files": [{"url": "...", "filename": "...", "path": "..."}], "count": 1 }`

### 删除评教文件

```http
DELETE /upload/evaluation/{task_id}/{dim_code}/{filename}
```

**权限**: 管理员或该任务评教记录提交者。

### 附件下载

```http
GET /upload/*filepath
GET /files/*filepath
```

按相对路径返回附件（attachment 下载，防路径穿越）。

***

## 课程表接口 (`/api/v1/course-schedules`)

整组路由均需 `schedule:view` 权限码。

```http
GET    /course-schedules/list?page=&page_size=&semester=&teacher_id=&teacher_name=   # 课表列表
GET    /course-schedules/detail/{schedule_id}                                        # 课表详情
GET    /course-schedules/teacher/{teacher_id}?semester=2024-2025-1                   # 教师课表
GET    /course-schedules/teachers?page=&page_size=&keyword=&college_id=&research_room_id=  # 教师列表（课表选教师用）
GET    /course-schedules/my-schedule?semester=2024-2025-1                            # 我的课表（教师）
GET    /course-schedules/versions?semester=2024-2025-1                               # 版本历史
GET    /course-schedules/teacher-status?semester=2024-2025-1                         # 教师课表同步状态（semester 必填）
GET    /course-schedules/semesters                                                   # 学期列表
GET    /course-schedules/stats?semester=2024-2025-1                                  # 学期统计
GET    /course-schedules/semester-configs                                            # 全部学期配置
GET    /course-schedules/semester-configs/current                                    # 当前学期配置
GET    /course-schedules/semester-configs/{semester}                                 # 指定学期配置
POST   /course-schedules/semester-configs                                            # 新建学期配置（org:view）
PUT    /course-schedules/semester-configs/{semester}                                 # 更新/创建学期配置（org:view）
DELETE /course-schedules/semester-configs/{semester}                                 # 删除学期配置（org:view，当前学期不可删）
```

学期配置请求体：`{ "semester": "2024-2025-1", "start_date": "2024-09-01", "weeks": 20, "is_current": false }`（更新时只需 `start_date` / `weeks` / `is_current`）。`weeks` 为每学期周数，范围 1-52，默认 20。

学期结束日期 = 开学日期 + weeks × 7 天，前端各评教统计页面的默认日期区间取该学期期间。

***

## 数据同步接口

以下均需 `sync:execute` 权限码（标注除外）；教务系统凭据默认取自后端配置（JWXT\_USERNAME 等），`/crawl/*` 可请求自带。

```http
POST /teachers/sync-from-jwxt             # 同步教师信息
GET  /teachers/sync-status                # 最近一次教师同步状态（仅登录）
POST /course-schedules/crawl              # 爬取课表并导入（json: semester/college_code/teacher_name/username/password；仅系统/学院管理员）
POST /course-schedules/parse-html         # 解析并导入课表HTML（json: html_content 必填, semester；仅系统/学院管理员）
POST /course-schedules/sync-from-jwxt     # 从教务系统同步课表（json: semester）
POST /sync/all                            # 全量同步：单位 + 教师 + 课表（json: semester）
POST /sync/course-schedule                # 同步课表（同 sync-from-jwxt）
POST /sync/units-and-teachers             # 同步单位 + 教师
POST /sync/llsykb                         # 同步指定教师课表（json: xnxq01id 可选, teacher_ids 必填，至少1人）
POST /sync/llsykb/preview                 # 预览 llsykb 数据，不入库（query: xnxq01id, teacher_ids 可重复；role:manage）
POST /sync/llsykb/batch                   # 批量同步全校/学院教师课表（后台任务；json: xnxq01id, college_id）
GET  /sync/llsykb/progress/{taskId}       # 批量同步进度
GET  /sync/scan-invalid-users             # 扫描工号格式异常用户（返回 records + suggest_delete 名单）
DELETE /sync/cleanup-user/{userId}        # 删除工号异常用户及其关联（仅限异常工号）
```

**批量同步响应**: `{ "task_id": "xxx", "total": 200, "semester": "2024-2025-1", "scope": "all" }`，之后轮询 `/sync/llsykb/progress/{task_id}`。

***

## 数据爬取接口 (`/api/v1/crawl`)

权限：system\_admin / school\_admin / college\_admin（且须有所属学院）。

```http
POST /crawl/timetable?semester=&username=&password=        # 同步爬取课表并导入
POST /crawl/timetable/preview?semester=&username=&password=  # 预览（当前仅校验登录）
POST /crawl/timetable/async?semester=&username=&password=    # 后台任务异步爬取
POST /crawl/llsykb                                          # 单教师 llsykb 爬取并入库
```

`/crawl/llsykb` 请求体（json）：`username`、`password`、`xnxq01id`（必填）、`teacherID`（必填）、可选 `type`/`zc`/`yxx`/`teacherIDmc`/`jg0101mc`/`jszc`。
返回结构特殊：外层 `success`/`message`/`html`/`records`/`record_count`（非标准 code 响应）。

***

## 统计报表接口 (`/api/v1/stats`)

整组路由均需 `stats:view` 权限码。除标注外，`page` 默认 1，`page_size` 默认 10。

未传 `start_date` / `end_date` 时，统计默认按当前学期区间汇总（开学日 \~ 开学日 + weeks×7 天，weeks 为学期配置的每学期周数）。

**统计口径说明**：`/stats` 下所有接口的时间筛选统一按\*\*评教任务的上课时间（`class_time`）\*\*归属时间段，而非评教记录的提交时间（`submit_time`）。原因：`class_time` 位于任务表（`evaluation_task`），任务级指标（总任务/已评/待评）只能基于任务时间；记录级指标（评教记录数、明细）通过 `task_id` 关联任务后同样按上课时间过滤，从而保证各统计页与导出在相同时间段下数据一致。听课明细响应中保留的 `submit_time` 字段仅作展示，不参与统计筛选。

### 当前学期

```http
GET /stats/current-semester
```

**响应**: `data` 为 `{ "semester": "2026-2027-1", "start_date": "2026-08-31", "end_date": "2027-01-18" }`，`end_date = start_date + weeks × 7 天`（weeks 为学期配置的每学期周数）。

### 系统概览

```http
GET /stats/overview?semester=2024-2025-1&start_date=2026-08-31&end_date=2027-01-18
```

`semester` 可选，按学期过滤数据；`start_date` / `end_date` 格式 `YYYY-MM-DD`，按上课时间（`class_time`）筛选。未传日期时默认按当前学期区间汇总（与其他统计接口口径一致）。同时传 `semester` 与日期时以日期区间为准。

**响应示例**:

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "users": { "total": 50, "teachers": 30, "supervisors": 5 },
    "organization": { "campuses": 2, "colleges": 8 },
    "tasks": { "total": 100, "evaluated": 80, "pending": 20, "evaluation_rate": 80.0 },
    "evaluations": { "total": 500 }
  }
}
```

### 教师评教统计

```http
GET /stats/teachers?page=&page_size=&college_ids=&keyword=&evaluator_roles=&start_date=&end_date=
```

`college_ids` 逗号分隔；`evaluator_roles` 逗号分隔角色码；`start_date`/`end_date` 格式 `YYYY-MM-DD`。

**响应示例**:

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "list": [
      {
        "teacher_id": 1, "teacher_name": "张老师", "college_name": "计算机学院",
        "total_tasks": 20, "evaluated_tasks": 15, "pending_tasks": 5,
        "total_evaluations": 45, "average_score": 18.5, "evaluation_rate": 75.0
      }
    ],
    "total": 30, "page": 1, "page_size": 10
  }
}
```

### 学院评教统计

```http
GET /stats/college?college_id=1&semester=2024-2025-1     # 汇总（不传 college_id 则统计全部）
GET /stats/colleges?page=&page_size=&college_ids=&semester=&evaluator_roles=&start_date=&end_date=   # 分页列表
GET /stats/college-teachers/{college_id}                 # 指定学院教师评教详情
```

### 校区评教统计

```http
GET /stats/campus?campus_id=1&start_date=&end_date=
```

不传 `campus_id` 则统计所有校区；不传日期默认按当前学期区间。

### 督导/评教人统计

```http
GET /stats/supervisors?page=&page_size=&college_ids=&keyword=&start_date=&end_date=
GET /stats/evaluators?page=&page_size=&college_ids=&keyword=&evaluator_roles=&start_date=&end_date=
```

**supervisors 响应示例**:

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "list": [
      { "supervisor_id": 1, "supervisor_name": "张督导", "user_no": "D001", "college_name": "计算机学院", "total_evaluations": 25, "average_score": 18.5 }
    ],
    "total": 5, "page": 1, "page_size": 10
  }
}
```

### 未被听课教师

```http
GET /stats/unteached-teachers?page=&page_size=&college_id=&start_date=&end_date=
```

不传 `college_id` 时按当前用户数据范围返回；不传日期默认按当前学期区间，返回该区间内无评教任务的教师。

### 评教记录合并统计

```http
GET /stats/evaluation-records?page=&page_size=&college_ids=&teacher_id=&evaluator_id=&keyword=&evaluator_roles=&start_date=&end_date=
```

### 教师评教汇总

```http
GET /stats/teacher-evaluation-summary?page=&page_size=&college_ids=&campus_id=&research_room_ids=&keyword=&sort_by=&sort_order=desc&evaluator_roles=&has_courses=&start_date=&end_date=
```

`has_courses` 取值 `true`/`false`，按是否教师角色筛选。

### 导出接口

```http
GET  /stats/export/teachers?format=xlsx|pdf&college_ids=&keyword=&evaluator_roles=&start_date=&end_date=&fields=
GET  /stats/export/colleges?format=xlsx|pdf&college_ids=&semester=&evaluator_roles=&start_date=&end_date=
GET  /stats/export/supervisors?format=xlsx|pdf&college_ids=&keyword=&start_date=&end_date=
GET  /stats/export/teacher-evaluation-summary?format=xlsx|pdf&college_ids=&campus_id=&research_room_ids=&keyword=&sort_by=&sort_order=&evaluator_roles=&start_date=&end_date=&fields=
POST /stats/evaluation-records/export     # 评教记录导出
```

* GET 系列 `format` 仅支持 `xlsx` / `pdf`（pdf 为表格类 PDF 附件，对齐旧端 reportlab 模板；依赖容器内中文字体），默认 xlsx；`fields` 重复传参控制导出列，如 `?fields=teacher_name&fields=course_name`。

* `GET /stats/export/teacher-evaluation-summary` 的 `start_date` / `end_date` 为必填（列表页默认携带学期区间）。

* `POST /stats/evaluation-records/export` 请求体：

```json
{
  "college_ids": [1, 2],
  "teacher_id": 1,
  "keyword": "",
  "evaluator_roles": ["supervisor"],
  "start_date": "2024-01-01",
  "end_date": "2024-12-31",
  "fields": ["teacher_name", "course_name", "class_time", "classroom", "evaluator_name", "submit_time"]
}
```

导出均为 XLSX 附件下载。

***

## 角色说明

| 角色编码                | 说明            |
| ------------------- | ------------- |
| system\_admin       | 系统管理员         |
| college\_admin      | 学院管理员         |
| school\_admin       | 学院管理员（向后兼容别名） |
| school\_supervisor  | 校级督导          |
| college\_supervisor | 院级督导          |
| supervisor          | 督导老师（通用）      |
| teacher             | 教师            |

## 错误码说明

| 错误码 | 说明                    |
| --- | --------------------- |
| 200 | 成功                    |
| 400 | 请求参数错误                |
| 401 | 未认证或token过期           |
| 403 | 无权限访问                 |
| 404 | 资源不存在                 |
| 422 | 请求验证错误（如 path 数字解析失败） |
| 500 | 服务器内部错误               |

## 通用错误响应格式

```json
{
  "code": 400,
  "message": "错误信息",
  "data": null
}
```

