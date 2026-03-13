# AGENTS.md - Moss 开发指南

本文档为 AI 代理提供代码开发规范和构建命令参考。

## 项目概述

Moss 是一个基于 Go (Fiber) + Vue 3 的 CMS 系统：
- **后端**: Go 1.23, Fiber, GORM, Zap 日志, Viper 配置
- **前端**: Vue 3, Vite, Pinia, Vue Router, Vue I18n, Naive UI / Arco Design, PWA 支持
- **主题**: Tailwind CSS (主题: germ, dute8)
- **存储**: 支持 SQLite/MySQL/PostgreSQL, Redis 缓存, 对象存储 (阿里云 OSS/腾讯云 COS/AWS S3/Google Cloud)
- **开发工具**: Air (热重载), Task (任务管理)

---

## 构建与测试命令

### 后端 (Go)

```bash
# 进入后端目录
cd main

# 安装依赖
go mod tidy

# 运行后端 (无热重载)
go run ./cmd/web/main.go

# 运行后端 (热重载，需先安装 air)
air

# 运行测试
go test ./...

# 运行单个测试文件
go test -v ./domain/core/service/...

# 运行单个测试函数
go test -v -run TestArticleGet ./domain/core/service/

# 构建后端 (当前平台)
go build -ldflags="-s -w" -trimpath -o ./cmd/web/moss ./cmd/web/main.go

# 代码格式检查
go fmt ./...

# 代码审查
go vet ./...
```

### 前端 (Vue)

```bash
# 进入前端目录
cd admin

# 安装依赖
npm install

# 开发模式 (热重载，端口 3000)
npm run dev

# 构建生产版本
npm run build

# 预览构建结果
npm run preview
```

### 主题开发

```bash
# 进入主题目录 (以 germ 为例)
cd theme/germ

# 安装依赖
npm install

# 构建 CSS
npx tailwindcss -i ./css/style.css -o ./css/dist/style.css --minify

# 更新模板到资源文件 (开发默认模板时使用)
npx tailwindcss -i ./css/style.css -o ../../main/tmp/themes/germ/public/style.css --minify
rm -rf ./main/resources/themes
cp -r ./main/tmp/themes ./main/resources/themes
```

### 使用 Taskfile (推荐)

```bash
# 安装 Task: https://taskfile.dev

# 初始化依赖
task init-admin      # 前端依赖
task init-themes     # 主题依赖

# 启动完整开发环境 (前后端热重载)
task dev

# 启动后端热重载
task dev-backend

# 启动前端开发服务器
task dev-frontend

# 启动后端服务 (无热重载)
task run

# 启动前端开发服务器 (快捷方式)
task admin

# 检查开发环境状态
task status

# 查看开发日志
task logs

# 实时查看 Nginx 日志
task logs-tail

# 重载 Nginx 配置
task restart-nginx

# 重置管理员账户 (10分钟内有效)
task reset-admin

# 构建所有 (前端 + 多平台后端)
task build

# 构建所有 (前端 + 所有平台后端)
task build-all

# 构建前端
task build-admin

# 构建后端 (所有平台)
task build-main
```

### Docker 开发环境

```bash
# 使用 Docker Compose 启动开发环境
docker-compose -f docker-compose.dev.yml up

# 后台运行
docker-compose -f docker-compose.dev.yml up -d

# 查看日志
docker-compose -f docker-compose.dev.yml logs -f
```

---

## 代码风格指南

### Go 后端

#### 命名规范
- **包名**: 简短 lowercase (如 `controller`, `service`, `mapper`)
- **文件命名**: snake_case (如 `article.go`, `common.go`)
- **结构体**: PascalCase (如 `ArticleService`, `ArticlePost`)
- **变量/函数**: camelCase (如 `articleList`, `GetArticle`)
- **常量**: 导出用 PascalCase，非导出用 camelCase
- **接口**: 以 `er` 结尾 (如 `Repository`, `Service`)

#### 导入顺序
```go
import (
    // 标准库
    "encoding/json"
    "fmt"
    "time"

    // 第三方库
    "github.com/gofiber/fiber/v2"
    "gorm.io/gorm"

    // 项目内部包
    "moss/api/web/mapper"
    "moss/domain/core/entity"
)
```

