package core

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"

	"github.com/redis/go-redis/v9"
)

type AppConfig struct {
	// Database Common
	DbType string `json:"db_type"`

	// MySQL Config
	DbHost     string `json:"db_host"`
	DbPort     int    `json:"db_port"`
	DbUser     string `json:"db_user"`
	DbPass     string `json:"db_pass"`
	DbName     string `json:"db_name"`
	DbMaxConn  int    `json:"db_max_conn"`
	DbIdleConn int    `json:"db_idle_conn"`

	// SQLite Config
	SqliteDbPath string `json:"sqlite_db_path"`

	// Server Config
	BindPort     string `json:"bind_port"`
	FrontendPath string `json:"frontend"`
	AllowOrigins string `json:"allow_origins"`
	JwtSecret    string `json:"jwt_secret"`

	// Redis Config
	UseRedis  bool   `json:"use_redis"`
	RedisHost string `json:"redis_host"`
	RedisPort int    `json:"redis_port"`
	RedisPass string `json:"redis_pass"`
	RedisDB   int    `json:"redis_db"`

	// AI Config
	AiBaseUrl string `json:"ai_base_url"`
	AiApiKey  string `json:"ai_api_key"`
	AiModel   string `json:"ai_model"`
	AiWorkers int    `json:"ai_workers"`

	// Mirror
	GravatarMirror string `json:"gravatar_mirror"`
}

