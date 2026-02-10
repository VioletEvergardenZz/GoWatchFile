// 本文件用于配置加载与校验
package config

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"file-watch/internal/alert"
	"file-watch/internal/match"
	"file-watch/internal/models"
	"file-watch/internal/pathutil"
)

const (
	defaultUploadWorkers        = 3
	defaultUploadQueueSize      = 100
	defaultLogLevel             = "info"
	defaultLogToStd             = true
	defaultAPIBind              = ":8080"
	defaultSilence              = "10s"
	defaultAlertPollInterval    = "2s"
	defaultAlertStartFromEnd    = true
	defaultAlertSuppressEnabled = true
	defaultAIMaxLines           = 200
	defaultAITimeout            = "20s"
)

var allowedLogLevels = map[string]struct{}{
	"debug": {},
	"info":  {},
	"warn":  {},
	"error": {},
}

// LoadConfig 加载配置文件并应用默认值
func LoadConfig(configFile string) (*models.Config, error) {
	envCandidates := []string{".env"}
	if dir := filepath.Dir(configFile); dir != "" && dir != "." {
		envCandidates = append(envCandidates, filepath.Join(dir, ".env"))
	}
	if err := loadEnvFiles(envCandidates...); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg models.Config //值类型结构体 在栈上分配、读完填数据后再取地址即可
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	runtimeCfg, err := loadRuntimeConfig(configFile)
	if err != nil {
		return nil, err
	}
	applyRuntimeConfig(&cfg, runtimeCfg)

	if err := applyEnvOverrides(&cfg); err != nil {
		return nil, err
	}
	applyDefaults(&cfg)
	if err := ValidateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}
	return &cfg, nil
}

// ValidateConfig 验证配置必填项
func ValidateConfig(config *models.Config) error {
	if err := validateWatchDirs(config.WatchDir); err != nil {
		return err
	}
	if err := validateFileExt(config.FileExt); err != nil {
		return err
	}
	if err := requireValue(config.Bucket, "S3 Bucket"); err != nil {
		return err
	}
	if config.AK == "" || config.SK == "" {
		return fmt.Errorf("S3认证信息不能为空")
	}
	if err := validateEndpoint(config.Endpoint); err != nil {
		return err
	}
	if err := requireValue(config.Region, "S3 Region"); err != nil {
		return err
	}
	if err := validateLogLevel(config.LogLevel); err != nil {
		return err
	}
	if err := requireValue(config.APIBind, "API 监听地址"); err != nil {
		return err
	}
	if err := validateAlertConfig(config); err != nil {
		return err
	}
	if err := validateAIConfig(config); err != nil {
		return err
	}

	return nil
}

