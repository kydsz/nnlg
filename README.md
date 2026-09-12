# 教学评价系统 V3

一个面向南宁理工学院的教学评价系统，支持移动端和管理端，支持从教务系统自动同步数据。

V3 为全量重构版本：后端使用 **Go + Gin + GORM**，前端使用 **React 18 + TypeScript + Vite**。系统与旧版 V2（FastAPI + Vue3）**共用同一 MySQL 数据库**，接口路径、统一响应格式、JWT、时间格式均与旧端保持兼容，因此管理端与移动端前端可无缝对接任意一端后端。

- 后端（Go）：`backend-go`
- 前端（React）：`frontend-react`
- 部署编排（Nginx 反向代理）：`nginx`

## 技术栈

### 后端（backend-go）
- **框架**: Gin
- **ORM**: GORM（MySQL 驱动）
- **数据库**: MySQL 8.0+
- **缓存/消息队列**: Redis 7+（go-redis v9）— 统计报表缓存 + 评价提交异步落库削峰
- **认证**: JWT（golang-jwt/v5，HS256，与旧端互通）
- **密码加密**: bcrypt（golang.org/x/crypto，与旧端 passlib 兼容）
- **导出**: excelize（XLSX）、gopdf（评教记录 PDF）
- **爬虫**: 教务系统同步（jwxt / llsykb）
- 依赖已 vendor，可离线构建

### 前端（frontend-react）
- **框架**: React 18 + TypeScript
- **UI组件库**: antd 5（管理端）+ antd-mobile 5（移动端）
- **构建工具**: Vite 5
- **状态管理**: Zustand（persist）
- **数据请求**: TanStack Query + axios
- **路由**: React Router v6（`/admin/*` 与 `/mobile/*` 双域）
- **图表/公式**: ECharts、KaTeX

## 项目结构

```
├── backend-go/              # Go 后端
│   ├── cmd/server/          # 入口 main.go
│   ├── internal/
│   │   ├── config/          # 环境变量配置
│   │   ├── database/        # MySQL 连接
│   │   ├── cache/           # Redis 封装（JSON缓存/幂等/Stream生产消费/死信清扫）
│   │   ├── handler/         # 接口处理（auth/user/role/org/dimension/task/
│   │   │                    #   evaluation/export/upload/schedule/stats/sync）
│   │   ├── service/         # 业务逻辑
│   │   ├── middleware/      # 认证 JWT / 权限码 / CORS
│   │   ├── model/           # GORM 模型（表结构由旧端/SQL迁移管理，禁止 AutoMigrate）
│   │   ├── router/          # 路由注册（与旧 FastAPI 路径一一兼容）
│   │   ├── pdfgen/          # 评教记录 PDF 导出（gopdf）
│   │   └── jwxt/            # 教务系统爬虫与解析
│   └── pkg/                 # jwtutil / pwd / response 公共库
│
└── frontend-react/          # React 前端
    └── src/
        ├── api/             # axios 封装 + 按模块拆分的接口
        ├── stores/          # Zustand 全局状态（auth 等）
        ├── routes/          # 路由表 + 守卫
        ├── components/      # ScheduleGrid / FormulaEditor / ExportFieldSelector 等
        ├── views/
        │   ├── admin/       # 管理端（Dashboard/Users/Roles/Dimensions/Tasks/
        │   │                #   Evaluations/Stats/DataSync/CourseSchedule ...）
        │   └── mobile/      # 移动端（Home/Schedule/Evaluation/Evaluated/Profile）
        └── utils/ hooks/    # 工具与自定义 hooks
```

## 功能特性

### 组织架构管理
- 校区管理 / 学院管理 / 教研室管理

### 用户与角色（RBAC）
- 系统管理员 / 学院管理员 / 校级督导 / 院级督导 / 督导老师 / 教师
- 用户可拥有多角色，支持动态角色管理与自定义权限码
- 督导负责范围（学院 / 教研室）配置
- 会话安全：**会话 epoch 即时撤销**（改密/禁用/删除即踢下线）、登录接口限流、附件/PDF 图片安全下载

### 评教维度配置
- 支持动态维度配置与分组管理，批量排序
- 字段类型：评分、单选、多选、文本、数字、日期、日期时间、富文本、图片、文件

### 评教任务管理
- 创建 / 批量创建 / 编辑 / 取消 / 删除评教任务
- 查看评教状态（已评次数、评教记录汇总、督导已评、当前用户是否已评）