#### 错误处理
- 使用 `errors.New()` 或自定义错误定义
- 控制器层返回错误: `return ctx.JSON(mapper.MessageResult(err))`
- 服务层: 简洁的 `if err != nil { return }` 模式
- 错误消息使用英文 (与 message 包定义保持一致)

#### 日志规范
- 使用 Zap 日志库: `zap.L()`, `zap.S()`
- 使用结构化日志: `zap.String("key", "value")`
- 错误日志使用 `zap.Error(err)`
- 日志文件位于 `main/runtime/log/`

#### 代码组织
```
main/
├── api/web/           # HTTP 层
│   ├── controller/    # 控制器
│   ├── dto/           # 数据传输对象
│   ├── mapper/        # DTO 转换
│   ├── middleware/    # 中间件
│   └── router/        # 路由定义
├── application/       # 应用服务层
│   ├── dto/
│   ├── mapper/
│   └── service/
├── domain/            # 领域层
│   ├── config/        # 配置领域
│   │   ├── aggregate/
│   │   ├── entity/    # 配置实体
│   │   ├── repository/
│   │   └── service/
│   ├── core/          # 核心业务
│   │   ├── aggregate/
│   │   ├── entity/
│   │   ├── event/
│   │   ├── repository/
│   │   ├── service/
│   │   ├── utils/
│   │   └── vo/
│   └── support/       # 支持模块
│       ├── entity/
│       ├── factory/
│       ├── repository/
│       ├── service/
│       └── utils/
├── infrastructure/   # 基础设施层
│   ├── general/       # 通用基础设施
│   │   ├── command/
│   │   ├── conf/
│   │   ├── constant/
│   │   └── message/
│   ├── persistent/    # 持久化
│   │   └── db/
│   ├── support/       # 支持服务
│   └── utils/         # 工具函数
├── plugins/           # 插件
├── resources/         # 静态资源
│   ├── admin/         # 前端构建产物
│   ├── app/           # 应用资源
│   └── themes/        # 主题资源
├── runtime/           # 运行时文件
│   └── log/           # 日志文件
├── tmp/               # 临时文件
├── themes/            # 主题源文件
├── startup/           # 初始化
└── cmd/web/           # 入口
```

#### 其他规范
- 使用 `var Xxx = new(XxxService)` 定义包级单例服务
- 事件驱动: 在服务中定义事件切片，通过 `AddXxxEvents` 方法注册
- 配置通过 Viper 加载，使用 `config.Config` 访问
- 日志使用 Zap: `zap.L()`, `zap.S()`
- 支持多数据库: SQLite (开发), MySQL, PostgreSQL
- 支持多缓存: Redis, BigCache
- 支持多云存储: 阿里云 OSS, 腾讯云 COS, AWS S3, Google Cloud, FTP

---

### Vue 前端

#### 命名规范
- **组件文件**: PascalCase (如 `ArticlePost.vue`, `DataTable.vue`)
- **目录**: kebab-case (如 `views/article/`, `components/data/`)
- **组件引用**: PascalCase
- **props/事件**: kebab-case
- **CSS 类**: kebab-case

#### 导入规范
```javascript
// Vue 组件
import PostRight from "@/views/article/PostRight.vue";
import PostLeft from "@/views/article/PostLeft.vue";

// 路径别名
@/     -> src/
```

#### 目录结构
```
admin/src/
├── api/              # API 请求
├── assets/           # 静态资源
├── components/       # 公共组件
│   ├── admin/        # 管理后台组件
│   ├── app/          # 应用组件
│   ├── data/         # 数据组件
│   ├── dataTable/    # 表格组件
│   ├── storage/      # 存储组件
│   └── utils/        # 工具组件
├── hooks/            # 组合式函数
│   ├── utils.js
│   └── app/          # 应用相关 hooks
├── layout/           # 布局组件
│   ├── base.vue
│   ├── subMenu.vue
│   └── main/
├── locale/           # i18n 翻译
│   ├── index.js
│   └── lang/
├── router/           # 路由配置
│   ├── index.js
│   └── routes.js
├── store/            # Pinia 状态管理
│   └── index.js
├── views/            # 页面组件
│   ├── admin/        # 管理页面
│   ├── article/      # 文章页面
│   ├── category/     # 分类页面
│   ├── config/       # 配置页面
│   ├── dashboard/    # 仪表盘
│   ├── link/         # 链接页面
│   ├── log/          # 日志页面
│   ├── plugin/       # 插件页面
│   ├── store/        # 存储页面
│   └── tag/          # 标签页面
├── App.vue           # 根组件
├── main.js           # 入口
└── style.css         # 全局样式
```