func applyEnvOverrides(cfg *models.Config) error {
	cfg.WatchDir = sanitizeConfigString(cfg.WatchDir)
	cfg.WatchExclude = sanitizeConfigString(cfg.WatchExclude)
	cfg.FileExt = sanitizeConfigString(cfg.FileExt)
	cfg.Silence = sanitizeConfigString(cfg.Silence)
	cfg.EmailHost = sanitizeConfigString(cfg.EmailHost)
	cfg.EmailFrom = sanitizeConfigString(cfg.EmailFrom)
	cfg.EmailTo = sanitizeConfigString(cfg.EmailTo)
	cfg.Bucket = sanitizeConfigString(cfg.Bucket)
	cfg.Endpoint = sanitizeConfigString(cfg.Endpoint)
	cfg.Region = sanitizeConfigString(cfg.Region)
	cfg.LogLevel = sanitizeConfigString(cfg.LogLevel)
	cfg.LogFile = sanitizeConfigString(cfg.LogFile)
	cfg.APIBind = sanitizeConfigString(cfg.APIBind)
	cfg.APIAuthToken = sanitizeConfigString(cfg.APIAuthToken)
	cfg.APICORSOrigins = sanitizeConfigString(cfg.APICORSOrigins)
	cfg.AlertRulesFile = sanitizeConfigString(cfg.AlertRulesFile)
	cfg.AlertLogPaths = sanitizeConfigString(cfg.AlertLogPaths)
	cfg.AlertPollInterval = sanitizeConfigString(cfg.AlertPollInterval)
	cfg.AIBaseURL = sanitizeConfigString(cfg.AIBaseURL)
	cfg.AIAPIKey = sanitizeConfigString(cfg.AIAPIKey)
	cfg.AIModel = sanitizeConfigString(cfg.AIModel)
	cfg.AITimeout = sanitizeConfigString(cfg.AITimeout)
	cfg.RobotKey = stringFromEnv("ROBOT_KEY", cfg.RobotKey)
	cfg.DingTalkWebhook = stringFromEnv("DINGTALK_WEBHOOK", cfg.DingTalkWebhook)
	cfg.DingTalkSecret = stringFromEnv("DINGTALK_SECRET", cfg.DingTalkSecret)
	cfg.EmailUser = stringFromEnv("EMAIL_USER", cfg.EmailUser)
	cfg.EmailPass = stringFromEnv("EMAIL_PASS", cfg.EmailPass)
	cfg.EmailHost = stringFromEnv("EMAIL_HOST", cfg.EmailHost)
	cfg.EmailFrom = stringFromEnv("EMAIL_FROM", cfg.EmailFrom)
	cfg.EmailTo = stringFromEnv("EMAIL_TO", cfg.EmailTo)
	emailPort, ok, err := intFromEnv("EMAIL_PORT")
	if err != nil {
		return err
	}
	if ok {
		cfg.EmailPort = emailPort
	}
	emailUseTLS, ok, err := boolFromEnv("EMAIL_USE_TLS")
	if err != nil {
		return err
	}
	if ok {
		cfg.EmailUseTLS = emailUseTLS
	}
	cfg.Bucket = stringFromEnv("S3_BUCKET", cfg.Bucket)
	cfg.Endpoint = stringFromEnv("S3_ENDPOINT", cfg.Endpoint)
	cfg.Region = stringFromEnv("S3_REGION", cfg.Region)
	cfg.APIAuthToken = stringFromEnv("API_AUTH_TOKEN", cfg.APIAuthToken)
	cfg.APICORSOrigins = stringFromEnv("API_CORS_ORIGINS", cfg.APICORSOrigins)
	forcePathStyle, ok, err := boolFromEnv("S3_FORCE_PATH_STYLE")
	if err != nil {
		return err
	}
	if ok {
		cfg.ForcePathStyle = forcePathStyle
	}
	disableSSL, ok, err := boolFromEnv("S3_DISABLE_SSL")
	if err != nil {
		return err
	}
	if ok {
		cfg.DisableSSL = disableSSL
	}
	cfg.AK = stringFromEnv("S3_AK", cfg.AK)
	cfg.SK = stringFromEnv("S3_SK", cfg.SK)
	aiEnabled, ok, err := boolFromEnv("AI_ENABLED")
	if err != nil {
		return err
	}
	if ok {
		cfg.AIEnabled = aiEnabled
	}
	cfg.AIBaseURL = stringFromEnv("AI_BASE_URL", cfg.AIBaseURL)
	cfg.AIAPIKey = stringFromEnv("AI_API_KEY", cfg.AIAPIKey)
	cfg.AIModel = stringFromEnv("AI_MODEL", cfg.AIModel)
	cfg.AITimeout = stringFromEnv("AI_TIMEOUT", cfg.AITimeout)
	aiMaxLines, ok, err := intFromEnv("AI_MAX_LINES")
	if err != nil {
		return err
	}
	if ok {
		cfg.AIMaxLines = aiMaxLines
	}
	return nil
}

