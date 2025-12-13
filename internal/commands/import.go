package commands

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"nativedb/internal/core"
)

const (
	URL_NATIVE_GTA     = "https://static.cfx.re/natives/natives.json"
	URL_NATIVE_CFX     = "https://static.cfx.re/natives/natives_cfx.json"
	URL_NATIVE_GITHUB  = "https://github.com/alloc8or/gta5-nativedb-data/raw/master/natives.json"
	FILE_NATIVE_GITHUB = "natives_github.json"
)

type NativeDoc struct {
	Name        string          `json:"name"`
	NameSP      string          `json:"-"`
	JHash       string          `json:"jhash"`
	Comment     string          `json:"comment"`
	Params      []NativeParam   `json:"params"`
	Results     string          `json:"results"`
	Description string          `json:"description"`
	Examples    []NativeExample `json:"examples"`
	Apiset      string          `json:"apiset"`
	Game        string          `json:"game"`
	Build       interface{}     `json:"build"`
}

type NativeParam struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Description   string `json:"description"`
	DescriptionCn string `json:"description_cn,omitempty"`
}

type NativeExample struct {
	Lang string `json:"lang"`
	Code string `json:"code"`
}

/**
 * @brief 初始化导入命令
 */
func init() {
	Register("import", "Import data. Usage: import <native|nativecfx|sources> [file/path]", handleImport)
}

/**
 * @brief 检查并自动导入数据
 */
func CheckAndAutoImport() {
	if core.DB == nil {
		return
	}

	var count int
	err := core.DB.QueryRow("SELECT COUNT(*) FROM natives").Scan(&count)
	if err != nil {
		log.Printf("[AutoImport] Failed to check database status: %v", err)
		return
	}

	if count == 0 {
		log.Println("[AutoImport] Database is empty. Starting automatic import...")
		log.Println("[AutoImport] Processing natives.json (GTA5)...")
		if err := runImportNative("natives.json", URL_NATIVE_GTA, "gta5", true); err != nil {
			log.Printf("[AutoImport] Failed to import natives.json: %v", err)
		}
		log.Println("[AutoImport] Processing natives_cfx.json (CFX)...")
		if err := runImportNative("natives_cfx.json", URL_NATIVE_CFX, "gta5", true); err != nil {
			log.Printf("[AutoImport] Failed to import natives_cfx.json: %v", err)
		}
		log.Println("[AutoImport] Automatic import completed.")
	}
}

/**
 * @brief 处理导入命令
 * @param args 命令参数
 * @return error 导入错误
 */
func handleImport(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("missing subcommand. usage: import <native|nativecfx|sources> [args]")
	}

	subCmd := args[0]
	restArgs := args[1:]

	if core.DB == nil {
		if core.Config == nil {
			return fmt.Errorf("config not loaded")
		}
		core.InitDB(core.Config)
	}

	switch subCmd {
	case "native":
		targetFile := "natives.json"
		if len(restArgs) > 0 {
			targetFile = restArgs[0]
		}
		return runImportNative(targetFile, URL_NATIVE_GTA, "gta5", true)
	case "nativecfx":
		targetFile := "natives_cfx.json"
		if len(restArgs) > 0 {
			targetFile = restArgs[0]
		}
		return runImportNative(targetFile, URL_NATIVE_CFX, "gta5", true)
	case "sources":
		targetDir := "natives"
		if len(restArgs) > 0 {
			targetDir = restArgs[0]
		}
		return runImportSources(targetDir)
	case "clear":
		return clearNatives()
	default:
		return fmt.Errorf("unknown subcommand: %s", subCmd)
	}
}

/**
 * @brief 运行导入原生函数数据
 * @param filePath 文件路径
 * @param downloadURL 下载 URL
 * @param defaultGame 默认游戏
 * @param usePatch 是否使用补丁
 * @return error 导入错误
 */
