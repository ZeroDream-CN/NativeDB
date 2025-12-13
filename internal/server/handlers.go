package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"nativedb/internal/core"
	"nativedb/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	CacheKeyNativesList = "natives:list"
	CacheKeyNativeBase  = "native:"
	CacheExpire         = 30 * time.Minute
)

/**
 * @brief 从缓存中获取数据
 * @param c Gin 上下文
 * @param key 缓存键
 * @return 是否成功获取缓存
 */
func cacheGet(c *gin.Context, key string) bool {
	if core.Config.UseRedis && core.RDB != nil {
		val, err := core.RDB.Get(core.Ctx, key).Result()
		if err == nil {
			c.Header("Content-Type", "application/json; charset=utf-8")
			c.Header("X-Cache", "HIT-REDIS")
			c.String(http.StatusOK, val)
			return true
		}
		return false
	}

	val, ok := core.LocalCache.Load(key)
	if ok {
		item, ok := val.(core.MemCacheItem)
		if ok {
			// 检查过期
			if time.Now().After(item.ExpiresAt) {
				core.LocalCache.Delete(key)
				return false
			}
			c.Header("Content-Type", "application/json; charset=utf-8")
			c.Header("X-Cache", "HIT-MEM")
			c.String(http.StatusOK, string(item.Data))
			return true
		}
	}

	return false
}

/**
 * @brief 设置缓存数据
 * @param key 缓存键
 * @param data 要缓存的数据
 */
func cacheSet(key string, data interface{}) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return
	}

	if core.Config.UseRedis && core.RDB != nil {
		core.RDB.Set(core.Ctx, key, jsonBytes, CacheExpire)
		return
	}

	core.LocalCache.Store(key, core.MemCacheItem{
		Data:      jsonBytes,
		ExpiresAt: time.Now().Add(CacheExpire),
	})
}

/**
 * @brief 清除缓存
 * @param nativeHash 可选，指定要清除的原生哈希
 */
func clearCache(nativeHash string) {
	keysToDelete := []string{CacheKeyNativesList}
	if nativeHash != "" {
		keysToDelete = append(keysToDelete, CacheKeyNativeBase+nativeHash)
	}

	if core.Config.UseRedis && core.RDB != nil {
		core.RDB.Del(core.Ctx, keysToDelete...)
	}

	for _, k := range keysToDelete {
		core.LocalCache.Delete(k)
	}
}

/**
 * @brief 登录处理函数
 * @param c Gin 上下文
 */
func LoginHandler(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	var user core.User
	err := core.DB.QueryRow("SELECT id, username, password_hash, email FROM native_users WHERE username = ?", req.Username).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Email)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
		return
	}

	if err2 := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err2 != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"uid":      user.ID,
		"username": user.Username,
		"exp":      time.Now().Add(time.Hour * 72).Unix(),
	})

	tokenString, err := token.SignedString([]byte(core.Config.JwtSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": tokenString,
		"user": gin.H{
			"username": user.Username,
			"email":    user.Email,
			"avatar":   core.GetGravatar(user.Email),
		},
	})
}

/**
 * @brief 密码修改处理函数
 * @param c Gin 上下文
 */
func ChangePasswordHandler(c *gin.Context) {
	var req models.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	uidRaw, _ := c.Get("uid")
	uid := int(uidRaw.(float64))

	var currentHash string
	err := core.DB.QueryRow("SELECT password_hash FROM native_users WHERE id = ?", uid).Scan(&currentHash)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	if err2 := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.OldPassword)); err2 != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "旧密码错误"})
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Hash error"})
		return
	}

	_, err = core.DB.Exec("UPDATE native_users SET password_hash = ? WHERE id = ?", string(newHash), uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database update error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "密码修改成功"})
}

/**
 * @brief 获取当前用户信息
 * @param c Gin 上下文
 */