#### 组件规范
- 使用 `<script setup>` 语法
- 组件按功能模块组织在 views 目录下
- 通用组件放在 components 目录
- 使用 Naive UI / Arco Design 组件库
- 使用 Tailwind CSS 进行样式管理
- 支持 PWA (渐进式 Web 应用)

#### 前端配置
- **开发服务器**: Vite, 端口 3000
- **API 代理**: `/admin/api` → `http://127.0.0.1:9008`
- **构建输出**: `main/resources/admin`
- **代码分割**: 自动分割 node_modules
- **CSS 预处理**: 支持 Less

---

## 常用开发模式

### 后端: 添加新 API

1. 在 `api/web/controller/` 创建或编辑控制器
2. 在 `api/web/router/` 注册路由
3. 在 `application/service/` 添加业务逻辑
4. 如需数据库操作，在 `domain/core/repository/` 定义接口

### 后端: 添加新配置项

1. 在 `domain/config/entity/` 创建或编辑配置实体
2. 实现 `ConfigID() string` 方法
3. 在 `domain/config/service/` 添加配置服务逻辑
4. 配置自动加载到 `config.Config`

### 前端: 添加新页面

1. 在 `views/` 创建页面组件
2. 在 `router/routes.js` 配置路由
3. 如需状态管理，在 `store/` 创建 Pinia store
4. 添加国际化翻译到 `locale/lang/`

### 插件开发

插件位于 `main/plugins/` 目录，实现特定接口后自动注册。

#### 现有插件列表

**内容处理插件:**
- `ArticleSanitizer.go` - 文章内容净化 (使用 bluemonday)
- `DetectLinks.go` - 链接检测
- `GenerateDescription.go` - 自动生成文章描述
- `GenerateSlug.go` - 自动生成 URL Slug
- `SaveArticleImages.go` - 保存文章图片到本地/云存储 (支持速率限制)

**搜索引擎推送插件:**
- `PushToBaidu.go` - 百度搜索引擎推送
- `PushToBing.go` - 必应搜索引擎推送
- `NewDidiAuto.go` - 滴滴自动推送

**缓存优化插件:**
- `PreBuildArticleCache.go` - 预构建文章缓存

**数据采集插件:**
- `GnDownSpider.go` - 火车头内容采集

**内容展示插件:**
- `MakeCarousel.go` - 生成轮播图

**云存储插件:**
- `BaiduCloudTransfer.go` - 百度网盘转存
- `QuarkCloudTransfer.go` - 夸克网盘转存（支持广告处理）

**访问控制插件:**
- `DownloadLimit.go` - 下载频率限制 (支持 Cloudflare, IP 白名单)

**数据存储插件:**
- `PostStore.go` - 文章存储

#### 插件开发规范
- 实现必要的插件接口
- 提供清晰的插件元数据
- 使用结构化日志记录
- 支持配置热加载
- 遵循错误处理规范

---

## 开发环境配置

### 热重载配置

#### 后端热重载 (Air)
配置文件: `main/.air.toml`
- 监控 `.go`, `.tpl`, `.tmpl`, `.html`, `.toml` 文件
- 排除测试文件和临时目录
- 支持日志记录到 `logs/air-build-errors.log`
- 1 秒延迟后自动重启

#### 前端热重载 (Vite)
配置文件: `admin/vite.config.js`
- 开发服务器: `http://0.0.0.0:3000`
- API 代理: `/admin/api` → `http://127.0.0.1:9008`
- 支持 PWA
- 自动代码分割