type User struct {
	ID           int    `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Email        string `json:"email"`
	Avatar       string `json:"avatar"`
}

type MemCacheItem struct {
	Data      []byte
	ExpiresAt time.Time
}

var (
	DB         *sql.DB
	RDB        *redis.Client
	LocalCache sync.Map
	Config     *AppConfig
	Ctx        = context.Background()
)

/**
 * @brief 加载应用配置
 * @param path 配置文件路径
 * @return *AppConfig 应用配置
 * @return error 加载错误
 */
func LoadConfig(path string) (*AppConfig, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Printf("Configuration file '%s' not found. Creating default config...\n", path)
		defaultConfig := AppConfig{
			DbType:         "sqlite",
			DbHost:         "127.0.0.1",
			DbPort:         3306,
			DbUser:         "root",
			DbPass:         "root",
			DbName:         "nativedb",
			DbMaxConn:      100,
			DbIdleConn:     10,
			SqliteDbPath:   "./nativedb.sqlite",
			BindPort:       ":58080",
			FrontendPath:   "./frontend",
			AllowOrigins:   "*",
			JwtSecret:      "please_change_this_secret_key_" + GenerateRandomPassword(8),
			UseRedis:       false,
			RedisHost:      "127.0.0.1",
			RedisPort:      6379,
			RedisPass:      "",
			RedisDB:        0,
			AiBaseUrl:      "https://api.deepseek.com",
			AiApiKey:       "your-api-key-here",
			AiModel:        "deepseek-chat",
			AiWorkers:      10,
			GravatarMirror: "https://www.gravatar.com/avatar/",
		}

		file, createErr := os.Create(path)
		if createErr != nil {
			return nil, fmt.Errorf("failed to create default config file: %v", createErr)
		}
		defer file.Close()

		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "    ")
		if encodeErr := encoder.Encode(defaultConfig); encodeErr != nil {
			return nil, fmt.Errorf("failed to write default config: %v", encodeErr)
		}
		fmt.Println("Default config created successfully.")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	config := &AppConfig{}
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(config); err != nil {
		return nil, err
	}
	if config.JwtSecret == "" {
		config.JwtSecret = "default-secret-please-change-me"
	}
	if config.AiWorkers <= 0 {
		config.AiWorkers = 5
	}
	if config.GravatarMirror == "" {
		config.GravatarMirror = "https://www.gravatar.com/avatar/"
	}
	if config.DbType == "" {
		config.DbType = "mysql"
	}

	Config = config
	return config, nil
}

/**
 * @brief 初始化数据库连接
 * @param config 应用配置
 */
func InitDB(config *AppConfig) {
	var dsn string
	var driverName string

	if config.DbType == "sqlite" {
		driverName = "sqlite"
		dsn = config.SqliteDbPath
		dir := filepath.Dir(dsn)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			os.MkdirAll(dir, 0755)
		}
		fmt.Printf("Using SQLite database: %s\n", dsn)
	} else {
		driverName = "mysql"
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			config.DbUser, config.DbPass, config.DbHost, config.DbPort, config.DbName)
		fmt.Printf("Using MySQL database: %s:%d\n", config.DbHost, config.DbPort)
	}

	var err error
	DB, err = sql.Open(driverName, dsn)
	if err != nil {
		log.Fatal("Error opening database: ", err)
	}

	if config.DbType == "sqlite" {
		DB.SetMaxOpenConns(1)
	} else {
		DB.SetMaxOpenConns(config.DbMaxConn)
		DB.SetMaxIdleConns(config.DbIdleConn)
	}
	DB.SetConnMaxLifetime(5 * time.Minute)

	if err := DB.Ping(); err != nil {
		log.Fatalf("Database ping failed: %v", err)
	}

	if config.DbType == "sqlite" {
		_, _ = DB.Exec("PRAGMA journal_mode=WAL;")
		_, _ = DB.Exec("PRAGMA foreign_keys=ON;")
	}

	createTables(config.DbType)
}

/**
 * @brief 创建数据库表
 * @param dbType 数据库类型 (mysql 或 sqlite)
 */
func createTables(dbType string) {
	var tables []string

	if dbType == "sqlite" {
		tables = []string{
			`CREATE TABLE IF NOT EXISTS natives (
				hash TEXT PRIMARY KEY,
				jhash TEXT,
				name TEXT,
				name_sp TEXT DEFAULT '', 
				namespace TEXT NOT NULL,
				params TEXT,
				return_type TEXT DEFAULT 'void',
				apiset TEXT DEFAULT 'client',
				game TEXT DEFAULT 'gta5',
				build_number INTEGER DEFAULT 0,
				description_original TEXT,
				description_cn TEXT,
				translation_status INTEGER DEFAULT 0,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);`,
			`CREATE INDEX IF NOT EXISTS idx_name ON natives(name);`,
			`CREATE INDEX IF NOT EXISTS idx_namespace ON natives(namespace);`,
			`CREATE INDEX IF NOT EXISTS idx_status ON natives(translation_status);`,

			`CREATE TABLE IF NOT EXISTS native_users (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				username TEXT NOT NULL UNIQUE,
				password_hash TEXT NOT NULL,
				email TEXT NOT NULL,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);`,

			`CREATE TABLE IF NOT EXISTS native_examples (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				native_hash TEXT NOT NULL,
				language TEXT DEFAULT 'lua',
				code TEXT NOT NULL,
				contributor TEXT DEFAULT 'System',
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (native_hash) REFERENCES natives(hash) ON DELETE CASCADE
			);`,
			`CREATE INDEX IF NOT EXISTS idx_ex_hash ON native_examples(native_hash);`,

			`CREATE TABLE IF NOT EXISTS native_sources (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				native_hash TEXT NOT NULL,
				code_content TEXT NOT NULL,
				code_lang TEXT DEFAULT 'cpp',
				source_type TEXT NOT NULL DEFAULT 'game_reversed',
				game_build TEXT DEFAULT NULL,
				contributor TEXT DEFAULT 'System',
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (native_hash) REFERENCES natives(hash) ON DELETE CASCADE
			);`,
			`CREATE INDEX IF NOT EXISTS idx_src_hash ON native_sources(native_hash);`,
		}
	} else {
		// MySQL Schema
		tables = []string{
			`CREATE TABLE IF NOT EXISTS natives (
				hash char(18) NOT NULL,
				jhash varchar(20) DEFAULT NULL,
				name varchar(100) DEFAULT NULL,
				name_sp varchar(100) DEFAULT '' COMMENT 'Single Player Name (Alloc8or)',
				namespace varchar(50) NOT NULL,
				params longtext CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL,
				return_type varchar(100) DEFAULT 'void',
				apiset varchar(20) DEFAULT 'client',
				game varchar(20) DEFAULT 'gta5',
				build_number int(11) DEFAULT 0,
				description_original text DEFAULT NULL,
				description_cn text DEFAULT NULL,
				translation_status tinyint(1) DEFAULT 0,
				created_at timestamp NULL DEFAULT current_timestamp(),
				updated_at timestamp NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
				PRIMARY KEY (hash),
				KEY idx_name (name),
				KEY idx_namespace (namespace),
				KEY idx_status (translation_status)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;`,

			`CREATE TABLE IF NOT EXISTS native_users (
				id int(11) NOT NULL AUTO_INCREMENT,
				username varchar(50) NOT NULL,
				password_hash varchar(255) NOT NULL,
				email varchar(100) NOT NULL,
				created_at timestamp NULL DEFAULT current_timestamp(),
				PRIMARY KEY (id),
				UNIQUE KEY username (username)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci ROW_FORMAT=DYNAMIC;`,

			`CREATE TABLE IF NOT EXISTS native_examples (
				id int(11) NOT NULL AUTO_INCREMENT,
				native_hash char(18) NOT NULL,
				language varchar(10) DEFAULT 'lua',
				code text NOT NULL,
				contributor varchar(50) DEFAULT 'System',
				updated_at datetime DEFAULT current_timestamp(),
				PRIMARY KEY (id),
				KEY native_hash (native_hash),
				CONSTRAINT native_examples_ibfk_1 FOREIGN KEY (native_hash) REFERENCES natives (hash) ON DELETE CASCADE
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;`,

			`CREATE TABLE IF NOT EXISTS native_sources (
				id int(10) unsigned NOT NULL AUTO_INCREMENT,
				native_hash char(18) NOT NULL,
				code_content text NOT NULL,
				code_lang varchar(20) DEFAULT 'cpp',
				source_type enum('cfx_open_source','game_reversed','pseudo_logic') NOT NULL DEFAULT 'game_reversed',
				game_build varchar(20) DEFAULT NULL,
				contributor varchar(50) DEFAULT 'System',
				created_at timestamp NULL DEFAULT current_timestamp(),
				updated_at timestamp NULL DEFAULT current_timestamp() ON UPDATE current_timestamp(),
				PRIMARY KEY (id),
				KEY idx_native_hash (native_hash),
				CONSTRAINT fk_source_native FOREIGN KEY (native_hash) REFERENCES natives (hash) ON DELETE CASCADE
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci ROW_FORMAT=DYNAMIC;`,
		}
	}

	for _, sqlStmt := range tables {
		if _, err := DB.Exec(sqlStmt); err != nil {
			log.Printf("Warning: Failed to ensure table exists. Error: %v", err)
		}
	}

	autoMigrate(dbType)
}

func autoMigrate(dbType string) {
	fmt.Println("Checking database schema...")

	if dbType == "sqlite" {
		_, err := DB.Exec("ALTER TABLE natives ADD COLUMN name_sp TEXT DEFAULT '';")
		if err == nil {
			fmt.Println("Migrated: Added 'name_sp' column to 'natives' table (SQLite).")
		}
	} else {
		// MySQL 检查列是否存在
		var dummy string
		err := DB.QueryRow("SELECT name_sp FROM natives LIMIT 1").Scan(&dummy)
		if err != nil {
			// 列不存在，添加
			_, err := DB.Exec("ALTER TABLE natives ADD COLUMN name_sp varchar(100) DEFAULT '' AFTER name;")
			if err != nil {
				log.Printf("Migration failed: %v", err)
			} else {
				fmt.Println("Migrated: Added 'name_sp' column to 'natives' table (MySQL).")
			}
		}
	}
}

/**
 * @brief 初始化 Redis 连接
 * @param config 应用配置
 */
func InitRedis(config *AppConfig) {
	if !config.UseRedis {
		fmt.Println("Redis is disabled in config. Using in-memory cache.")
		RDB = nil
		return
	}

	RDB = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", config.RedisHost, config.RedisPort),
		Password: config.RedisPass,
		DB:       config.RedisDB,
	})

	_, err := RDB.Ping(Ctx).Result()
	if err != nil {
		log.Printf("[Warning] Failed to connect to Redis: %v. Switching to in-memory cache.", err)
		RDB = nil
	} else {
		fmt.Println("Redis connected successfully.")
	}
}

/**
 * @brief 获取 Gravatar 头像 URL
 * @param email 用户邮箱
 * @return string Gravatar 头像 URL
 */
func GetGravatar(email string) string {
	hasher := md5.New()
	hasher.Write([]byte(strings.ToLower(strings.TrimSpace(email))))

	mirror := "https://www.gravatar.com/avatar/"
	if Config != nil && Config.GravatarMirror != "" {
		mirror = Config.GravatarMirror
		if !strings.HasSuffix(mirror, "/") {
			mirror += "/"
		}
	}

	return fmt.Sprintf("%s%s?d=retro", mirror, hex.EncodeToString(hasher.Sum(nil)))
}

/**
 * @brief 生成随机密码
 * @param length 密码长度
 * @return string 随机密码
 */
func GenerateRandomPassword(length int) string {
	chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return string(b)
}

/**
 * @brief 计算 GTA 哈希值
 * @param key 输入字符串
 * @return string 哈希值
 */
func Joaat(key string) string {
	key = strings.ToLower(key)
	var hash uint32 = 0
	for _, c := range key {
		hash += uint32(c)
		hash += (hash << 10)
		hash ^= (hash >> 6)
	}
	hash += (hash << 3)
	hash ^= (hash >> 11)
	hash += (hash << 15)
	return fmt.Sprintf("0x%08X", hash)
}

/**
 * @brief 下载文件
 * @param filepath 目标文件路径
 * @param url 源文件 URL
 * @return error 下载错误
 */
func DownloadFile(filepath string, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

/**
 * @brief 检查文件是否存在
 * @param filename 文件路径
 * @return bool 文件是否存在
 */
func FileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
