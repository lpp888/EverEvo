package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"everevo/internal/httpclient"
	"time"
)

// verifyClient 带超时的 HTTP 客户端，避免网络不通时长期挂起。
var verifyClient = httpclient.New(8 * time.Second)

// UserInfo 验证后的用户信息。
type UserInfo struct {
	Valid    bool   `json:"valid"`
	Username string `json:"username"`
	Reason   string `json:"reason,omitempty"`
}

// Verify 验证 cookie/token 是否有效。返回用户信息。
func Verify(source, credential string) *UserInfo {
	switch source {
	case "huggingface":
		return verifyHF(credential)
	case "modelscope":
		return verifyMS(credential)
	}
	return &UserInfo{Valid: false, Reason: "不支持的平台"}
}

func verifyHF(credential string) *UserInfo {
	req, _ := http.NewRequest("GET", "https://huggingface.co/api/whoami-v2", nil)
	req.Header.Set("Cookie", credential)
	req.Header.Set("User-Agent", "everevo/0.1")
	resp, err := verifyClient.Do(req)
	if err != nil {
		return &UserInfo{Valid: false, Reason: "网络错误: " + err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return &UserInfo{Valid: false, Reason: "凭证已过期或无效"}
	}
	if resp.StatusCode != 200 {
		return &UserInfo{Valid: false, Reason: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := readJSON(resp, &body); err != nil {
		return &UserInfo{Valid: false, Reason: "解析响应失败"}
	}
	return &UserInfo{Valid: true, Username: body.Name}
}

func verifyMS(credential string) *UserInfo {
	// 不跟随重定向：访问需要登录的页面，302→login 说明未登录
	client := httpclient.New(8 * time.Second)
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	req, _ := http.NewRequest("GET", "https://modelscope.cn/my/overview", nil)
	req.Header.Set("Cookie", credential)
	req.Header.Set("User-Agent", "everevo/0.1")
	resp, err := client.Do(req)
	if err != nil {
		return &UserInfo{Valid: false, Reason: "网络错误: " + err.Error()}
	}
	defer resp.Body.Close()

	// 被重定向到登录页 → 无效
	if resp.StatusCode == 301 || resp.StatusCode == 302 {
		loc := resp.Header.Get("Location")
		if strings.Contains(loc, "login") || strings.Contains(loc, "signin") {
			return &UserInfo{Valid: false, Reason: "凭证已过期或无效"}
		}
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return &UserInfo{Valid: false, Reason: "凭证已过期或无效"}
	}
	if resp.StatusCode == 200 {
		// 已登录，从 HTML 提取用户名
		body, _ := io.ReadAll(resp.Body)
		html := string(body)
		username := extractUsername(html)
		return &UserInfo{Valid: true, Username: username}
	}
	return &UserInfo{Valid: false, Reason: fmt.Sprintf("HTTP %d", resp.StatusCode)}
}

// extractUsername 从 HTML 里提取用户名（多策略）。
func extractUsername(html string) string {
	// 策略 1：常见 JSON 字段（宽松匹配，覆盖 camelCase 和各种命名）
	jsonPatterns := []string{
		`"(?:userName|username|nickName|nickname|displayName|login|loginName|user_name|screen_name)"\s*:\s*"([^"\\]{1,80})"`,
	}
	for _, p := range jsonPatterns {
		re := regexp.MustCompile(p)
		m := re.FindStringSubmatch(html)
		if len(m) > 1 && m[1] != "" && m[1] != "null" {
			return m[1]
		}
	}

	// 策略 2：从 <title> 标签提取（如 "用户名 - 魔搭ModelScope"）
	titleRe := regexp.MustCompile(`<title>\s*([^<\-]{1,60})`)
	if m := titleRe.FindStringSubmatch(html); len(m) > 1 {
		name := strings.TrimSpace(m[1])
		// 排除通用的标题
		skip := []string{"ModelScope", "魔搭", "我的", "Login", "登录", "首页"}
		ignored := false
		for _, s := range skip {
			if strings.Contains(name, s) {
				ignored = true
				break
			}
		}
		if !ignored && name != "" {
			return name
		}
	}

	// 策略 3：从 meta 标签找
	metaRe := regexp.MustCompile(`<meta\s+(?:[^>]*\s)?(?:name|property)=["'](?:user|author|profile)["']\s+content=["']([^"']{1,80})["']`)
	if m := metaRe.FindStringSubmatch(html); len(m) > 1 {
		return m[1]
	}

	// 策略 4：从 data 属性找
	dataRe := regexp.MustCompile(`data-(?:username|user|login|name)=["']([^"']{1,80})["']`)
	if m := dataRe.FindStringSubmatch(html); len(m) > 1 {
		return m[1]
	}

	return ""
}

func readJSON(resp *http.Response, out interface{}) error {
	return json.NewDecoder(resp.Body).Decode(out)
}
