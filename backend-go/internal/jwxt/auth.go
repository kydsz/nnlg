// Package jwxt 实现教务系统（正方类）爬虫与数据同步，
// 与旧 Python 版 app/crawl 逻辑对齐。
package jwxt

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// Auth 教务系统认证（登录 + 切换管理端）
type Auth struct {
	BaseURLXS   string
	BaseURLGL   string
	MainPageURL string
	LoginURL    string
	Client      *http.Client
	Headers     map[string]string

	// mu 保护登录态字段；loginMu 串行化 Login/Relogin，
	// 避免多个同步任务并发登录时重复登录、cookie 会话互相覆盖。
	mu      sync.Mutex
	loginMu sync.Mutex

	loggedIn           bool
	lastLoginAt        time.Time
	lastUser, lastPass string
}

// IsLoggedIn 返回当前登录态（并发安全）
func (a *Auth) IsLoggedIn() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.loggedIn
}

// credentials 读取已保存凭据（并发安全）
func (a *Auth) credentials() (string, string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastUser, a.lastPass
}

// reloginMinInterval 内刚登录成功则不再重复登录，
// 让并发触发的 Relogin 收敛为一次真实登录（避免并发任务轮番登录把会话打乱）。
const reloginMinInterval = 3 * time.Second

// Relogin 会话失效时重新登录（并发调用时只登录一次，其余直接复用刚建立的会话）
func (a *Auth) Relogin() error {
	user, pass := a.credentials()
	if user == "" {
		return errors.New("无已保存的凭据")
	}
	a.mu.Lock()
	fresh := a.loggedIn && time.Since(a.lastLoginAt) < reloginMinInterval
	a.mu.Unlock()
	if fresh {
		return nil
	}
	return a.Login(user, pass)
}

// NewAuth 创建认证器（含 cookie 会话）
func NewAuth(baseURLXS, baseURLGL string) *Auth {
	jar, _ := cookiejar.New(nil)
	xs := strings.TrimSuffix(baseURLXS, "/")
	gl := strings.TrimSuffix(baseURLGL, "/")
	mainPage := xs + "/framework/jsMain.jsp"
	return &Auth{
		BaseURLXS:   xs,
		BaseURLGL:   gl,
		MainPageURL: mainPage,
		LoginURL:    xs + "/xk/LoginToXk",
		Client: &http.Client{
			Jar:     jar,
			Timeout: 60 * time.Second,
		},
		Headers: map[string]string{
			"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8",
			"Accept-Encoding": "gzip, deflate",
			"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
			"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36 Edg/127.0.0.0",
			"Connection":      "keep-alive",
		},
	}
}

// encodeBase64Custom 登录参数编码：字母表与教务系统前端 encodeInp 一致。
//
// 原实现按 rune 遍历并直接用 rune 值索引字母表：遇到非 ASCII（如中文密码）
// 会出现下标越界 panic，且编码结果并非字节序列。现按 UTF-8 字节编码——
// ASCII 输入结果与前端完全一致（即标准 Base64），非 ASCII 输入不再 panic。
func encodeBase64Custom(input string) string {
	return base64.StdEncoding.EncodeToString([]byte(input))
}

// GetInitialCookie 访问主页获取初始 cookie
func (a *Auth) GetInitialCookie() bool {
	req, err := http.NewRequest(http.MethodGet, a.MainPageURL, nil)
	if err != nil {
		return false
	}
	for k, v := range a.Headers {
		req.Header.Set(k, v)
	}
	resp, err := a.Client.Do(req)
	if err != nil {
		log.Printf("[jwxt] 获取初始 cookie 异常: %v", err)
		return false
	}
	_, _ = readBody(resp)
	if resp.StatusCode == http.StatusOK {
		log.Printf("[jwxt] 获取初始 cookie 成功")
		return true
	}
	log.Printf("[jwxt] 获取初始 cookie 失败，状态码: %d", resp.StatusCode)
	return false
}

// 登录结果判定：先看结构化响应，再看明确的成功/失败标记，
// 两者都拿不准时返回 unknown（由调用方按旧版宽容策略处理），不再用
// "响应里出现 error 就算失败" 这类子串启发式，避免页面改版后误判。
const (
	loginUnknown = iota
	loginOK
	loginFail
)

// 明确的失败标记（教务系统各分支页面文案）
var loginFailMarkers = []string{
	"用户名或密码错误", "密码错误", "用户不存在", "账号不存在", "账号或密码错误",
	"验证码错误", "登录失败", "登录超时", "请重新登录", "无权登录",
}

// 明确的成功标记
var loginOKMarkers = []string{
	"登录成功", "登录信息正确", "jsMain.jsp",
}

