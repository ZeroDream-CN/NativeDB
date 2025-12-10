package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nativedb/internal/core"
	"nativedb/internal/models"
)

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionRequest struct {
	Model          string        `json:"model"`
	Messages       []ChatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	ResponseFormat *FormatObj    `json:"response_format,omitempty"`
	MaxTokens      int           `json:"max_tokens"`
	Stream         bool          `json:"stream"`
}

type FormatObj struct {
	Type string `json:"type"`
}

type ChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

type TranslateTask struct {
	Hash                string
	Name                string
	DescriptionOriginal string
	ParamsJSON          []byte
}

var (
	totalCount      int32
	translatedCount int32
)

/**
 * @brief 初始化翻译命令
 */
func init() {
	Register("translate", "Auto translate natives using AI. Config in config.json", handleTranslate)
}

/**
 * @brief 处理翻译命令
 * @param args 命令参数
 * @return error 执行错误
 */
func handleTranslate(args []string) error {
	if core.Config == nil {
		return fmt.Errorf("config not loaded")
	}
	if core.DB == nil {
		core.InitDB(core.Config)
	}

	apiKey := core.Config.AiApiKey
	if apiKey == "" || apiKey == "your-api-key-here" {
		return fmt.Errorf("AI API Key not configured. Please edit config.json")
	}

	fmt.Print("Calculating pending tasks...\r")
	var pendingCount int
	err := core.DB.QueryRow("SELECT COUNT(*) FROM natives WHERE translation_status = 0").Scan(&pendingCount)
	if err != nil {
		return fmt.Errorf("failed to count pending tasks: %v", err)
	}

	atomic.StoreInt32(&totalCount, int32(pendingCount))
	atomic.StoreInt32(&translatedCount, 0)

	if pendingCount == 0 {
		fmt.Println("No pending translation tasks found. All done!")
		return nil
	}

	workerCount := core.Config.AiWorkers
	fmt.Printf("Starting AI Translation. Pending: %d, Workers: %d, Model: %s\n", pendingCount, workerCount, core.Config.AiModel)

	tasks := make(chan TranslateTask, workerCount*2)
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go worker(i, tasks, &wg)
	}

	go func() {
		defer close(tasks)
		for {
			// 每次取 100 条
			rows, err := core.DB.Query("SELECT hash, name, description_original, params FROM natives WHERE translation_status = 0 LIMIT 100")
			if err != nil {
				log.Printf("\nDB Query Error: %v", err)
				break
			}

			count := 0
			for rows.Next() {
				var t TranslateTask
				var paramsRaw []byte
				if err := rows.Scan(&t.Hash, &t.Name, &t.DescriptionOriginal, &paramsRaw); err != nil {
					continue
				}
				if len(paramsRaw) == 0 {
					t.ParamsJSON = []byte("[]")
				} else {
					t.ParamsJSON = paramsRaw
				}
				tasks <- t
				count++
			}
			rows.Close()

			if count == 0 {
				var check int
				core.DB.QueryRow("SELECT COUNT(*) FROM natives WHERE translation_status = 0").Scan(&check)
				if check == 0 {
					break
				}
				time.Sleep(2 * time.Second)
				continue
			}
		}
	}()

	wg.Wait()
	fmt.Println("\nTranslation job finished.")
	return nil
}

/**
 * @brief 翻译任务处理函数
 * @param id 工作线程 ID
 * @param tasks 任务通道
 * @param wg 等待组
 */
func worker(id int, tasks <-chan TranslateTask, wg *sync.WaitGroup) {
	defer wg.Done()

	for task := range tasks {
		processTask(task)
	}
}

/**
 * @brief 处理单个翻译任务
 * @param task 翻译任务
 */
func processTask(task TranslateTask) {
	var params []models.NativeParam
	if err := json.Unmarshal(task.ParamsJSON, &params); err != nil {
		params = []models.NativeParam{}
	}

	hasDesc := len(strings.TrimSpace(task.DescriptionOriginal)) > 1
	hasParamDesc := false
	for _, p := range params {
		if len(strings.TrimSpace(p.Description)) > 0 {
			hasParamDesc = true
			break
		}
	}

	if !hasDesc && !hasParamDesc {
		markAsTranslated(task.Hash, "", task.ParamsJSON)
		printProgress(task.Name, "SKIPPED")
		return
	}

	var lastErr error
	for retry := 0; retry < 3; retry++ {
		resultJSON, err := callAI(task.Name, task.DescriptionOriginal, params)
		if err != nil {
			lastErr = err
			time.Sleep(time.Second)
			continue
		}

		cleanJSON := cleanCodeBlock(resultJSON)

		var aiResult struct {
			DescriptionCn string            `json:"description_cn"`
			ParamsCn      map[string]string `json:"params_cn"`
		}

		if err := json.Unmarshal([]byte(cleanJSON), &aiResult); err != nil {
			lastErr = err
			continue
		}

		finalDescCn := aiResult.DescriptionCn
		updatedParams := false
		if aiResult.ParamsCn != nil {
			for i := range params {
				if cn, ok := aiResult.ParamsCn[params[i].Name]; ok && cn != "" {
					params[i].DescriptionCn = cn
					updatedParams = true
				}
			}
		}

		finalParamsJSON := task.ParamsJSON
		if updatedParams {
			finalParamsJSON, _ = json.Marshal(params)
		}

		if err := updateDatabase(task.Hash, finalDescCn, finalParamsJSON); err != nil {
			lastErr = err
		} else {
			printProgress(task.Name, "OK")
		}

		return
	}

	printProgress(task.Name, "FAILED")
	log.Printf("Translation failed for %s: %v", task.Name, lastErr)
}

