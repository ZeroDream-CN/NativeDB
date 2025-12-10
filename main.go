package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"nativedb/internal/commands"
	"nativedb/internal/core"
	"nativedb/internal/server"
)

//go:embed frontend
var embeddedFrontend embed.FS

func main() {
	configPath := flag.String("c", "config.json", "Path to configuration file")
	flag.Parse()

	config, err := core.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if commands.Dispatch(os.Args) {
		return
	}

	core.InitDB(config)
	defer core.DB.Close()

	commands.CheckAndAutoImport()
	core.InitRedis(config)

	if core.RDB != nil {
		defer core.RDB.Close()
	}

	if config.FrontendPath != "" {
		if err := checkAndReleaseFrontend(config.FrontendPath); err != nil {
			log.Printf("[Warning] Failed to release frontend files: %v", err)
		}
	}

	if err := server.Start(config); err != nil {
		log.Fatal("Server start failed: ", err)
	}
}

func checkAndReleaseFrontend(targetPath string) error {
	// 1. 检查目录是否存在
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		return nil
	}

	fmt.Printf("Frontend directory '%s' not found. Extracting embedded assets...\n", targetPath)

	root := "frontend"

	err := fs.WalkDir(embeddedFrontend, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			name := d.Name()

			if strings.HasSuffix(name, ".map") {
				return nil
			}

			if strings.HasSuffix(name, ".js") && !strings.HasSuffix(name, ".min.js") {
				return nil
			}

			if strings.HasSuffix(name, ".css") && !strings.HasSuffix(name, ".min.css") {
				return nil
			}
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		destPath := filepath.Join(targetPath, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		return extractFile(path, destPath)
	})

	if err != nil {
		return err
	}

	fmt.Println("Frontend assets extracted successfully.")
	return nil
}

func extractFile(srcPath, destPath string) error {
	srcFile, err := embeddedFrontend.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if err2 := os.MkdirAll(filepath.Dir(destPath), 0755); err2 != nil {
		return err2
	}

	destFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	return err
}