// loginVerdict 判定登录响应；detail 为失败原因（可从 JSON 消息中提取）
func loginVerdict(body []byte) (int, string) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		var payload map[string]interface{}
		if err := json.Unmarshal(trimmed, &payload); err == nil {
			if v, ok := payload["success"]; ok {
				switch t := v.(type) {
				case bool:
					if t {
						return loginOK, ""
					}
					return loginFail, jsonMessage(payload)
				case string:
					if t == "true" || t == "1" {
						return loginOK, ""
					}
					if t == "false" || t == "0" {
						return loginFail, jsonMessage(payload)
					}
				}
			}
		}
	}
	text := toUTF8(body, "")
	for _, marker := range loginFailMarkers {
		if strings.Contains(text, marker) {
			return loginFail, marker
		}
	}
	for _, marker := range loginOKMarkers {
		if strings.Contains(text, marker) {
			return loginOK, ""
		}
	}
	return loginUnknown, ""
}

// jsonMessage 从结构化响应中取错误消息
func jsonMessage(payload map[string]interface{}) string {
	for _, key := range []string{"msg", "message", "info", "error"} {
		if s, ok := payload[key].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return "教务系统返回登录失败"
}

// Login 登录教务系统并尝试切换到管理端（并发安全：同一实例上的登录串行执行）
func (a *Auth) Login(username, password string) error {
	a.loginMu.Lock()
	defer a.loginMu.Unlock()

	a.mu.Lock()
	a.lastUser, a.lastPass = username, password
	a.mu.Unlock()

	if !a.GetInitialCookie() {
		return errors.New("获取初始 cookie 失败")
	}

	encoded := encodeBase64Custom(username) + "%%%" + encodeBase64Custom(password)
	form := url.Values{"encoded": {encoded}}
	req, err := http.NewRequest(http.MethodPost, a.LoginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	for k, v := range a.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.Client.Do(req)
	if err != nil {
		return fmt.Errorf("登录请求异常: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return fmt.Errorf("读取登录响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("登录请求失败，状态码: %d", resp.StatusCode)
	}
	switch verdict, detail := loginVerdict(body); verdict {
	case loginFail:
		return fmt.Errorf("登录失败: %s", detail)
	case loginOK:
		log.Printf("[jwxt] 登录请求成功")
	default:
		// 页面改版后无法判定时保持旧版宽容行为，避免误报登录失败阻断同步
		log.Printf("[jwxt] 登录响应无明确成功标记，按已登录处理（响应 %d 字节）", len(body))
	}

	// 切换管理端（失败不阻断，与旧版一致）
	if err := a.SwitchToAdmin(username); err != nil {
		log.Printf("[jwxt] 切换到管理端失败（教学端已登录）: %v", err)
	}
	a.mu.Lock()
	a.loggedIn = true
	a.lastLoginAt = time.Now()
	a.mu.Unlock()
	return nil
}

// SwitchToAdmin 登录后切换到管理端
func (a *Auth) SwitchToAdmin(username string) error {
	// 步骤 1：获取 upd/time 切换参数
	ajaxURL := a.BaseURLXS + "/system/loginupdtoGld.do"
	req, err := http.NewRequest(http.MethodPost, ajaxURL, nil)
	if err != nil {
		return err
	}
	for k, v := range a.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Referer", a.MainPageURL)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := a.Client.Do(req)
	if err != nil {
		return fmt.Errorf("获取切换参数异常: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return fmt.Errorf("读取切换参数失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("获取切换参数失败，状态码: %d", resp.StatusCode)
	}
	var data struct {
		Val  string `json:"val"`
		Upd  string `json:"upd"`
		Time string `json:"time"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return fmt.Errorf("解析切换参数失败: %w", err)
	}
	if data.Val != "1" {
		return fmt.Errorf("切换参数返回错误: %s", truncate(string(body), 120))
	}

	// 步骤 2：提交切换表单
	form := url.Values{
		"view":        {"0"},
		"useraccount": {username},
		"ticket":      {""},
		"upd":         {data.Upd},
		"time":        {data.Time},
	}
	switchURL := a.BaseURLGL + "/Logon.do?method=logonFromJsxsd"
	req2, err := http.NewRequest(http.MethodPost, switchURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	for k, v := range a.Headers {
		req2.Header.Set(k, v)
	}
	req2.Header.Set("Referer", a.MainPageURL)
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp2, err := a.Client.Do(req2)
	if err != nil {
		return fmt.Errorf("切换请求异常: %w", err)
	}
	body2, err := readBody(resp2)
	if err != nil {
		return fmt.Errorf("读取切换响应失败: %w", err)
	}
	if resp2.StatusCode != http.StatusOK {
		return fmt.Errorf("切换失败，状态码: %d", resp2.StatusCode)
	}
	text := string(body2)
	if strings.Contains(text, "出错页面") || strings.Contains(text, "数据处理错误") {
		return errors.New("切换到管理端失败，返回出错页面")
	}
	log.Printf("[jwxt] 切换请求已发送")
	return nil
}

// Get 在已登录会话上发起 GET（原始字节，适合二进制下载）
func (a *Auth) Get(rawURL string, timeout time.Duration) ([]byte, error) {
	data, _, err := a.getWithType(rawURL, timeout)
	return data, err
}

// getWithType GET 并返回响应声明的 Content-Type（供编码判定使用）
func (a *Auth) getWithType(rawURL string, timeout time.Duration) ([]byte, string, error) {
	client := *a.Client
	if timeout > 0 {
		client.Timeout = timeout
	}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	for k, v := range a.Headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	data, err := readBody(resp)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.Header.Get("Content-Type"), fmt.Errorf("GET %s 状态码: %d", rawURL, resp.StatusCode)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// GetText GET 并将 GBK 等编码转为 UTF-8（适合 HTML 页面）
func (a *Auth) GetText(rawURL string, timeout time.Duration) (string, error) {
	data, contentType, err := a.getWithType(rawURL, timeout)
	if err != nil {
		return "", err
	}
	return toUTF8(data, contentType), nil
}

// PostForm 在已登录会话上发起表单 POST（原始字节）
func (a *Auth) PostForm(rawURL string, form url.Values, referer string, timeout time.Duration) ([]byte, error) {
	data, _, err := a.postFormWithType(rawURL, form, referer, timeout)
	return data, err
}

// postFormWithType POST 并返回响应声明的 Content-Type（供编码判定使用）
func (a *Auth) postFormWithType(rawURL string, form url.Values, referer string, timeout time.Duration) ([]byte, string, error) {
	client := *a.Client
	if timeout > 0 {
		client.Timeout = timeout
	}
	req, err := http.NewRequest(http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, "", err
	}
	for k, v := range a.Headers {
		req.Header.Set(k, v)
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	data, err := readBody(resp)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return data, resp.Header.Get("Content-Type"), fmt.Errorf("POST %s 状态码: %d", rawURL, resp.StatusCode)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// PostFormText POST 并将响应转 UTF-8（适合 HTML/JSON）
func (a *Auth) PostFormText(rawURL string, form url.Values, referer string, timeout time.Duration) (string, error) {
	data, contentType, err := a.postFormWithType(rawURL, form, referer, timeout)
	if err != nil {
		return "", err
	}
	return toUTF8(data, contentType), nil
}

// toUTF8 将教务系统返回内容转为 UTF-8。
// 优先按响应声明的 charset 解码（GBK/GB2312/BIG5 等由 htmlindex 解析），
// 未声明时再按「UTF-8 BOM -> UTF-8 合法性 -> GB18030」顺序探测。
func toUTF8(data []byte, contentType string) string {
	if len(data) == 0 {
		return ""
	}
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		return string(data[3:])
	}
	if name := charsetFromContentType(contentType); name != "" {
		if isUTF8Charset(name) {
			if utf8.Valid(data) {
				return string(data)
			}
		} else if enc, err := htmlindex.Get(name); err == nil {
			if out, derr := enc.NewDecoder().Bytes(data); derr == nil {
				return string(out)
			}
		}
	}
	if utf8.Valid(data) {
		return string(data)
	}
	if out, err := simplifiedchinese.GB18030.NewDecoder().Bytes(data); err == nil {
		return string(out)
	}
	return string(data)
}

// charsetFromContentType 解析 Content-Type 中的 charset（去引号、小写）
func charsetFromContentType(contentType string) string {
	idx := strings.Index(strings.ToLower(contentType), "charset=")
	if idx < 0 {
		return ""
	}
	value := contentType[idx+len("charset="):]
	if end := strings.IndexAny(value, ";, "); end >= 0 {
		value = value[:end]
	}
	return strings.ToLower(strings.Trim(value, `"'`))
}

func isUTF8Charset(name string) bool {
	switch name {
	case "utf-8", "utf8", "unicode-1-1-utf-8":
		return true
	}
	return false
}

// readBody 读取响应体（自动处理 gzip/deflate）
func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	var reader io.Reader = resp.Body
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			// 部分 jeeww 服务器误标 gzip，尝试原样读取
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, resp.Body)
			return buf.Bytes(), nil
		}
		defer gz.Close()
		reader = gz
	case "deflate":
		reader = flate.NewReader(resp.Body)
	}
	return io.ReadAll(reader)
}