### Nginx 配置 (生产环境)
- `/` → Go 后端 (9008 端口) - 网站前台
- `/admin` → Vue 前端 (静态文件) - 管理后台
- `/admin/api/` → Go 后端 (9008 端口) - API 接口

---

## 开发注意事项

1. **数据库**: 开发环境使用 SQLite (`main/moss.db`)
2. **热重载**: 后端用 Air，前端用 Vite
3. **API 代理**: Vite 代理 `/admin/api` 到 `http://127.0.0.1:9008`
4. **端口占用**: 确保 3000、9008 端口可用
5. **提交前**: 运行 `go test ./...` 确保后端测试通过
6. **代码格式**: 提交前运行 `go fmt ./...` 格式化代码
7. **日志查看**: 使用 `task logs` 查看 Nginx 日志
8. **状态检查**: 使用 `task status` 检查开发环境状态
9. **开发访问**:
   - 网站前台: http://localhost:9008/
   - 管理后台: http://localhost:3000/admin/
   - API 地址: http://localhost:3000/admin/api/*
10. **生产访问** (通过 Nginx):
    - 网站前台: http://moss.l9.lc/
    - 管理后台: http://moss.l9.lc/admin
    - API 地址: http://moss.l9.lc/admin/api/*

---

## 技术栈详情

### 后端依赖
- **Web 框架**: Fiber v2.52.5
- **ORM**: GORM v1.25.12
- **配置管理**: Viper v1.19.0
- **日志**: Zap v1.27.0
- **数据库驱动**: SQLite, MySQL, PostgreSQL
- **缓存**: Redis (go-redis v9), BigCache
- **对象存储**: 阿里云 OSS, 腾讯云 COS, AWS S3, Google Cloud
- **HTML 解析**: goquery
- **文本处理**: bluemonday (HTML 净化)
- **定时任务**: robfig/cron v3
- **ID 生成**: Sony Snowflake, Yitter IDGenerator
- **性能分析**: Google pprof
- **并发**: ants (协程池)

### 前端依赖
- **框架**: Vue 3.2.47
- **构建工具**: Vite 3.2.3
- **UI 组件**: Naive UI 2.34.3, Arco Design 2.48.0
- **状态管理**: Pinia 2.1.4
- **路由**: Vue Router 4.2.4
- **国际化**: Vue I18n 9.2.2
- **HTTP 客户端**: Axios 1.3.2
- **富文本编辑器**: WangEditor 5.1.23
- **代码编辑器**: CodeMirror 6.0.1
- **工具库**: VueUse 9.12.0, vue-request 2.0.3
- **样式**: Tailwind CSS 3.2.6, Less 4.1.3
- **PWA**: vite-plugin-pwa 0.16.4

---

## 常见问题排查

### 前端不更新
```bash
# 重启 Vite
cd admin && pkill -f vite
npm run dev
```

### 后端不热重载
```bash
# 检查 Air 配置
cd main && air -v

# 手动重启
pkill moss
cd main && air
```

### Nginx 配置问题
```bash
# 测试配置
nginx -t

# 重载配置
systemctl reload nginx

# 查看错误日志
tail -f /www/wwwlogs/moss.l9.lc.error.log
```

### 数据库连接问题
- 检查配置文件 `main/conf.toml`
- 确认数据库服务运行正常
- 检查连接字符串格式

### 缓存问题
- 如使用 Redis, 确认 Redis 服务运行
- 检查缓存配置
- 使用 `task reset-admin` 重置管理员可清除部分缓存

---

## 相关文档

- [DEVELOPMENT.md](./DEVELOPMENT.md) - 详细开发指南
- [README.md](./README.md) - 项目简介
- [docs/README_EN.md](./docs/README_EN.md) - 英文文档
- [docs/theme/README.md](./docs/theme/README.md) - 主题开发文档
- [docs/template/README.md](./docs/template/README.md) - 模板开发文档

---

## 社区支持

- **GitHub**: https://github.com/ctwj/moss
- **QQ 交流群**: 68396947
- **TG 交流群**: https://t.me/mosscms
- **问题反馈**: GitHub Issues