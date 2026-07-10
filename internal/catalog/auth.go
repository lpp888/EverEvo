package catalog

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Credential 平台凭证。
type Credential struct {
	Token  string `json:"token,omitempty"`  // API token / Cookie
	Cookie string `json:"cookie,omitempty"` // 完整 Cookie 字符串
}

// Credentials 全局凭证库。
var Credentials = map[string]Credential{}

// SetCredential 设置平台的认证凭证。
// 例如 SetCredential("modelscope", Credential{Cookie: "session_xxx=yyy"})
func SetCredential(source string, c Credential) {
	Credentials[source] = c
}

// authRequest 为请求注入认证头。
func authRequest(req *http.Request, source string) {
	c, ok := Credentials[source]
	if !ok {
		return
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if c.Cookie != "" {
		for _, part := range splitCookies(c.Cookie) {
			if stringsTrim(part) != "" {
				req.Header.Add("Cookie", stringsTrim(part))
			}
		}
	}
	req.Header.Set("User-Agent", "everevo/0.1")
}

// simpleAuthGet 发送带认证的 GET 请求（替代不带认证的 httpGetJSON）。
func simpleAuthGet(urlStr, source string) (*http.Response, error) {
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	authRequest(req, source)
	return httpClient.Do(req)
}

// authGetJSON 带认证的 JSON GET。
func authGetJSON(urlStr, source string, out interface{}) error {
	resp, err := simpleAuthGet(urlStr, source)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, urlStr)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func splitCookies(s string) []string {
	// Cookie string may contain "; " separated cookies
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ';' {
			parts = append(parts, stringsTrim(s[start:i]))
			start = i + 1
		}
	}
	if start < len(s) {
		parts = append(parts, stringsTrim(s[start:]))
	}
	return parts
}

func stringsTrim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