func GetCurrentUser(c *gin.Context) {
	username, _ := c.Get("username")
	var user core.User
	err := core.DB.QueryRow("SELECT id, username, email FROM native_users WHERE username = ?", username).Scan(&user.ID, &user.Username, &user.Email)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	user.Avatar = core.GetGravatar(user.Email)
	c.JSON(http.StatusOK, user)
}

/**
 * @brief 获取函数列表
 * @param c Gin 上下文
 */
func GetNativesList(c *gin.Context) {
	if cacheGet(c, CacheKeyNativesList) {
		return
	}

	query := `
		SELECT 
			n.hash, n.jhash, n.name, n.name_sp, n.namespace, n.apiset, n.return_type, n.params, n.build_number,
			(ns.native_hash IS NOT NULL) AS source_available,
			(ne.native_hash IS NOT NULL) AS example_available
		FROM natives n
		LEFT JOIN (SELECT DISTINCT native_hash FROM native_sources) ns ON n.hash = ns.native_hash
		LEFT JOIN (SELECT DISTINCT native_hash FROM native_examples) ne ON n.hash = ne.native_hash
		ORDER BY n.namespace ASC, n.name ASC;
	`
	rows, err := core.DB.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	natives := make([]models.NativeListResponse, 0, 6500)
	for rows.Next() {
		var n models.NativeListResponse
		var paramsJSON []byte
		if err := rows.Scan(&n.Hash, &n.JHash, &n.Name, &n.NameSP, &n.Namespace, &n.ApiSet, &n.ReturnType, &paramsJSON, &n.Build, &n.SourceAvailable, &n.ExampleAvailable); err != nil {
			continue
		}
		n.Params = json.RawMessage(paramsJSON)
		if len(paramsJSON) == 0 {
			n.Params = json.RawMessage("[]")
		}
		natives = append(natives, n)
	}

	cacheSet(CacheKeyNativesList, natives)
	c.JSON(http.StatusOK, natives)
}

/**
 * @brief 获取函数详情
 * @param c Gin 上下文
 */
