package commands

import (
	"fmt"
	"nativedb/internal/core"

	"golang.org/x/crypto/bcrypt"
)

/**
 * @brief 初始化命令处理程序
 */
func init() {
	Register("createuser", "Create a new admin user. Usage: createuser <username> <email>", handleCreateUser)
	Register("resetpass", "Reset user password. Usage: resetpass <username>", handleResetPass)
	Register("clearcache", "Clear all Redis cache.", handleClearCache)
}

/**
 * @brief 创建新用户
 * @param args 命令参数
 * @return error 创建错误
 */
func handleCreateUser(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("missing arguments. Usage: createuser <username> <email>")
	}
	username := args[0]
	email := args[1]
	password := core.GenerateRandomPassword(12)

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = core.DB.Exec("INSERT INTO native_users (username, password_hash, email) VALUES (?, ?, ?)", username, string(hash), email)
	if err != nil {
		return fmt.Errorf("failed to create user: %v", err)
	}

	fmt.Printf("User created successfully!\nUsername: %s\nPassword: %s\n", username, password)
	return nil
}

/**
 * @brief 重置用户密码
 * @param args 命令参数
 * @return error 重置错误
 */
func handleResetPass(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("missing arguments. Usage: resetpass <username>")
	}
	username := args[0]
	password := core.GenerateRandomPassword(12)

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	res, err := core.DB.Exec("UPDATE native_users SET password_hash = ? WHERE username = ?", string(hash), username)
	if err != nil {
		return fmt.Errorf("failed to update password: %v", err)
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("user '%s' not found", username)
	}

	fmt.Printf("Password reset successfully!\nUsername: %s\nNew Password: %s\n", username, password)
	return nil
}

/**
 * @brief 清除所有 Redis 缓存
 * @param args 命令参数
 * @return error 清除错误
 */
func handleClearCache(args []string) error {
	fmt.Println("Connecting to Redis...")

	if core.RDB == nil {
		core.InitRedis(core.Config)
	}

	if core.RDB == nil {
		return fmt.Errorf("redis connection failed, cannot clear cache")
	}

	err := core.RDB.FlushDB(core.Ctx).Err()
	if err != nil {
		return fmt.Errorf("failed to flush Redis: %v", err)
	}

	fmt.Println("Redis cache cleared successfully.")
	return nil
}
