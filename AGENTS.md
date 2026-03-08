# AGENTS.md - Moss 开发指南

本文档为 AI 代理提供代码开发规范和构建命令参考。

## 项目概述

Moss 是一个基于 Go (Fiber) + Vue 3 的 CMS 系统：
- **后端**: Go 1.23, Fiber, GORM
- **前端**: Vue 3, Vite, Pinia, Vue Router, Vue I18n, Naive UI / Arco Design
- **主题**: Tailwind CSS (主题: germ, dute8)

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
task init-admin
task init-themes

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

# 构建前端
task build-admin

# 构建后端 (所有平台)
task build-main
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
├── infrastructure/   # 基础设施层
│   ├── general/       # 通用基础设施
│   │   ├── command/
│   │   ├── conf/
│   │   ├── constant/
│   │   └── message/
│   ├── persistent/    # 持久化
│   ├── support/       # 支持服务
│   └── utils/         # 工具函数
├── plugins/           # 插件
├── resources/         # 静态资源
├── startup/           # 初始化
└── cmd/web/           # 入口
```

#### 其他规范
- 使用 `var Xxx = new(XxxService)` 定义包级单例服务
- 事件驱动: 在服务中定义事件切片，通过 `AddXxxEvents` 方法注册
- 配置通过 Viper 加载，使用 `config.Config` 访问
- 日志使用 Zap: `zap.L()`, `zap.S()`

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
├── hooks/            # 组合式函数
├── layout/           # 布局组件
├── locale/           # i18n 翻译
├── router/           # 路由配置
├── store/            # Pinia 状态管理
├── views/            # 页面组件
└── main.js           # 入口
```

#### 组件规范
- 使用 `<script setup>` 语法
- 组件按功能模块组织在 views 目录下
- 通用组件放在 components 目录
- 使用 Naive UI / Arco Design 组件库

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
2. 在 `router/` 配置路由
3. 如需状态管理，在 `store/` 创建 Pinia store

### 插件开发

插件位于 `main/plugins/` 目录，实现特定接口后自动注册。

现有插件:
- `ArticleSanitizer.go` - 文章内容净化
- `DetectLinks.go` - 链接检测
- `GenerateDescription.go` - 生成描述
- `GenerateSlug.go` - 生成 Slug
- `GnDownSpider.go` - 火车头内容采集
- `MakeCarousel.go` - 生成轮播图
- `NewDidiAuto.go` - 滴滴自动推送
- `PostStore.go` - 文章存储
- `PreBuildArticleCache.go` - 预构建文章缓存
- `PushToBaidu.go` - 百度推送
- `PushToBing.go` - 必应推送
- `SaveArticleImages.go` - 保存文章图片

---

## 开发注意事项

1. **数据库**: 开发环境使用 SQLite (`main/moss.db`)
2. **热重载**: 后端用 Air，前端用 Vite
3. **API 代理**: Vite 代理 `/admin/api` 到 `http://127.0.0.1:9008`
4. **端口占用**: 确保 3000、9008 端口可用
5. **提交前**: 运行 `go test ./...` 确保后端测试通过
6. **开发访问**:
   - 网站前台: http://localhost:9008/
   - 管理后台: http://localhost:3000/admin/
   - API 地址: http://localhost:3000/admin/api/*

---

## 相关文档

- [DEVELOPMENT.md](./DEVELOPMENT.md) - 详细开发指南
- [README.md](./README.md) - 项目简介