### 评教功能
- 移动端评教，支持匿名评教、富文本（LaTeX 公式）
- 文件类维度附件上传（图片/文档），单条记录导出（可打印 HTML / PDF）
- 评教可见性控制：自己评的、被评教师、系统管理员，或按权限码 `evaluation:view_all` 按学院范围查看
- **高并发支持**（Redis Stream 异步削峰）：集中评教期间大批教职工同时提交时，提交先入 Redis 队列秒回成功，后台消费者批量落库，避免 MySQL 行锁竞争打满连接池
- **死信保护**（Redis Stream 消费者组）：单条消息**投递超过阈值**或**在 Pending 停留超 10 分钟**仍未成功落库，自动挪入独立死信流 `<stream>:dead` 并记告警日志，避免无限重试积压且不丢数据（死信流同样受 AOF 保护）。首次死信会**自动重放一次**回主队列，重放仍失败才滞留待人工处理；消费者对"确定不可处理的业务拒绝"才确认丢弃，DB 瞬时故障的消息留待重试。管理端可 `POST /api/v1/queue/replay-dead` 一键重放死信回主队列，`GET /api/v1/queue/status` 查看队列健康（需系统管理员 `role:manage`）

### 统计报表
- 教师评教统计、学院/校区评教统计、督导/评教人统计
- 未被听课教师、评教记录合并统计、教师评教汇总
- 支持导出 XLSX（任务、教师、学院、督导、评教记录、教师汇总）
- **统计缓存**：读密集统计接口结果缓存到 Redis（60s TTL），多人同时查看时不再反复全量计算聚合
- 评教数据以当前学期为节点：统计、评教记录、教师评教汇总默认按当前学期区间（开学日 ~ 开学日 + 周数×7 天）汇总，学期变更后数据随之变动

### 课程表
- 学期配置与课表查询：学期配置（开学日期、每学期周数、设为当前学期）与课表查询（列表 / 详情 / 教师课表 / 我的课表）、版本历史
- 从教务系统同步课表

### 数据同步与爬取
- 同步单位信息、教师信息、课程表（单任务 / 批量 / 全量）
- 教务系统课表爬取（同步 / 预览 / 异步 / llsykb）
- 工号异常用户扫描与清理

## 快速开始

### 环境要求
- Go 1.26+
- Node.js 18+
- MySQL 8.0+（与旧端共用，业务表结构由 `v2/backend` 初始化脚本或 SQL 迁移文件创建；增量结构变更由 `backend-go/internal/database/migrations` 下的 SQL 文件在服务启动时自动执行，幂等可重复启动）
- Redis 7+（可选但推荐，高并发缓存/消息队列；**不配置则自动降级为直连 DB，不影响原有功能**）

### 后端启动（Go）

```bash
cd backend-go

# 配置环境变量（复制 .env.example 为 .env 并修改）
cp .env.example .env
# 必须配置：DB_HOST / DB_USER / DB_PASSWORD / DB_NAME / SECRET_KEY（至少16字符）
# 可选配置：REDIS_ADDR=localhost:6379（启用统计缓存与评价异步落库）

# 直接运行
go run ./cmd/server
# 或构建后运行
go build -o server ./cmd/server
./server
```

后端服务将运行在 http://localhost:8000

- 健康检查：`GET /health`
- 接口文档见 [API.md](./API.md)（Go 端无 Swagger UI）
- 前端开发代理 `/api` 到 `http://localhost:80`（docker 部署的 Nginx），本地直连后端时可在 `frontend-react/vite.config.ts` 中改为 8000

> 注意：本机若存在非布尔值的全局 `DEBUG` 环境变量（如 `DEBUG=release`），程序启动时会自动清理，`.env` 中的 `DEBUG` 才会生效；`DEBUG=true` 时 Gin 输出调试日志。

### 前端启动（React）

```bash
cd frontend-react

# 安装依赖
npm install

# 启动开发服务器（http://localhost:3001，/api 代理到 http://localhost:80）
npm run dev

# 生产构建（产物在 dist/）
npm run build
```

### Docker 部署

两个目录均提供 Dockerfile：

```bash
# 后端（Alpine，时区 UTC，暴露 8000）
cd backend-go && docker build -t te-backend-go .

# 前端（Nginx，暴露 80，/api 与 /uploads 反代到后端 backend:8000）
cd frontend-react && docker build -t te-frontend-react .
```

## 默认账号

系统首次运行时由 V2 初始化脚本创建以下账号（初始密码由后端配置 `DEFAULT_USER_PASSWORD` 决定，请在 `.env` 中配置）。该初始密码同样用于新增用户与教务导入账号，服务端会标记 `must_change_password` 并**强制首次登录改密**（未改密前除改密/查看自己/登出外的接口一律 403）：

| 账号 | 角色 | 说明 |
|------|------|------|
| admin | 系统管理员 | 拥有全部权限 |
| S001 | 学院管理员 | 学院数据管理 |
| T001 | 教师 | 测试老师 |
| D001 | 督导老师 | 听课评教 |

## 相关文档

- [API.md](./API.md) - 接口文档（与 Go 后端 router.go 一一对应）
- [RBAC.md](./RBAC.md) - RBAC 权限模型与权限码说明

## 联系方式

如有问题，请联系项目维护者。