func applyDefaults(cfg *models.Config) {
	if cfg.UploadWorkers <= 0 {
		cfg.UploadWorkers = defaultUploadWorkers
	}
	if cfg.UploadQueueSize <= 0 {
		cfg.UploadQueueSize = defaultUploadQueueSize
	}
	if cfg.UploadRetryEnabled == nil {
		cfg.UploadRetryEnabled = boolPtr(true)
	}
	if strings.TrimSpace(cfg.Silence) == "" {
		cfg.Silence = defaultSilence
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = defaultLogLevel
	}
	if cfg.LogToStd == nil {
		cfg.LogToStd = boolPtr(defaultLogToStd)
	}
	if strings.TrimSpace(cfg.APIBind) == "" {
		cfg.APIBind = defaultAPIBind
	}
	if strings.TrimSpace(cfg.AlertPollInterval) == "" {
		cfg.AlertPollInterval = defaultAlertPollInterval
	}
	if cfg.AlertSuppressEnabled == nil {
		cfg.AlertSuppressEnabled = boolPtr(defaultAlertSuppressEnabled)
	}
	if cfg.AlertStartFromEnd == nil {
		cfg.AlertStartFromEnd = boolPtr(defaultAlertStartFromEnd)
	}
	if strings.TrimSpace(cfg.AITimeout) == "" {
		cfg.AITimeout = defaultAITimeout
	}
	if cfg.AIMaxLines <= 0 {
		cfg.AIMaxLines = defaultAIMaxLines
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func stringFromEnv(envKey, current string) string {
	if val, ok := os.LookupEnv(envKey); ok {
		trimmed := strings.TrimSpace(val)
		if trimmed != "" {
			return trimmed
		}
	}
	return strings.TrimSpace(resolveEnvPlaceholder(current))
}

func boolFromEnv(envKey string) (bool, bool, error) {
	val, ok := os.LookupEnv(envKey)
	// 环境变量不存在
	if !ok {
		return false, false, nil
	}
	if strings.TrimSpace(val) == "" {
		return false, false, nil
	}
	// 把环境变量里的字符串解析成布尔值（允许前后空格）
	parsed, err := strconv.ParseBool(strings.TrimSpace(val))
	if err != nil {
		return false, false, fmt.Errorf("环境变量 %s 不是合法的布尔值: %w", envKey, err)
	}
	return parsed, true, nil
}

func intFromEnv(envKey string) (int, bool, error) {
	val, ok := os.LookupEnv(envKey)
	if !ok {
		return 0, false, nil
	}
	if strings.TrimSpace(val) == "" {
		return 0, false, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(val))
	if err != nil {
		return 0, false, fmt.Errorf("环境变量 %s 不是合法的整数: %w", envKey, err)
	}
	return parsed, true, nil
}

// 检查字符串是否为空
func requireValue(value, name string) error {
	if value == "" {
		return fmt.Errorf("%s不能为空", name)
	}
	return nil
}

func validateWatchDir(path string) error {
	if err := requireValue(path, "监控目录"); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("监控目录无效: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("监控目录不是一个目录")
	}
	return nil
}

func validateWatchDirs(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	dirs := pathutil.SplitWatchDirs(raw)
	if len(dirs) == 0 {
		return requireValue("", "监控目录")
	}
	for _, dir := range dirs {
		if err := validateWatchDir(dir); err != nil {
			return err
		}
	}
	return nil
}

func validateFileExt(ext string) error {
	trimmed := strings.TrimSpace(ext)
	if trimmed == "" {
		// 允许留空，表示不过滤后缀，监控所有文件
		return nil
	}
	// 支持多后缀并进行格式校验
	if _, err := match.ParseExtList(trimmed); err != nil {
		return err
	}
	return nil
}

func validateEndpoint(endpoint string) error {
	if err := requireValue(endpoint, "S3 Endpoint"); err != nil {
		return err
	}
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return fmt.Errorf("S3 Endpoint不能为空")
	}

	parsed, err := url.Parse(trimmed)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return nil
	}

	parsed, err = url.Parse("//" + trimmed)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("无效的 S3 Endpoint: %s", endpoint)
	}
	return nil
}

func validateLogLevel(level string) error {
	if err := requireValue(level, "日志级别"); err != nil {
		return err
	}
	level = strings.ToLower(strings.TrimSpace(level))
	if _, ok := allowedLogLevels[level]; !ok {
		return fmt.Errorf("不支持的日志级别: %s", level)
	}
	return nil
}

func validateAlertConfig(config *models.Config) error {
	if config == nil {
		return nil
	}
	ruleset := config.AlertRules
	logPaths := strings.TrimSpace(config.AlertLogPaths)
	enabled := config.AlertEnabled
	if !enabled {
		return nil
	}
	if ruleset == nil {
		return fmt.Errorf("告警规则不能为空")
	}
	if logPaths == "" {
		return fmt.Errorf("告警日志路径不能为空")
	}
	if err := alert.NormalizeRuleset(ruleset); err != nil {
		return fmt.Errorf("告警规则无效: %w", err)
	}
	if _, err := parseAlertInterval(config.AlertPollInterval); err != nil {
		return fmt.Errorf("告警轮询间隔无效: %w", err)
	}
	return nil
}

