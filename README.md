# Prototype Hub

一个基于 Go 单体架构实现的内部原型托管平台 PoC，支持：

- 账号密码登录与 JWT Cookie 鉴权
- 系统级 RBAC：`admin` / `project_admin` / `viewer`
- 项目、版本、权限和审计日志管理
- 上传 HTML 原型 ZIP，自动校验、解压并托管预览
- 通过受控地址访问原型 HTML 和静态资源
- 存储层支持 `S3/MinIO` 与本地文件系统
- 数据库支持 `PostgreSQL`、`MySQL`、`SQLite`

## 快速启动

1. 复制配置：

```powershell
Copy-Item .env.example .env
```

2. 启动依赖与应用：

```powershell
docker compose up --build
```

如果要切换到 MySQL 示例：

```powershell
Copy-Item .env.example .env
# 把 .env 里的 DB_DRIVER / DB_DSN 改成 MySQL 示例
docker compose --profile mysql up app-mysql mysql minio --build
```

如果使用腾讯云 COS 这类预先建桶的对象存储，建议设置：

```env
S3_AUTO_CREATE_BUCKET=false
```

3. 打开：

- 应用：[http://localhost:8080](http://localhost:8080)
- MinIO Console：[http://localhost:9001](http://localhost:9001)

默认管理员账号来自 `.env`：

- 用户名：`admin`
- 密码：`ChangeMe123!`

## 数据库配置

### PostgreSQL

```env
DB_DRIVER=postgres
DB_DSN=host=postgres user=prototype password=prototype dbname=prototype_hub port=5432 sslmode=disable TimeZone=Asia/Shanghai
```

### MySQL

```env
DB_DRIVER=mysql
DB_DSN=prototype:prototype@tcp(mysql:3306)/prototype_hub?charset=utf8mb4&parseTime=True&loc=Local
```

对应的 Compose 示例服务：

- 应用：`app-mysql`
- 数据库：`mysql`
- 启动命令：`docker compose --profile mysql up app-mysql mysql minio --build`

### SQLite

```env
DB_DRIVER=sqlite
DB_DSN=file:data/prototypehub.db?_foreign_keys=on
```

## 本地存储模式

如果不想依赖 MinIO，可以切到本地文件模式：

```env
STORAGE_DRIVER=local
LOCAL_STORAGE_PATH=data/storage
```

## 核心 API

- `POST /api/auth/login`
- `POST /api/auth/logout`
- `GET /api/me`
- `POST /api/me/password`
- `GET /api/projects`
- `POST /api/projects`
- `GET /api/projects/:id`
- `GET /api/projects/:id/versions`
- `GET /api/projects/:id/versions/:versionID`
- `POST /api/projects/:id/versions/upload`
- `POST /api/projects/:id/current-version`
- `GET /api/admin/users`
- `POST /api/admin/users`
- `PATCH /api/admin/users/:id/status`
- `POST /api/admin/users/:id/roles`
- `GET /api/admin/audit-logs`

## ZIP 约束

- 必须是 HTML 静态原型包
- 入口页默认优先 `index.html`、`首页.html`
- 禁止路径穿越
- 限制压缩包大小、文件数、解压总大小，可通过 `UPLOAD_MAX_*` 环境变量调整
- 拦截明显的可执行脚本类型
