<div align="center">
  <h1 align="center">Native Database</h1>
  <p align="center">
    用于查看 FiveM / Grand Theft Auto V 原生函数的数据库系统
    <br />
    <br />
    ·&nbsp;
    <a href="https://ndb.cfx.rs/"><strong>🌍 在线体验</strong></a>
    &nbsp;·&nbsp;
    <a href="https://github.com/ZeroDream-CN/NativeDB/issues">❔ 反馈问题</a>
    &nbsp;·&nbsp;
    <a href="https://github.com/ZeroDream-CN/NativeDB/issues">💭 请求功能</a>
    &nbsp;·
  </p>
</div>

![preview](https://github.com/user-attachments/assets/493e0f71-a159-495d-8909-0077b11236a7)

## 项目介绍

NativeDB 是一个用于浏览和查询 FiveM / Grand Theft Auto V 原生函数（Natives）的高性能数据库系统。它提供了一个现代化的 Web 界面，支持多用户管理、AI 辅助翻译、源码关联以及多种数据库后端支持。

该项目旨在为开发者提供一个快速、准确且易于部署的 Native 文档查询服务。

## 主要功能

* 现代化 Web 界面：基于 Tailwind CSS 构建的响应式界面，支持自定义主题，提供流畅的搜索和浏览体验。
* 多用户系统：支持多用户注册与管理，使用 JWT 进行安全鉴权，集成 Gravatar 头像支持。
* 双数据库支持：
  * MySQL: 适用于生产环境，支持高并发。
  * SQLite: 零配置启动，适用于个人开发或小型部署（无需安装 MySQL）。
* 智能缓存系统：
  * Redis: 可选开启，用于高性能缓存。
  * 内存缓存: 当 Redis 未启用时，自动降级为内存缓存，无需额外依赖。
* 数据导入与补全：
  * 自动从 CFX 官方源下载并导入 natives.json。
  * 集成 GitHub (alloc8or) 数据源，自动补全缺失的描述。
  * 前端支持切换 FiveM（Cfx.re）数据源和 alloc8or 单机数据源。
* 源码关联：
  * 支持导入 C++ 底层源码和 Lua/C#/JS 示例代码。
* AI 辅助翻译：
  * 内置 AI 翻译引擎（支持 OpenAI 格式接口，如 DeepSeek），可批量自动翻译函数描述和参数说明。
* 单文件部署：前端静态资源可打包进 Go 二进制文件，运行时自动释放，开箱即用。

## 构建与安装

### 前置要求

* Go 1.20+
* (可选) MySQL 5.7+ / 8.0+ (如果使用 MySQL 模式)
* (可选) Redis (如果开启 Redis 缓存)

### 编译

项目提供了一键构建脚本 `build.sh`，可自动生成二进制文件并打包。

```bash
# 给予执行权限
chmod +x build.sh
# 执行构建
./build.sh
```

构建完成后，会在 `bin` 目录生成 `nativedb` 可执行文件的压缩包。

注意：构建时会将 `frontend` 目录下的静态资源嵌入到二进制文件中。请确保在构建前该目录已存在且包含前端文件。

## 配置说明

首次运行程序时，会自动在当前目录下生成 `config.json` 配置文件。

```js
{
    // 数据库配置
    "db_type": "sqlite",                       // 数据库类型: "mysql" 或 "sqlite"
    "db_host": "127.0.0.1",                    // MySQL 主机
    "db_port": 3306,                           // MySQL 端口
    "db_user": "root",                         // MySQL 用户
    "db_pass": "password",                     // MySQL 密码
    "db_name": "nativedb",                     // MySQL 数据库名
    "sqlite_db_path": "./nativedb.sqlite",     // SQLite 文件路径
    // 系统配置
    "bind_port": ":8080",                      // Web 服务监听端口
    "frontend": "./frontend",                  // 前端资源释放/读取路径
    "jwt_secret": "change_me",                 // JWT 密钥 (自动生成，建议修改)
    // 缓存相关配置
    "use_redis": false,                        // 是否启用 Redis
    "redis_host": "127.0.0.1",                 // Redis 主机
    "redis_port": 6379,                        // Redis 端口
    // AI 翻译相关配置
    "ai_base_url": "https://api.deepseek.com", // AI API 地址
    "ai_api_key": "your-api-key",              // AI API 密钥
    "ai_model": "deepseek-chat",               // AI 模型名称
    "ai_workers": 10,                          // 翻译并发线程数
    // Gravatar 镜像源
    "gravatar_mirror": "https://cravatar.cn/avatar/"
}
```

## 命令行工具

NativeDB 程序内置了多个 CLI 命令用于管理和维护系统。

### 1. 数据导入

初始化数据库或更新数据时使用。程序会自动下载最新的数据源。

```bash
# 导入/更新 GTA5 Native 数据 (自动补全缺失信息)
./nativedb import native

# 导入/更新 CFX (FiveM) 专有 Native 数据
./nativedb import nativecfx

# 导入本地 C++ 源码文件 (指定目录)
./nativedb import sources ./natives_txt_dir
# 关于源码：在指定目录下存放 函数名称.txt 格式的文件，即可自动导入。
# 可从泄露的 GTA5 源代码中提取，由于涉及 Rockstar Games 商业机密，本项目无法提供。
```

提示：如果数据库为空，直接运行 `./nativedb` 启动服务时也会自动触发初次导入流程。

### 2. 用户管理

```bash
# 创建管理员用户
# 用法: ./nativedb createuser <用户名> <邮箱>
./nativedb createuser admin admin@example.com

# 重置用户密码 (生成随机密码)
# 用法: ./nativedb resetpass <用户名>
./nativedb resetpass admin
```

### 3. AI 翻译

启动 AI 翻译任务，自动扫描数据库中未翻译的条目进行处理。

```bash
./nativedb translate
```

### 4. 缓存管理

```bash
# 清空 Redis 缓存 (仅在启用了 Redis 时有效)
./nativedb clearcache
```

## 启动服务

完成配置和数据导入后，直接运行程序即可启动 Web 服务器：

```bash
./nativedb
```

服务启动后，访问 http://localhost:58080 (或配置的端口) 即可使用。

## 在线编辑翻译

您在前端界面点击右上角的登录按钮，输入您创建的用户名和默认分配的密码即可登录。登录后即可实时对函数介绍、参数介绍以及示例代码进行编辑，编辑内容会自动保存到数据库，更新数据时不会被覆盖。

## 鸣谢

* [alloc8or](https://github.com/alloc8or/gta5-nativedb-data/) - 提供 GTA5 Native 数据和补全。
* [CFX](https://docs.fivem.net/) - 提供 FiveM 专有 Native 数据。

## 开源协议

本项目采用 [MIT License](LICENSE) 开源。


