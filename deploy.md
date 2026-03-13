# Moss CMS 部署指南

本指南详细介绍如何在 Windows 或 Mac 上编译 Moss CMS，并将其部署到安装了宝塔系统的服务器上。

## 目录

- [环境准备](#环境准备)
- [本地编译](#本地编译)
  - [Windows 编译](#windows-编译)
  - [Mac 编译](#mac-编译)
- [服务器部署](#服务器部署)
  - [上传程序](#上传程序)
  - [配置程序](#配置程序)
  - [使用宝塔进程守护管理器](#使用宝塔进程守护管理器)
  - [配置 Nginx 反向代理](#配置-nginx-反向代理)
- [数据库配置](#数据库配置)
- [常见问题](#常见问题)
- [更新升级](#更新升级)

---

## 环境准备

### 本地环境要求

**Windows:**
- Go 1.23 或更高版本
- Node.js 18+ 和 npm/pnpm
- Git（可选，用于克隆代码）

**Mac:**
- Go 1.23 或更高版本
- Node.js 18+ 和 npm/pnpm
- Git（可选，用于克隆代码）

### 服务器环境要求

- 宝塔面板（推荐使用 Linux 系统，如 CentOS、Ubuntu、Debian）
- 支持的数据库：SQLite / MySQL / PostgreSQL
- （可选）Redis 缓存
- （可选）对象存储：阿里云 OSS / 腾讯云 COS / AWS S3 / Google Cloud

---

## 本地编译

### Windows 编译

#### 1. 安装开发工具

**安装 Go:**
1. 访问 https://golang.org/dl/
2. 下载 Windows 安装包
3. 运行安装程序，按照提示完成安装
4. 验证安装：打开命令提示符，输入 `go version`

**安装 Node.js:**
1. 访问 https://nodejs.org/
2. 下载 LTS 版本安装包
3. 运行安装程序，按照提示完成安装
4. 验证安装：打开命令提示符，输入 `node -v` 和 `npm -v`

#### 2. 克隆或下载源码

```bash
# 使用 Git 克隆（推荐）
git clone https://github.com/ctwj/moss.git
cd moss

# 或者从 GitHub Releases 下载源码压缩包并解压
```

#### 3. 安装依赖

```bash
# 安装前端依赖
cd admin
npm install

# 返回项目根目录
cd ..

# 安装 Go 依赖
cd main
go mod tidy

# 返回项目根目录
cd ..
```

#### 4. 编译项目

**方法一：使用 Taskfile（推荐）**

确保已安装 Task 工具：
```bash
# Windows 安装 Task（使用 PowerShell）
Invoke-WebRequest -Uri "https://github.com/go-task/task/releases/latest/download/task_windows_amd64.exe" -OutFile "task.exe"
# 将 task.exe 移动到 PATH 路径中
```

执行编译：
```bash
# 编译前端和管理后台
task build-admin

# 编译后端（Linux 版本，用于服务器部署）
task build-linux-amd64

# 或者一次性编译所有组件
task build
```

**方法二：手动编译**

```bash
# 编译前端
cd admin
npm run build
cd ..

# 编译后端（Linux 版本）
cd main
set GOOS=linux
set GOARCH=amd64
set CGO_ENABLED=0
go build -ldflags="-s -w" -trimpath -o ../dist/moss_linux_amd64 ./cmd/web/main.go
cd ..
```

编译完成后，产物位于：
- 前端：`main/resources/admin/`
- 后端：`dist/moss_linux_amd64`

#### 5. 打包发布文件

创建一个发布目录，将必要的文件打包：

```bash
# 创建发布目录
mkdir release
cd release

# 复制后端可执行文件
copy ..\dist\moss_linux_amd64 moss

# 复制前端资源
xcopy ..\main\resources\admin admin\ /E /I

# 复制主题资源
xcopy ..\main\resources\themes themes\ /E /I

# 复制应用资源
xcopy ..\main\resources\app app\ /E /I

# （可选）复制默认配置文件
copy ..\main\conf.toml conf.toml.example

# （可选）创建 README
echo "Moss CMS 部署包" > README.txt
echo "启动命令: ./moss" >> README.txt
echo "后台地址: /admin" >> README.txt
```

然后使用压缩工具（如 7-Zip）将 `release` 目录打包为 `moss-cms.zip`。

---

### Mac 编译

#### 1. 安装开发工具

**使用 Homebrew 安装（推荐）：**

```bash
# 如果没有安装 Homebrew，先安装
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# 安装 Go
brew install go

# 安装 Node.js
brew install node
```

**手动安装：**
- Go: https://golang.org/dl/
- Node.js: https://nodejs.org/

#### 2. 克隆或下载源码

```bash
# 使用 Git 克隆（推荐）
git clone https://github.com/ctwj/moss.git
cd moss

# 或者从 GitHub Releases 下载源码压缩包并解压
```

#### 3. 安装依赖

```bash
# 安装前端依赖
cd admin
npm install

# 返回项目根目录
cd ..

# 安装 Go 依赖
cd main
go mod tidy

# 返回项目根目录
cd ..
```

#### 4. 编译项目

**方法一：使用 Taskfile（推荐）**

安装 Task：
```bash
# macOS 安装 Task
brew install go-task
```

执行编译：
```bash
# 编译前端和管理后台
task build-admin

# 编译后端（Linux 版本，用于服务器部署）
task build-linux-amd64

# 或者一次性编译所有组件
task build
```

**方法二：手动编译**

```bash
# 编译前端
cd admin
npm run build
cd ..

# 编译后端（Linux 版本）
cd main
export GOOS=linux
export GOARCH=amd64
export CGO_ENABLED=0
go build -ldflags="-s -w" -trimpath -o ../dist/moss_linux_amd64 ./cmd/web/main.go
cd ..
```

编译完成后，产物位于：
- 前端：`main/resources/admin/`
- 后端：`dist/moss_linux_amd64`

#### 5. 打包发布文件

```bash
# 创建发布目录
mkdir release
cd release

# 复制后端可执行文件
cp ../dist/moss_linux_amd64 moss

# 复制前端资源
cp -r ../main/resources/admin admin/

# 复制主题资源
cp -r ../main/resources/themes themes/

# 复制应用资源
cp -r ../main/resources/app app/

# （可选）复制默认配置文件
cp ../main/conf.toml conf.toml.example

# （可选）创建 README
cat > README.txt << EOF
Moss CMS 部署包
启动命令: ./moss
后台地址: /admin
EOF

# 返回根目录并打包
cd ..
zip -r moss-cms.zip release
```

---

## 服务器部署

### 上传程序

#### 1. 通过 SFTP 上传

使用 FileZilla、WinSCP 或其他 SFTP 客户端连接服务器：

- 主机：服务器 IP
- 用户名：SSH 用户名
- 密码：SSH 密码
- 端口：22

将本地的 `moss-cms.zip` 上传到服务器的 `/www/wwwroot/` 目录。

#### 2. 通过宝塔面板上传

1. 登录宝塔面板
2. 点击左侧菜单「文件」
3. 进入 `/www/wwwroot/` 目录
4. 点击「上传」，选择 `moss-cms.zip`
5. 上传完成后，右键点击压缩包，选择「解压」

#### 3. 解压和设置权限

```bash
# 进入宝塔面板的终端，或通过 SSH 连接服务器

# 创建网站目录
mkdir -p /www/wwwroot/moss

# 解压文件
cd /www/wwwroot/
unzip moss-cms.zip -d moss/

# 进入 moss 目录
cd moss/release

# 设置执行权限
chmod +x moss

# 设置目录权限（确保程序可以读写）
chmod -R 755 .
chown -R www:www .
```

### 配置程序

#### 1. 创建配置文件

```bash
# 复制示例配置文件
cp conf.toml.example conf.toml

# 编辑配置文件
nano conf.toml
```

#### 2. 配置文件说明

```toml
# 监听地址和端口
# 建议使用固定端口，方便 Nginx 反向代理
addr = ':9008'

# 数据库类型：sqlite / mysql / postgresql
db = 'sqlite'

# 数据源名称（DSN）
# SQLite 示例：
dsn = './moss.db?_pragma=journal_mode(WAL)'

# MySQL 示例：
# dsn = 'username:password@tcp(127.0.0.1:3306)/moss?charset=utf8mb4&parseTime=True&loc=Local'

# PostgreSQL 示例：
# dsn = 'host=127.0.0.1 port=5432 user=postgres password=your_password dbname=moss sslmode=disable'

# （可选）Redis 配置
# 如果需要使用 Redis 缓存，取消以下注释：
# [redis]
# addr = '127.0.0.1:6379'
# password = ''
# db = 0

# （可选）对象存储配置
# 根据需要选择云存储服务商
```

#### 3. 数据库准备

**使用 SQLite（默认，无需额外配置）：**
程序首次运行时会自动创建 SQLite 数据库文件 `moss.db`。

**使用 MySQL：**

1. 在宝塔面板中创建 MySQL 数据库
2. 记录数据库名称、用户名和密码
3. 在 `conf.toml` 中配置 DSN

```toml
db = 'mysql'
dsn = 'your_user:your_password@tcp(127.0.0.1:3306)/moss?charset=utf8mb4&parseTime=True&loc=Local'
```

**使用 PostgreSQL：**

1. 在宝塔面板中安装 PostgreSQL（如果未安装）
2. 创建数据库和用户
3. 在 `conf.toml` 中配置 DSN

```toml
db = 'postgresql'
dsn = 'host=127.0.0.1 port=5432 user=postgres password=your_password dbname=moss sslmode=disable'
```

### 使用宝塔进程守护管理器

#### 1. 安装进程守护管理器

1. 登录宝塔面板
2. 点击左侧菜单「软件商店」
3. 搜索「进程守护管理器」
4. 点击「安装」

#### 2. 添加守护进程

1. 点击左侧菜单「软件」
2. 找到「进程守护管理器」，点击「设置」
3. 点击「添加守护进程」

填写以下信息：

- **名称**：moss-cms（或其他易于识别的名称）
- **运行目录**：`/www/wwwroot/moss/release`
- **启动命令**：`/www/wwwroot/moss/release/moss`

点击「确定」保存配置。

#### 3. 启动进程

1. 在进程守护管理器列表中找到刚添加的进程
2. 点击「启动」按钮
3. 查看「日志」确认启动是否成功

如果启动失败，查看日志中的错误信息：
- 常见错误：
  - 端口被占用：修改 `conf.toml` 中的端口
  - 权限不足：检查文件权限
  - 数据库连接失败：检查数据库配置

#### 4. 验证服务运行

```bash
# 检查进程是否运行
ps aux | grep moss

# 检查端口是否监听
netstat -tlnp | grep 9008

# 或使用 ss 命令
ss -tlnp | grep 9008

# 测试 API
curl http://127.0.0.1:9008/
```

### 配置 Nginx 反向代理

#### 1. 创建网站

1. 登录宝塔面板
2. 点击左侧菜单「网站」
3. 点击「添加站点」

填写以下信息：

- **域名**：你的域名（如 `example.com`）
- **根目录**：`/www/wwwroot/moss/release`（或选择一个空目录）
- **FTP**：不创建
- **数据库**：不创建（已在步骤中配置）
- **PHP 版本**：纯静态

点击「提交」创建站点。

#### 2. 配置反向代理

1. 在网站列表中找到刚创建的站点
2. 点击「设置」
3. 点击「反向代理」
4. 点击「添加反向代理」

填写以下信息：

- **代理名称**：moss（或其他名称）
- **目标 URL**：`http://127.0.0.1:9008`
- **发送域名**：`$host`

点击「提交」保存配置。

#### 3. 高级配置（可选）

如果需要自定义 Nginx 配置，点击「配置文件」进行编辑：

```nginx
location / {
    proxy_pass http://127.0.0.1:9008;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    
    # 支持大文件上传
    client_max_body_size 100m;
    
    # 增加超时时间
    proxy_connect_timeout 600;
    proxy_send_timeout 600;
    proxy_read_timeout 600;
}

# 静态文件缓存（可选）
location ~* \.(jpg|jpeg|png|gif|ico|css|js)$ {
    proxy_pass http://127.0.0.1:9008;
    expires 30d;
    add_header Cache-Control "public, immutable";
}
```

点击「保存」应用配置。

#### 4. 配置 SSL（HTTPS）

1. 在网站设置中点击「SSL」
2. 选择证书类型：
   - **Let's Encrypt**：免费自动续期证书
   - **其他证书**：上传自己的证书
3. 填写相关信息，点击「申请」或「保存」
4. 开启「强制 HTTPS」

#### 5. 测试访问

1. 在浏览器中访问你的域名
2. 应该能看到 Moss CMS 的首页
3. 访问 `http://your-domain.com/admin` 进入管理后台

#### 6. 重置管理员账户

如果首次启动需要创建管理员账户：

```bash
# 停止进程
# 在宝塔进程守护管理器中点击「停止」

# 使用命令行启动并创建管理员
cd /www/wwwroot/moss/release
./moss --username="admin" --password="your_password"

# 启动成功后，按 Ctrl+C 停止

# 然后在宝塔进程守护管理器中重新启动
```

或在宝塔进程守护管理器中：

1. 修改启动命令为：
   ```
   /www/wwwroot/moss/release/moss --username="admin" --password="your_password"
   ```
2. 启动进程
3. 启动成功后，停止进程
4. 修改启动命令为原命令：
   ```
   /www/wwwroot/moss/release/moss
   ```
5. 重新启动

**注意**：管理员账户重置功能仅在启动后 10 分钟内有效。

---

## 数据库配置

### SQLite（默认）

- 无需额外配置
- 数据库文件：`/www/wwwroot/moss/release/moss.db`
- 备份：定期复制 `moss.db` 文件

### MySQL

#### 1. 在宝塔面板创建数据库

1. 点击左侧菜单「数据库」
2. 点击「添加数据库」

填写信息：
- 数据库名：moss
- 用户名：moss
- 密码：设置强密码
- 访问权限：本地服务器

#### 2. 配置 Moss

编辑 `/www/wwwroot/moss/release/conf.toml`：

```toml
db = 'mysql'
dsn = 'moss:your_password@tcp(127.0.0.1:3306)/moss?charset=utf8mb4&parseTime=True&loc=Local'
```

#### 3. 重启服务

在宝塔进程守护管理器中重启进程。

### PostgreSQL

#### 1. 安装 PostgreSQL

1. 点击左侧菜单「软件商店」
2. 搜索「PostgreSQL」
3. 点击「安装」

#### 2. 创建数据库和用户

```bash
# 切换到 postgres 用户
su - postgres

# 进入 PostgreSQL 命令行
psql

# 创建数据库
CREATE DATABASE moss;

# 创建用户
CREATE USER moss_user WITH PASSWORD 'your_password';

# 授权
GRANT ALL PRIVILEGES ON DATABASE moss TO moss_user;

# 退出
\q
exit
```

#### 3. 配置 Moss

编辑 `/www/wwwroot/moss/release/conf.toml`：

```toml
db = 'postgresql'
dsn = 'host=127.0.0.1 port=5432 user=moss_user password=your_password dbname=moss sslmode=disable'
```

#### 4. 重启服务

在宝塔进程守护管理器中重启进程。

---

## 常见问题

### 1. 端口被占用

**错误信息：**
```
bind: address already in use
```

**解决方案：**
```bash
# 查找占用端口的进程
netstat -tlnp | grep 9008

# 或使用 lsof
lsof -i :9008

# 杀死占用端口的进程
kill -9 <PID>

# 或修改 conf.toml 中的端口
addr = ':9009'
```

### 2. 权限不足

**错误信息：**
```
permission denied
```

**解决方案：**
```bash
# 设置正确的权限
cd /www/wwwroot/moss/release
chmod +x moss
chmod -R 755 .
chown -R www:www .
```

### 3. 数据库连接失败

**错误信息：**
```
connection refused
access denied for user
```

**解决方案：**
- 检查数据库服务是否运行
- 验证数据库用户名和密码
- 检查数据库地址和端口
- 确认数据库已创建

### 4. 前端资源 404

**错误信息：**
```
404 Not Found: /admin/xxx.js
```

**解决方案：**
- 确认前端资源已正确编译
- 检查 `main/resources/admin/` 目录是否存在
- 验证文件权限

### 5. Nginx 502 Bad Gateway

**错误信息：**
```
502 Bad Gateway
```

**解决方案：**
```bash
# 检查后端服务是否运行
ps aux | grep moss

# 检查端口是否监听
netstat -tlnp | grep 9008

# 查看 Nginx 错误日志
tail -f /www/wwwlogs/your-domain.error.log

# 查看 Moss 日志
# 在宝塔进程守护管理器中点击「日志」
```

### 6. 上传文件失败

**解决方案：**

1. 检查 Nginx 配置中的 `client_max_body_size`：
```nginx
client_max_body_size 100m;
```

2. 检查 Moss 配置中的上传限制

3. 检查磁盘空间

### 7. 内存不足

**解决方案：**

1. 增加服务器内存
2. 使用 Redis 缓存减轻数据库压力
3. 调整 Go 运行时的内存限制

### 8. 如何查看日志

**宝塔进程守护管理器：**
1. 点击「设置」
2. 点击「日志」
3. 查看实时日志

**命令行：**
```bash
# 查看 Moss 日志
tail -f /www/wwwroot/moss/release/logs/app.log

# 查看 Nginx 访问日志
tail -f /www/wwwlogs/your-domain.log

# 查看 Nginx 错误日志
tail -f /www/wwwlogs/your-domain.error.log
```

---

## 更新升级

### 方法一：覆盖更新

1. **备份数据**
   ```bash
   # 备份数据库
   cp /www/wwwroot/moss/release/moss.db /www/wwwroot/moss/release/moss.db.backup

   # 备份配置文件
   cp /www/wwwroot/moss/release/conf.toml /www/wwwroot/moss/release/conf.toml.backup
   ```

2. **停止服务**
   - 在宝塔进程守护管理器中停止进程

3. **上传新版本**
   - 上传新的 `moss-cms.zip` 到服务器
   - 解压覆盖旧文件（保留 `conf.toml` 和 `moss.db`）

4. **设置权限**
   ```bash
   cd /www/wwwroot/moss/release
   chmod +x moss
   chmod -R 755 .
   ```

5. **启动服务**
   - 在宝塔进程守护管理器中启动进程

### 方法二：目录切换更新

1. **准备新版本**
   ```bash
   # 创建新目录
   mkdir /www/wwwroot/moss_new

   # 上传并解压新版本到 moss_new
   ```

2. **复制配置和数据**
   ```bash
   # 复制配置文件
   cp /www/wwwroot/moss/release/conf.toml /www/wwwroot/moss_new/release/conf.toml

   # 复制数据库
   cp /www/wwwroot/moss/release/moss.db /www/wwwroot/moss_new/release/moss.db
   ```

3. **停止旧服务**
   - 在宝塔进程守护管理器中停止进程

4. **修改守护进程配置**
   - 运行目录改为：`/www/wwwroot/moss_new/release`
   - 启动命令改为：`/www/wwwroot/moss_new/release/moss`

5. **启动新服务**
   - 在宝塔进程守护管理器中启动进程

6. **验证成功后删除旧版本**
   ```bash
   rm -rf /www/wwwroot/moss
   ```

### 数据库迁移

如果新版本需要数据库迁移：

```bash
# Moss 通常会在首次启动时自动迁移数据库
# 如果需要手动迁移，请查看版本说明

# 查看日志确认迁移状态
tail -f /www/wwwroot/moss/release/logs/app.log
```

---

## 性能优化建议

### 1. 使用 Redis 缓存

编辑 `conf.toml`：

```toml
[redis]
addr = '127.0.0.1:6379'
password = ''
db = 0
```

### 2. 启用 Gzip 压缩

在 Nginx 配置中添加：

```nginx
gzip on;
gzip_vary on;
gzip_min_length 1024;
gzip_types text/plain text/css text/xml text/javascript application/x-javascript application/xml+rss application/json;
```

### 3. 配置 CDN

将静态资源（图片、CSS、JS）上传到 CDN，减轻服务器压力。

### 4. 使用对象存储

配置云存储（如阿里云 OSS、腾讯云 COS）存储上传的文件。

### 5. 定期备份数据

设置宝塔的计划任务，定期备份数据库和文件。

```bash
# 每天凌晨 3 点备份数据库
0 3 * * * cp /www/wwwroot/moss/release/moss.db /www/backup/moss_$(date +\%Y\%m\%d).db
```

---

## 安全建议

1. **修改默认管理员密码**
   - 首次登录后立即修改

2. **修改后台路径**
   ```bash
   ./moss --adminpath="your_secret_path"
   ```

3. **启用 HTTPS**
   - 使用 SSL 证书加密传输

4. **定期更新**
   - 关注 GitHub Releases，及时更新到最新版本

5. **限制访问**
   - 在 Nginx 中配置 IP 白名单（仅允许特定 IP 访问后台）

6. **配置防火墙**
   - 只开放必要的端口（80、443）
   - 关闭不必要的端口

7. **定期备份**
   - 备份数据库和配置文件

---

## 附录

### 完整配置文件示例

```toml
# 监听配置
addr = ':9008'

# 数据库配置
db = 'sqlite'
dsn = './moss.db?_pragma=journal_mode(WAL)'

# MySQL 配置示例
# db = 'mysql'
# dsn = 'username:password@tcp(127.0.0.1:3306)/moss?charset=utf8mb4&parseTime=True&loc=Local'

# PostgreSQL 配置示例
# db = 'postgresql'
# dsn = 'host=127.0.0.1 port=5432 user=postgres password=password dbname=moss sslmode=disable'

# Redis 缓存配置
[redis]
addr = '127.0.0.1:6379'
password = ''
db = 0

# 阿里云 OSS 配置
[oss_aliyun]
endpoint = 'oss-cn-hangzhou.aliyuncs.com'
access_key_id = 'your_access_key_id'
access_key_secret = 'your_access_key_secret'
bucket = 'your_bucket_name'
base_url = 'https://your-bucket.oss-cn-hangzhou.aliyuncs.com'

# 腾讯云 COS 配置
[oss_tencent]
secret_id = 'your_secret_id'
secret_key = 'your_secret_key'
region = 'ap-guangzhou'
bucket = 'your-bucket'
base_url = 'https://your-bucket.cos.ap-guangzhou.myqcloud.com'

# AWS S3 配置
[oss_aws]
access_key = 'your_access_key'
secret_key = 'your_secret_key'
region = 'us-east-1'
bucket = 'your-bucket'
endpoint = 's3.amazonaws.com'
base_url = 'https://your-bucket.s3.amazonaws.com'

# Google Cloud Storage 配置
[oss_google]
credentials_json = '/path/to/credentials.json'
bucket = 'your-bucket'
base_url = 'https://storage.googleapis.com/your-bucket'
```

### 宝塔 Nginx 配置模板

```nginx
server {
    listen 80;
    listen 443 ssl http2;
    server_name your-domain.com;

    # SSL 配置
    ssl_certificate /www/server/panel/vhost/cert/your-domain.com/fullchain.pem;
    ssl_certificate_key /www/server/panel/vhost/cert/your-domain.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # 安全头
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;

    # 反向代理
    location / {
        proxy_pass http://127.0.0.1:9008;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 支持大文件上传
        client_max_body_size 100m;

        # 超时设置
        proxy_connect_timeout 600;
        proxy_send_timeout 600;
        proxy_read_timeout 600;

        # WebSocket 支持
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    # 静态文件缓存
    location ~* \.(jpg|jpeg|png|gif|ico|css|js|svg|woff|woff2|ttf|eot)$ {
        proxy_pass http://127.0.0.1:9008;
        expires 30d;
        add_header Cache-Control "public, immutable";
    }

    # 访问日志
    access_log /www/wwwlogs/your-domain.log;
    error_log /www/wwwlogs/your-domain.error.log;

    # 强制 HTTPS
    if ($scheme != "https") {
        return 301 https://$host$request_uri;
    }
}
```

---

## 技术支持

- **GitHub 仓库**: https://github.com/ctwj/moss
- **QQ 交流群**: 68396947
- **Telegram 交流群**: https://t.me/mosscms
- **问题反馈**: GitHub Issues

---

## 更新日志

请访问 [GitHub Releases](https://github.com/ctwj/moss/releases) 查看版本更新日志。

---

**祝部署顺利！**