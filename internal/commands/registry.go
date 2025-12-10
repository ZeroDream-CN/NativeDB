package commands

import (
	"fmt"
	"nativedb/internal/core"
	"os"
)

type CommandHandler func(args []string) error

type CommandInfo struct {
	Name        string
	Description string
	Handler     CommandHandler
}

var registry = make(map[string]CommandInfo)

/**
 * @brief 注册一个命令
 * @param name 命令名
 * @param description 命令描述
 * @param handler 命令处理函数
 */
func Register(name string, description string, handler CommandHandler) {
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("Command '%s' already registered", name))
	}
	registry[name] = CommandInfo{
		Name:        name,
		Description: description,
		Handler:     handler,
	}
}

/**
 * @brief 执行一个命令
 * @param args 命令参数
 * @return bool 是否成功执行
 */
func Dispatch(args []string) bool {
	if len(args) < 2 {
		return false
	}

	cmdName := args[1]

	if cmdName == "help" || cmdName == "--help" || cmdName == "-h" {
		PrintUsage()
		return true
	}

	cmdInfo, exists := registry[cmdName]
	if !exists {
		return false
	}

	if core.DB == nil && core.Config != nil {
		core.InitDB(core.Config)
	}

	err := cmdInfo.Handler(args[2:])
	if err != nil {
		fmt.Printf("Error executing command '%s': %v\n", cmdName, err)
		os.Exit(1)
	}

	os.Exit(0)
	return true
}

/**
 * @brief 打印命令使用说明
 */
func PrintUsage() {
	fmt.Println("Available commands:")
	for name, info := range registry {
		fmt.Printf("  %-15s %s\n", name, info.Description)
	}
}
