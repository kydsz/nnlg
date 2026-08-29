// Package jwxt 实现教务系统（正方类）爬虫与数据同步，
// 与旧 Python 版 app/crawl 逻辑对齐。
package jwxt

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

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
	LoggedIn    bool

	lastUser, lastPass string
}

// Relogin 会话失效时重新登录
func (a *Auth) Relogin() error {
	if a.lastUser == "" {
		return errors.New("无已保存的凭据")
	}
	return a.Login(a.lastUser, a.lastPass)
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

// encodeBase64Custom 自定义 Base64 编码（与教务系统前端 JS 一致）
func encodeBase64Custom(input string) string {
	key := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/="
	var out strings.Builder
	chars := []rune(input)
	n := len(chars)
	for i := 0; i < n; {
		chr1 := chars[i]
		var chr2, chr3 rune
		if i+1 < n {
			chr2 = chars[i+1]
		}
		if i+2 < n {
			chr3 = chars[i+2]
		}
		i += 3

		enc1 := chr1 >> 2
		enc2 := ((chr1 & 3) << 4) | (chr2 >> 4)
		enc3 := ((chr2 & 15) << 2) | (chr3 >> 6)
		enc4 := chr3 & 63
		if chr2 == 0 {
			enc3, enc4 = 64, 64
		} else if chr3 == 0 {
			enc4 = 64
		}
		out.WriteByte(key[enc1])
		out.WriteByte(key[enc2])
		out.WriteByte(key[enc3])
		out.WriteByte(key[enc4])
	}
	return out.String()
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

// Login 登录教务系统并尝试切换到管理端
func (a *Auth) Login(username, password string) error {
	a.lastUser, a.lastPass = username, password
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
	lower := strings.ToLower(string(body))
	if strings.Contains(lower, "error") || strings.Contains(string(body), "失败") {
		return errors.New("登录失败：响应中包含错误信息")
	}
	log.Printf("[jwxt] 登录请求成功")

	// 切换管理端（失败不阻断，与旧版一致）
	if err := a.SwitchToAdmin(username); err != nil {
		log.Printf("[jwxt] 切换到管理端失败（教学端已登录）: %v", err)
	}
	a.LoggedIn = true
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
	client := *a.Client
	if timeout > 0 {
		client.Timeout = timeout
	}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range a.Headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	data, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s 状态码: %d", rawURL, resp.StatusCode)
	}
	return data, nil
}

// GetText GET 并将 GBK 等编码转为 UTF-8（适合 HTML 页面）
func (a *Auth) GetText(rawURL string, timeout time.Duration) (string, error) {
	data, err := a.Get(rawURL, timeout)
	if err != nil {
		return "", err
	}
	return toUTF8(data), nil
}

// PostForm 在已登录会话上发起表单 POST（原始字节）
func (a *Auth) PostForm(rawURL string, form url.Values, referer string, timeout time.Duration) ([]byte, error) {
	client := *a.Client
	if timeout > 0 {
		client.Timeout = timeout
	}
	req, err := http.NewRequest(http.MethodPost, rawURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
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
		return nil, err
	}
	data, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return data, fmt.Errorf("POST %s 状态码: %d", rawURL, resp.StatusCode)
	}
	return data, nil
}

// PostFormText POST 并将响应转 UTF-8（适合 HTML/JSON）
func (a *Auth) PostFormText(rawURL string, form url.Values, referer string, timeout time.Duration) (string, error) {
	data, err := a.PostForm(rawURL, form, referer, timeout)
	if err != nil {
		return "", err
	}
	return toUTF8(data), nil
}

// toUTF8 将教务系统常见 GBK 编码内容转为 UTF-8
func toUTF8(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}
	if out, err := simplifiedchinese.GB18030.NewDecoder().Bytes(data); err == nil {
		return string(out)
	}
	return string(data)
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