func runImportNative(filePath, downloadURL, defaultGame string, usePatch bool) error {
	if !core.FileExists(filePath) {
		fmt.Printf("File '%s' not found. Downloading from %s...\n", filePath, downloadURL)
		if err := core.DownloadFile(filePath, downloadURL); err != nil {
			return fmt.Errorf("download failed: %v", err)
		}
		fmt.Println("Download complete.")
	}

	patchMap := make(map[string]NativeDoc)
	if usePatch {
		if !core.FileExists(FILE_NATIVE_GITHUB) {
			if err := core.DownloadFile(FILE_NATIVE_GITHUB, URL_NATIVE_GITHUB); err == nil {
				fmt.Println("Patch file downloaded.")
			}
		}

		if core.FileExists(FILE_NATIVE_GITHUB) {
			if pm, err := loadNativeMap(FILE_NATIVE_GITHUB); err == nil {
				patchMap = pm
				fmt.Printf("Patch data loaded (%d entries).\n", len(patchMap))
			}
		}
	}

	fmt.Printf("Reading %s...\n", filePath)
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var data map[string]map[string]NativeDoc
	if err := json.Unmarshal(fileContent, &data); err != nil {
		return fmt.Errorf("json parse error: %v", err)
	}

	countProcessed := 0
	countUpdated := 0
	countExamples := 0

	for namespace, natives := range data {
		for hash, doc := range natives {
			if patch, found := patchMap[hash]; found {
				if patch.Name != "" && !strings.HasPrefix(patch.Name, "_0x") {
					doc.NameSP = patch.Name
				}

				if doc.Description == "" && patch.Description != "" {
					doc.Description = patch.Description
				}
				if len(patch.Examples) > 0 {
					doc.Examples = append(doc.Examples, patch.Examples...)
				}
			}

			if doc.Apiset == "" {
				doc.Apiset = "client"
			}
			if doc.Game == "" {
				doc.Game = defaultGame
			}

			buildNum := parseBuildNumber(doc.Build)

			finalParamsJSON, err := mergeParams(hash, doc.Params)
			if err != nil {
				log.Printf("Error merging params for %s: %v", hash, err)
				continue
			}

			var exists int
			core.DB.QueryRow("SELECT COUNT(*) FROM natives WHERE hash = ?", hash).Scan(&exists)

			if exists == 0 {
				_, err := core.DB.Exec(`
					INSERT INTO natives (hash, jhash, name, name_sp, namespace, params, return_type, description_original, apiset, game, build_number)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				`, hash, doc.JHash, doc.Name, doc.NameSP, namespace, finalParamsJSON, doc.Results, doc.Description, doc.Apiset, doc.Game, buildNum)
				if err != nil {
					log.Printf("Insert error %s: %v", hash, err)
				}
				countUpdated++
			} else {
				var updateSQL string
				if core.Config.DbType == "sqlite" {
					updateSQL = `UPDATE natives SET jhash=?, name=?, name_sp=?, namespace=?, params=?, return_type=?, description_original=?, apiset=?, game=?, build_number=?, updated_at=CURRENT_TIMESTAMP WHERE hash=?`
				} else {
					updateSQL = `UPDATE natives SET jhash=?, name=?, name_sp=?, namespace=?, params=?, return_type=?, description_original=?, apiset=?, game=?, build_number=?, updated_at=NOW() WHERE hash=?`
				}

				_, err := core.DB.Exec(updateSQL, doc.JHash, doc.Name, doc.NameSP, namespace, finalParamsJSON, doc.Results, doc.Description, doc.Apiset, doc.Game, buildNum, hash)
				if err != nil {
					log.Printf("Update error %s: %v", hash, err)
				}
				countUpdated++
			}

			if len(doc.Examples) > 0 {
				countExamples += importExamples(hash, doc.Examples)
			}

			countProcessed++
			if countProcessed%1000 == 0 {
				fmt.Printf("Processed %d natives...\r", countProcessed)
			}
		}
	}

	fmt.Printf("\nImport finished. Processed: %d, Examples added: %d\n", countProcessed, countExamples)
	return nil
}

/**
 * @brief 清除导入的原生函数数据
 * @return error 清除错误
 */
func clearNatives() error {
	filesToDelete := []string{FILE_NATIVE_GITHUB, "natives.json", "natives_cfx.json"}
	for _, file := range filesToDelete {
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("failed to delete %s: %v", file, err)
		}
	}
	return nil
}

/**
 * @brief 解析构建号
 * @param v 构建号值
 * @return int 解析后的构建号
 */
func parseBuildNumber(v interface{}) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case string:
		if val == "" {
			return 0
		}
		var i int
		fmt.Sscanf(val, "%d", &i)
		return i
	case float64:
		return int(val)
	case int:
		return val
	default:
		return 0
	}
}

/**
 * @brief 加载原生函数映射
 * @param filePath 文件路径
 * @return map[string]NativeDoc 原生函数映射
 * @return error 加载错误
 */
func loadNativeMap(filePath string) (map[string]NativeDoc, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var data map[string]map[string]NativeDoc
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, err
	}

	res := make(map[string]NativeDoc)
	for _, ns := range data {
		for hash, doc := range ns {
			res[hash] = doc
		}
	}
	return res, nil
}

/**
 * @brief 合并参数
 * @param hash 哈希值
 * @param newParams 新参数
 * @return []byte 合并后的参数 JSON
 * @return error 合并错误
 */
func mergeParams(hash string, newParams []NativeParam) ([]byte, error) {
	if len(newParams) == 0 {
		return []byte("[]"), nil
	}

	var oldParamsJSON []byte
	err := core.DB.QueryRow("SELECT params FROM natives WHERE hash = ?", hash).Scan(&oldParamsJSON)
	if err == sql.ErrNoRows {
		return json.Marshal(newParams)
	}
	if err != nil {
		return nil, err
	}

	var oldParams []NativeParam
	if len(oldParamsJSON) > 0 {
		_ = json.Unmarshal(oldParamsJSON, &oldParams)
	}

	cnMap := make(map[string]string)
	for _, p := range oldParams {
		if p.DescriptionCn != "" {
			cnMap[p.Name] = p.DescriptionCn
		}
	}

	for i := range newParams {
		if val, ok := cnMap[newParams[i].Name]; ok {
			newParams[i].DescriptionCn = val
		}
	}

	return json.Marshal(newParams)
}