/**
 * @brief 打印翻译进度
 * @param name 函数名
 * @param status 状态
 */
func printProgress(name, status string) {
	current := atomic.AddInt32(&translatedCount, 1)
	total := atomic.LoadInt32(&totalCount)
	fmt.Printf("\033[2K\r[%4d/%d] [%s] %s", current, total, status, name)
}

/**
 * @brief 调用 AI 模型进行翻译
 * @param name 函数名
 * @param originalDesc 原始描述
 * @param params 参数列表
 * @return string 翻译后的 JSON 字符串
 * @return error 调用错误
 */
func callAI(name, originalDesc string, params []models.NativeParam) (string, error) {
	paramsToTranslate := make(map[string]string)
	for _, p := range params {
		if p.Description != "" {
			paramsToTranslate[p.Name] = p.Description
		}
	}

	inputData := map[string]interface{}{
		"native_name": name,
		"description": originalDesc,
		"params":      paramsToTranslate,
	}
	inputJSON, _ := json.Marshal(inputData)

	systemPrompt := `你是一个 FiveM 文档翻译助手。请将输入内容翻译成中文。
### 严格规则：
1. **输出格式**：必须且只能返回合法的 **JSON** 对象。不要包含任何 Markdown 标记。
2. **保留格式**：保留 Markdown（代码块、加粗、列表）。
3. **术语映射**：
   - Ped -> 角色/实体
   - Vehicle -> 载具
   - Hash -> 哈希
   - Coordinates/Coords -> 坐标
   - Player -> 玩家
   - Native -> 函数
   - true/false -> true/false
4. **不要翻译**：参数名、变量名、代码片段。
5. **描述优化**：如果描述中的代码块把描述和代码混在了一起，请把代码块单独括起来，和描述分开。如果描述里没有代码或者不是代码，请去除代码块标记。

### 返回 JSON 结构示例：
{
    "description_cn": "翻译后的主描述...",
    "params_cn": {
        "p0": "翻译后的p0描述...",
        "modelHash": "翻译后的modelHash描述..."
    }
}`

	reqBody := ChatCompletionRequest{
		Model: core.Config.AiModel,
		Messages: []ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: string(inputJSON)},
		},
		Temperature:    0.1,
		ResponseFormat: &FormatObj{Type: "json_object"},
		MaxTokens:      4096,
	}

	jsonData, _ := json.Marshal(reqBody)

	url := strings.TrimRight(core.Config.AiBaseUrl, "/") + "/chat/completions"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+core.Config.AiApiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("api status %d", resp.StatusCode)
	}

	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(bodyBytes, &chatResp); err != nil {
		return "", err
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices")
	}

	return chatResp.Choices[0].Message.Content, nil
}

/**
 * @brief 更新数据库中的翻译结果
 * @param hash 函数哈希
 * @param descCn 翻译后的描述
 * @param paramsJSON 参数 JSON 字符串
 * @return error 更新错误
 */
func updateDatabase(hash, descCn string, paramsJSON []byte) error {
	_, err := core.DB.Exec("UPDATE natives SET description_cn = ?, params = ?, translation_status = 1 WHERE hash = ?", descCn, paramsJSON, hash)
	return err
}

/**
 * @brief 标记函数为已翻译
 * @param hash 函数哈希
 * @param descCn 翻译后的描述
 * @param paramsJSON 参数 JSON 字符串
 */
func markAsTranslated(hash, descCn string, paramsJSON []byte) {
	core.DB.Exec("UPDATE natives SET description_cn = ?, params = ?, translation_status = 1 WHERE hash = ?", descCn, paramsJSON, hash)
}

/**
 * @brief 清理代码块中的 JSON 字符串
 * @param s 包含 JSON 代码块的字符串
 * @return string 清理后的 JSON 字符串
 */
func cleanCodeBlock(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start != -1 && end != -1 && end > start {
		return s[start : end+1]
	}
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