func GetNativeDetail(c *gin.Context) {
	hash := c.Param("hash")
	if cacheGet(c, CacheKeyNativeBase+hash) {
		return
	}

	query := `SELECT hash, jhash, name, name_sp, namespace, apiset, return_type, params, build_number, description_original, description_cn FROM natives WHERE hash = ?`
	var n models.NativeDetailResponse
	var paramsJSON []byte
	err := core.DB.QueryRow(query, hash).Scan(&n.Hash, &n.JHash, &n.Name, &n.NameSP, &n.Namespace, &n.ApiSet, &n.ReturnType, &paramsJSON, &n.Build, &n.DescriptionOriginal, &n.DescriptionCn)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Native not found"})
		return
	}
	n.Params = json.RawMessage(paramsJSON)
	if len(paramsJSON) == 0 {
		n.Params = json.RawMessage("[]")
	}
	var hasSource bool
	core.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM native_sources ns JOIN natives n ON n.hash = ? WHERE ns.native_hash = n.hash OR (n.jhash IS NOT NULL AND ns.native_hash = n.jhash))`, hash).Scan(&hasSource)

	response := gin.H{"data": n, "source_available": hasSource}
	cacheSet(CacheKeyNativeBase+hash, response)
	c.JSON(http.StatusOK, response)
}

/**
 * @brief 获取函数源代码
 * @param c Gin 上下文
 */
func GetNativeSource(c *gin.Context) {
	hash := c.Param("hash")
	query := `
		SELECT ns.code_content, ns.code_lang, ns.source_type 
		FROM native_sources ns 
		JOIN natives n ON n.hash = ? 
		WHERE ns.native_hash = n.hash OR (n.jhash IS NOT NULL AND ns.native_hash = n.jhash) 
		ORDER BY CASE ns.source_type 
			WHEN 'game_reversed' THEN 1 
			WHEN 'cfx_open_source' THEN 2 
			ELSE 3 
		END DESC
		LIMIT 1
	`
	var s models.SourceCodeResponse
	err := core.DB.QueryRow(query, hash).Scan(&s.Content, &s.Language, &s.SourceType)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Source code not found"})
		return
	}
	c.JSON(http.StatusOK, s)
}

/**
 * @brief 获取函数示例代码
 * @param c Gin 上下文
 */
func GetNativeExamples(c *gin.Context) {
	hash := c.Param("hash")
	rows, err := core.DB.Query("SELECT id, language, code FROM native_examples WHERE native_hash = ?", hash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	examples := []models.ExampleResponse{}
	for rows.Next() {
		var ex models.ExampleResponse
		rows.Scan(&ex.ID, &ex.Language, &ex.Code)
		examples = append(examples, ex)
	}
	c.JSON(http.StatusOK, examples)
}

/**
 * @brief 添加或更新函数示例代码
 * @param c Gin 上下文
 */
func AddOrUpdateExample(c *gin.Context) {
	hash := c.Param("hash")
	var req models.ExampleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Language = strings.ToLower(req.Language)
	username := c.GetString("username")

	var existingId int
	err := core.DB.QueryRow("SELECT id FROM native_examples WHERE native_hash = ? AND language = ?", hash, req.Language).Scan(&existingId)
	if err == sql.ErrNoRows {
		_, err := core.DB.Exec("INSERT INTO native_examples (native_hash, language, code, contributor) VALUES (?, ?, ?, ?)", hash, req.Language, req.Code, username)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		var updateSQL string
		if core.Config.DbType == "sqlite" {
			updateSQL = "UPDATE native_examples SET code = ?, updated_at = CURRENT_TIMESTAMP, contributor = ? WHERE id = ?"
		} else {
			updateSQL = "UPDATE native_examples SET code = ?, updated_at = NOW(), contributor = ? WHERE id = ?"
		}
		_, err := core.DB.Exec(updateSQL, req.Code, username, existingId)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	clearCache(hash)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

/**
 * @brief 删除函数示例代码
 * @param c Gin 上下文
 */
func DeleteExample(c *gin.Context) {
	hash := c.Param("hash")
	lang := c.Query("language")
	if lang == "" {
		var req struct {
			Language string `json:"language"`
		}
		if c.ShouldBindJSON(&req) == nil {
			lang = req.Language
		}
	}
	if lang == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Language required"})
		return
	}
	res, err := core.DB.Exec("DELETE FROM native_examples WHERE native_hash = ? AND language = ?", hash, strings.ToLower(lang))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Example not found"})
		return
	}
	clearCache(hash)
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

/**
 * @brief 更新函数翻译
 * @param c Gin 上下文
 */
func UpdateNativeTranslation(c *gin.Context) {
	hash := c.Param("hash")
	var req models.UpdateTranslationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_, err := core.DB.Exec("UPDATE natives SET description_cn = ?, translation_status = 2 WHERE hash = ?", req.DescriptionCn, hash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	clearCache(hash)
	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

/**
 * @brief 更新函数参数翻译
 * @param c Gin 上下文
 */
func UpdateNativeParams(c *gin.Context) {
	hash := c.Param("hash")
	var req models.UpdateParamsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var currentParamsJSON []byte
	core.DB.QueryRow("SELECT params FROM natives WHERE hash = ?", hash).Scan(&currentParamsJSON)
	var currentParams []models.NativeParam
	json.Unmarshal(currentParamsJSON, &currentParams)
	updatedCount := 0
	for i := range currentParams {
		for _, newP := range req.Params {
			if currentParams[i].Name == newP.Name {
				currentParams[i].DescriptionCn = newP.DescriptionCn
				updatedCount++
				break
			}
		}
	}
	finalJSON, _ := json.Marshal(currentParams)
	core.DB.Exec("UPDATE natives SET params = ? WHERE hash = ?", finalJSON, hash)
	clearCache(hash)
	c.JSON(http.StatusOK, gin.H{"status": "updated", "updated_count": updatedCount})
}