/**
 * @brief 导入示例代码
 * @param hash 哈希值
 * @param examples 示例代码
 * @return int 导入的示例数量
 */
func importExamples(hash string, examples []NativeExample) int {
	added := 0
	for _, ex := range examples {
		lang := strings.ToLower(ex.Lang)
		code := strings.TrimSpace(ex.Code)
		if code == "" {
			continue
		}

		var exists int
		err := core.DB.QueryRow("SELECT 1 FROM native_examples WHERE native_hash = ? AND language = ? AND code = ?", hash, lang, code).Scan(&exists)
		if err == sql.ErrNoRows {
			_, err := core.DB.Exec("INSERT INTO native_examples (native_hash, language, code, contributor) VALUES (?, ?, ?, 'System_Import')", hash, lang, code)
			if err == nil {
				added++
			}
		}
	}
	return added
}

/**
 * @brief 运行导入原生函数源数据
 * @param dirPath 目录路径
 * @return error 导入错误
 */
func runImportSources(dirPath string) error {
	if !core.FileExists(dirPath) && !isDir(dirPath) {
		return fmt.Errorf("directory '%s' not found", dirPath)
	}

	fmt.Println("Building hash map from database...")
	hashMap, err := buildHashMap()
	if err != nil {
		return err
	}

	files, err := os.ReadDir(dirPath)
	if err != nil {
		return err
	}

	count := 0
	skipped := 0

	fmt.Println("Scanning files...")
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".txt") {
			continue
		}

		fileName := f.Name()
		nameNoExt := strings.TrimSuffix(fileName, ".txt")

		targetHash := matchHash(nameNoExt, hashMap)
		if targetHash == "" {
			continue
		}

		contentBytes, err := os.ReadFile(filepath.Join(dirPath, fileName))
		if err != nil {
			log.Printf("Failed to read %s: %v", fileName, err)
			continue
		}
		content := string(contentBytes)

		var id int
		err = core.DB.QueryRow("SELECT id FROM native_sources WHERE native_hash = ? AND source_type = 'game_reversed'", targetHash).Scan(&id)

		if err == sql.ErrNoRows {
			_, err = core.DB.Exec("INSERT INTO native_sources (native_hash, code_content, code_lang, source_type, contributor) VALUES (?, ?, 'cpp', 'game_reversed', 'Importer')", targetHash, content)
			if err == nil {
				count++
			}
		} else if err == nil {
			var updateSQL string
			if core.Config.DbType == "sqlite" {
				updateSQL = "UPDATE native_sources SET code_content = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?"
			} else {
				updateSQL = "UPDATE native_sources SET code_content = ?, updated_at = NOW() WHERE id = ?"
			}
			_, err = core.DB.Exec(updateSQL, content, id)
			if err == nil {
				count++
			}
		} else {
			skipped++
		}

		if (count+skipped)%100 == 0 {
			fmt.Printf("Processed sources: %d\r", count+skipped)
		}
	}

	fmt.Printf("\nSources import complete. Processed/Updated: %d\n", count)
	return nil
}

/**
 * @brief 检查路径是否为目录
 * @param path 路径
 * @return bool 是否为目录
 */
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

/**
 * @brief 构建哈希映射
 * @return map[string]string 哈希映射
 * @return error 构建错误
 */
func buildHashMap() (map[string]string, error) {
	rows, err := core.DB.Query("SELECT hash, name, jhash FROM natives")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]string)
	for rows.Next() {
		var hash, name string
		var jhash sql.NullString
		if err := rows.Scan(&hash, &name, &jhash); err != nil {
			continue
		}
		m[strings.ToLower(hash)] = hash
		if name != "" {
			m[strings.ToLower(name)] = hash
			joaat := strings.ToLower(core.Joaat(name))
			m[joaat] = hash
		}
		if jhash.Valid && jhash.String != "" {
			m[strings.ToLower(jhash.String)] = hash
		}
	}
	return m, nil
}

/**
 * @brief 匹配哈希值
 * @param filename 文件名
 * @param m 哈希映射
 * @return string 匹配的哈希值
 */
func matchHash(filename string, m map[string]string) string {
	key := strings.ToLower(filename)
	if hash, ok := m[key]; ok {
		return hash
	}
	if !strings.HasPrefix(key, "0x") {
		joaat := strings.ToLower(core.Joaat(filename))
		if hash, ok := m[joaat]; ok {
			return hash
		}
	}
	return ""
}
