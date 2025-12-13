<div align="center">
  <h1 align="center">Native Database</h1>
  <p align="center">
    A database system for viewing FiveM / Grand Theft Auto V native functions
    <br />
    <br />
    ·&nbsp;
    <a href="https://ndb.cfx.rs/"><strong>🌍 Live Demo</strong></a>
    &nbsp;·&nbsp;
    <a href="https://github.com/ZeroDream-CN/NativeDB/issues">❔ Report Issue</a>
    &nbsp;·&nbsp;
    <a href="https://github.com/ZeroDream-CN/NativeDB/issues">💭 Request Feature</a>
    &nbsp;·&nbsp;
    <a href="README_zh.md">📃 中文文档</a>
    &nbsp;·
  </p>
</div>

![preview](https://github.com/user-attachments/assets/493e0f71-a159-495d-8909-0077b11236a7)

## Project Introduction

NativeDB is a high-performance database system for browsing and querying FiveM / Grand Theft Auto V native functions (Natives). It provides a modern web interface, supporting multi-user management, AI-assisted translation, source code association, and multiple database backend support.

This project aims to provide developers with a fast, accurate, and easily deployable Native documentation query service.

## Main Features

* Modern Web Interface: Responsive interface built with Tailwind CSS, supports custom themes, and offers a smooth search and browsing experience.
* Multi-User System: Supports multi-user registration and management, uses JWT for secure authentication, and integrates Gravatar avatar support.
* Dual Database Support:
  * MySQL: Suitable for production environments, supports high concurrency.
  * SQLite: Zero-configuration startup, suitable for personal development or small deployments (no MySQL installation required).
* Intelligent Caching System:
  * Redis: Optional, used for high-performance caching.
  * In-Memory Cache: Automatically falls back to in-memory caching when Redis is not enabled, requiring no additional dependencies.
* Data Import and Completion:
  * Automatically downloads and imports natives.json from the official CFX source.
  * Integrates GitHub (alloc8or) data source to automatically complete missing descriptions.
  * Frontend supports switching between FiveM (Cfx.re) data source and alloc8or single-player data source.
* Source Code Association:
  * Supports importing C++ source code and Lua/C#/JS example code.
* AI-Assisted Translation:
  * Built-in AI translation engine (supports OpenAI format APIs, such as DeepSeek), capable of batch automatic translation of function descriptions and parameter explanations.
* Single-File Deployment: Frontend static resources can be packaged into the Go binary file and automatically released at runtime, ready-to-use.

## Build & Installation

### Prerequisites

* Go 1.20+
* (Optional) MySQL 5.7+ / 8.0+ (if using MySQL mode)
* (Optional) Redis (if enabling Redis cache)

### Compilation

The project provides a one-click build script `build.sh`, which automatically generates the binary file and packages it.

```bash
# Grant execution permission
chmod +x build.sh
# Execute build
./build.sh
```

After the build completes, a compressed package containing the `nativedb` executable will be generated in the `bin` directory.

Note: During the build, static resources from the `frontend` directory are embedded into the binary file. Please ensure this directory exists and contains the frontend files before building.

## Configuration

Upon first run, the program will automatically generate a `config.json` configuration file in the current directory.

```js
{
    // Database Configuration
    "db_type": "sqlite",                       // Database type: "mysql" or "sqlite"
    "db_host": "127.0.0.1",                    // MySQL host
    "db_port": 3306,                           // MySQL port
    "db_user": "root",                         // MySQL user
    "db_pass": "password",                     // MySQL password
    "db_name": "nativedb",                     // MySQL database name
    "sqlite_db_path": "./nativedb.sqlite",     // SQLite file path
    // System Configuration
    "bind_port": ":8080",                      // Web service listening port
    "frontend": "./frontend",                  // Frontend resource release/read path
    "jwt_secret": "change_me",                 // JWT secret key (auto-generated, recommended to change)
    // Cache Configuration
    "use_redis": false,                        // Whether to enable Redis
    "redis_host": "127.0.0.1",                 // Redis host
    "redis_port": 6379,                        // Redis port
    // AI Translation Configuration
    "ai_base_url": "https://api.deepseek.com", // AI API address
    "ai_api_key": "your-api-key",              // AI API key
    "ai_model": "deepseek-chat",               // AI model name
    "ai_workers": 10,                          // Translation concurrency thread count
    // Gravatar Mirror Source
    "gravatar_mirror": "https://cravatar.cn/avatar/"
}
```

## Command Line Tools

The NativeDB program includes several CLI commands for system management and maintenance.

### 1. Data Import

Used for initializing the database or updating data. The program will automatically download the latest data sources.

```bash
# Import/update GTA5 Native data (automatically completes missing information)
./nativedb import native

# Import/update CFX (FiveM) exclusive Native data
./nativedb import nativecfx

# Import local C++ source code files (specify directory)
./nativedb import sources ./natives_txt_dir
# About source code: Place files in the specified directory in the format function_name.txt for automatic import.
# Can be extracted from leaked GTA5 source code. Due to Rockstar Games' commercial confidentiality, this project cannot provide them.
```

Tip: If the database is empty, directly running `./nativedb` to start the service will also automatically trigger the initial import process.

### 2. User Management

```bash
# Create an admin user
# Usage: ./nativedb createuser <username> <email>
./nativedb createuser admin admin@example.com

# Reset user password (generates a random password)
# Usage: ./nativedb resetpass <username>
./nativedb resetpass admin
```

### 3. AI Translation

Starts an AI translation task, automatically scanning untranslated entries in the database for processing.

```bash
./nativedb translate
```

### 4. Cache Management

```bash
# Clear Redis cache (only effective if Redis is enabled)
./nativedb clearcache
```

## Start Service

After completing configuration and data import, run the program directly to start the web server:

```bash
./nativedb
```

Once the service starts, access http://localhost:58080 (or the configured port) to use it.

## Online Edit & Translation

You can click the login button in the upper right corner of the frontend interface, enter the username and default assigned password you created to log in. After logging in, you can edit function descriptions, parameter explanations, and example code in real-time. Edited content is automatically saved to the database and will not be overwritten during data updates.

## Acknowledgments

* [alloc8or](https://github.com/alloc8or/gta5-nativedb-data/) - For providing GTA5 Native data and completions.
* [CFX](https://docs.fivem.net/) - For providing FiveM exclusive Native data.

## Open Source License

This project is open-sourced under the [MIT License](LICENSE).