func validateAIConfig(config *models.Config) error {
	if config == nil {
		return nil
	}
	if !config.AIEnabled {
		return nil
	}
	baseURL := strings.TrimSpace(config.AIBaseURL)
	if baseURL == "" {
		return fmt.Errorf("AI_BASE_URL不能为空")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("AI_BASE_URL无效: %s", baseURL)
	}
	if strings.TrimSpace(config.AIAPIKey) == "" {
		return fmt.Errorf("AI_API_KEY不能为空")
	}
	if strings.TrimSpace(config.AIModel) == "" {
		return fmt.Errorf("AI_MODEL不能为空")
	}
	if strings.TrimSpace(config.AITimeout) == "" {
		return fmt.Errorf("AI_TIMEOUT不能为空")
	}
	if config.AIMaxLines <= 0 {
		return fmt.Errorf("AI_MAX_LINES必须大于零")
	}
	return nil
}

func parseAlertInterval(raw string) (time.Duration, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, nil
	}
	clean := strings.ToLower(trimmed)
	clean = strings.ReplaceAll(clean, "秒钟", "秒")
	clean = strings.ReplaceAll(clean, "秒", "s")
	clean = strings.ReplaceAll(clean, "分钟", "m")
	clean = strings.ReplaceAll(clean, "分", "m")
	clean = strings.ReplaceAll(clean, "小时", "h")
	clean = strings.TrimSpace(clean)
	if d, err := time.ParseDuration(clean); err == nil && d > 0 {
		return d, nil
	}
	numRe := regexp.MustCompile(`\d+`)
	if m := numRe.FindString(clean); m != "" {
		if v, err := strconv.Atoi(m); err == nil && v > 0 {
			return time.Duration(v) * time.Second, nil
		}
	}
	return 0, fmt.Errorf("时间间隔必须大于零")
}

func sanitizeConfigString(value string) string {
	trimmed := strings.TrimSpace(value)
	if isEnvPlaceholder(trimmed) {
		return ""
	}
	return trimmed
}

func isEnvPlaceholder(value string) bool {
	return strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}")
}

func resolveEnvPlaceholder(value string) string {
	trimmed := strings.TrimSpace(value)
	if isEnvPlaceholder(trimmed) {
		envKey := strings.TrimSuffix(strings.TrimPrefix(trimmed, "${"), "}")
		if envVal, ok := os.LookupEnv(envKey); ok {
			return strings.TrimSpace(envVal)
		}
		return ""
	}
	return value
}

func loadEnvFiles(paths ...string) error {
	seen := make(map[string]struct{})
	for _, p := range paths {
		if p == "" {
			continue //跳过当前循环剩余逻辑，直接进入下一次迭代
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{} //将路径添加到已处理集合中

		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			continue
		}
		if err := loadEnvFile(p); err != nil {
			return err
		}
	}
	return nil
}

func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("打开 env 文件 %s 失败: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f) //按行读取的扫描器
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text()) //在 Scan() 成功返回 true 后，取出刚刚读到的那一行内容（字符串）
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2) //只按第一个 = 拆成两段：parts[0] 是键，parts[1] 是值（后面允许值里再出现 = 也不会被继续拆）
		if len(parts) != 2 {
			return fmt.Errorf("env 文件 %s 中存在无效行: %s", path, line)
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key == "" {
			return fmt.Errorf("env 文件 %s 中存在无效键: %s", path, line)
		}

		if unquoted, ok := trimQuotes(val); ok {
			val = unquoted
		}

		if _, exists := os.LookupEnv(key); exists { //检查进程里是否已存在同名环境变量
			continue
		}
		if err := os.Setenv(key, val); err != nil { //设置进当前进程的环境变量 .env 不会覆盖现有值
			return fmt.Errorf("设置环境变量 %s 来自 %s 失败: %w", key, path, err)
		}
	}

	// 判断扫描过程中是否发生错误
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取 env 文件 %s 失败: %w", path, err)
	}
	return nil
}

func trimQuotes(val string) (string, bool) {
	//长度不足 2 时不可能同时有首尾引号
	if len(val) >= 2 {
		if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
			(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
			return val[1 : len(val)-1], true
		}
	}
	return val, false
}
