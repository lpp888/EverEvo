package auth

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// LoginResult 登录结果。
type LoginResult struct {
	Cookies string `json:"cookies"`
	Source  string `json:"source"`
}

// 各平台登录页 URL + 目标域名。
var loginPages = map[string]string{
	"huggingface": "https://huggingface.co/login",
	"modelscope":  "https://modelscope.cn/login",
}

// 目标域名（用于判断是否登录成功回到目标平台）。
var targetDomains = map[string]string{
	"huggingface": "huggingface.co",
	"modelscope":  "modelscope.cn",
}

// Login 打开浏览器让用户登录，自动截取 cookie。
// 检测逻辑：URL 回到目标域名 + 不在登录页 → 登录成功。
// 只截取目标域名的 cookie（不含第三方跳转的 cookie）。
func Login(source string, timeout time.Duration) (*LoginResult, error) {
	loginURL, ok := loginPages[source]
	if !ok {
		return nil, fmt.Errorf("不支持的平台: %s", source)
	}
	domain := targetDomains[source]

	browserPath, err := findBrowser()
	if err != nil {
		return nil, err
	}

	// 启动浏览器
	l := launcher.New().
		Bin(browserPath).
		Headless(false).
		Set("disable-blink-features", "AutomationControlled").
		Set("window-size", "1280,800")
	ctrlURL, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("启动浏览器失败: %w", err)
	}

	browser := rod.New().ControlURL(ctrlURL)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("连接浏览器失败: %w", err)
	}
	defer browser.MustClose()

	page, err := browser.Page(proto.TargetCreateTarget{URL: loginURL})
	if err != nil {
		return nil, fmt.Errorf("打开页面失败: %w", err)
	}
	page.MustWaitLoad()

	// 轮询：检测 URL 回到目标域名 + 不在登录/授权页
	deadline := time.Now().Add(timeout)
	errCount := 0 // 连续错误计数（浏览器关闭检测）
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)

		info, err := page.Info()
		if err != nil {
			errCount++
			// 连续 3 次错误 → 浏览器已关闭
			if errCount >= 3 {
				return nil, fmt.Errorf("浏览器已关闭，登录中断")
			}
			continue
		}
		errCount = 0 // 重置计数

		currentURL := strings.ToLower(info.URL)

		// 必须在目标域名（排除 github.com / oauth 中间页）
		if !strings.Contains(currentURL, domain) {
			continue
		}

		// 在目标域名但还在登录页/授权页 → 继续
		if strings.Contains(currentURL, "/login") ||
			strings.Contains(currentURL, "signin") ||
			strings.Contains(currentURL, "/oauth") ||
			strings.Contains(currentURL, "/authorize") {
			continue
		}

		// 回到目标域名 + 不在登录页 → 提取 cookie
		cookies, err := page.Cookies([]string{"https://" + domain})
		if err != nil || len(cookies) < 2 {
			continue
		}

		var sb strings.Builder
		for _, c := range cookies {
			sb.WriteString(c.Name)
			sb.WriteString("=")
			sb.WriteString(c.Value)
			sb.WriteString("; ")
		}
		return &LoginResult{Cookies: sb.String(), Source: source}, nil
	}
	return nil, fmt.Errorf("登录超时（%v 内未完成登录）", timeout)
}

// findBrowser 查找系统 Chrome 或 Edge。
func findBrowser() (string, error) {
	if path, ok := launcher.LookPath(); ok && path != "" {
		return path, nil
	}
	for _, p := range browserCandidates() {
		if path, err := exec.LookPath(p); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("未找到 Chrome 或 Edge")
}

func browserCandidates() []string {
	if runtime.GOOS == "windows" {
		return []string{
			"msedge", "chrome",
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		}
	}
	return []string{"google-chrome", "chromium", "chrome", "microsoft-edge"}
}
