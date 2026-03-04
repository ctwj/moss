# Moss 前后端一体化开发指南

## 🎯 开发环境目标
- **前端热重载**: Vue 3 + Vite (已实现)
- **后端热重载**: Go + Air (待配置)
- **统一开发入口**: 一个命令启动所有服务
- **集成日志查看**: 前后端日志集中显示

## 🚀 快速开始

### 方案A：使用 Taskfile (推荐)
```bash
# 安装依赖
task init-admin       # 前端依赖
cd main && go mod tidy # 后端依赖

# 启动开发环境
task dev              # 同时启动前后端
```

### 方案B：手动启动
```bash
# 终端1：后端热重载
cd main && air

# 终端2：前端开发服务器  
cd admin && npm run dev

# 终端3：查看日志
tail -f /www/wwwlogs/moss.l9.lc.log
```

### 方案C：Docker开发环境
```bash
docker-compose -f docker-compose.dev.yml up
```

## 🔧 配置说明

### 1. 后端热重载 (Air)
Air配置文件: `.air.toml`
```toml
[build]
  cmd = "go build -o ./tmp/main ./cmd/web/main.go"
  bin = "./tmp/main"
  include_ext = ["go", "tpl", "tmpl", "html"]
  exclude_dir = ["assets", "tmp", "vendor", "testdata"]
  include_dir = []
  exclude_regex = ["_test.go"]
  exclude_unchanged = false
  follow_symlink = false
  log = "build-errors.log"
  poll = false
  poll_interval = 500
  delay = 1000
  stop_on_error = true
  send_interrupt = false
  kill_delay = 500

[log]
  time = false

[color]
  main = "magenta"
  watcher = "cyan"
  build = "yellow"
  runner = "green"

[misc]
  clean_on_exit = true
```

### 2. 前端开发配置
已配置：Vite 3 + Vue 3 + 热重载
- 开发服务器: http://localhost:3000/admin/
- API代理: `/admin/api` → `http://127.0.0.1:9008`
- 基路径: `/admin/` (开发环境)

### 3. Nginx开发配置
- `/` → Go后端 (9008端口) - 网站前台
- `/admin` → Vite前端 (3000端口) - 管理后台
- `/admin/api/` → Go后端 (9008端口) - API接口

## 📝 新增 Taskfile 任务

```yaml
dev:
  desc: "启动完整开发环境（前后端）"
  cmds:
    - echo "🚀 启动 Moss 开发环境..."
    - echo "🔧 后端: http://localhost:9008"
    - echo "🎨 前端: http://localhost:3000/admin/"
    - echo "🌐 访问: http://moss.l9.lc/"
    - echo "📊 后台: http://moss.l9.lc/admin"
    - task: dev-backend
    - task: dev-frontend

dev-backend:
  desc: "启动后端开发服务器（热重载）"
  cmds:
    - cd ./main && air
  silent: false
  background: true

dev-frontend:
  desc: "启动前端开发服务器"
  cmds:
    - cd ./admin && npm run dev
  silent: false
  background: true

logs:
  desc: "查看开发日志"
  cmds:
    - echo "=== Nginx 访问日志 ==="
    - tail -f /www/wwwlogs/moss.l9.lc.log
  silent: false

logs-backend:
  desc: "查看后端日志"
  cmds:
    - echo "=== Go 后端日志 ==="
    # 需要根据实际日志路径调整
    - tail -f ./main/logs/app.log 2>/dev/null || echo "日志文件不存在，查看进程输出"

logs-frontend:
  desc: "查看前端日志"
  cmds:
    - echo "=== Vite 前端日志 ==="
    - echo "前端日志在进程输出中，查看终端"

restart-nginx:
  desc: "重载 Nginx 配置"
  cmds:
    - nginx -t && systemctl reload nginx
    - echo "✅ Nginx 配置已重载"

reset-admin:
  desc: "重置管理员账户"
  cmds:
    - pkill moss 2>/dev/null || true
    - cd ./main && ./moss --username="admin" --password="admin123" &
    - echo "✅ 管理员已重置: admin / admin123"
    - echo "⚠️  请在10分钟内使用"
```

## 🔍 调试工具

### 1. 检查服务状态
```bash
# 查看所有相关进程
ps aux | grep -E "(air|vite|moss|nginx)" | grep -v grep

# 检查端口监听
ss -tlnp | grep -E ':(3000|9008|80)'
```

### 2. API测试
```bash
# 检查后端API
curl http://localhost:9008/admin/api/admin/exists

# 检查前端代理
curl -H "Host: moss.l9.lc" http://localhost/admin/api/admin/exists
```

### 3. 浏览器调试
1. **F12开发者工具** → Console/Network
2. **禁用缓存**: Network → Disable cache
3. **清除存储**: Application → Clear storage

## 🐛 常见问题

### 1. 前端不更新
```bash
# 重启Vite
cd admin && pkill -f vite
npm run dev
```

### 2. 后端不热重载
```bash
# 检查Air配置
cd main && air -v

# 手动重启
pkill moss
cd main && air
```

### 3. Nginx配置问题
```bash
# 测试配置
nginx -t

# 重载配置
systemctl reload nginx

# 查看错误日志
tail -f /www/wwwlogs/moss.l9.lc.error.log
```

### 4. 路由问题
- 前台: http://moss.l9.lc/ → Go后端
- 后台: http://moss.l9.lc/admin → Vue前端
- API: http://moss.l9.lc/admin/api/* → Go后端

## 📁 文件结构说明
```
moss/
├── main/                 # Go后端
│   ├── cmd/web/main.go   # 入口文件
│   ├── .air.toml         # Air热重载配置
│   └── conf.toml         # 配置文件
├── admin/                # Vue前端
│   ├── src/              # 源代码
│   ├── vite.config.js    # Vite配置
│   └── package.json      # 依赖配置
├── Taskfile.yml          # 任务定义
└── docker-compose.dev.yml # Docker开发配置
```

## 🔄 开发工作流

### 日常开发
1. **启动环境**: `task dev`
2. **修改代码**: 前后端同时修改
3. **自动重载**: 前后端热重载生效
4. **测试访问**: 
   - 网站前台: http://moss.l9.lc/
   - 管理后台: http://moss.l9.lc/admin
5. **查看日志**: `task logs`

### 代码提交
1. **后端测试**: `cd main && go test ./...`
2. **前端构建**: `cd admin && npm run build`
3. **提交代码**: `git commit -m "feat: ..."`

## 🚨 注意事项

1. **后端热重载**: Air会监控`.go`文件变化并自动重启
2. **前端代理**: Vite代理`/admin/api`到后端，确保API调用正常
3. **跨域问题**: Nginx已配置CORS头，开发环境正常
4. **数据库**: 开发环境使用SQLite，数据在`main/moss.db`
5. **端口占用**: 确保3000、9008、80端口未被其他程序占用

## 📞 支持

- **项目文档**: https://github.com/ctwj/moss
- **QQ交流群**: 68396947
- **TG交流群**: https://t.me/mosscms
- **问题反馈**: GitHub Issues
