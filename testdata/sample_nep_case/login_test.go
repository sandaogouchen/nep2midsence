package login_test

import (
	"testing"

	"github.com/xxx/nep"
)

// TestLogin 测试用户登录流程
func TestLogin(t *testing.T) {
	page := nep.NewPage()

	// 打开登录页面
	page.Navigate("https://example.com/login")

	// 输入用户名
	page.FindElement("#username").SendKeys("testuser")

	// 输入密码
	page.FindElement("#password").SendKeys("testpass123")

	// 点击登录按钮
	page.FindElement(".login-btn").Click()

	// 等待跳转到仪表盘
	page.WaitForElement(".dashboard")

	// 验证登录成功
	text := page.FindElement(".welcome-msg").GetText()
	if text == "" {
		t.Error("登录后未显示欢迎信息")
	}
}

// TestLoginWithInvalidCredentials 测试无效凭证登录
func TestLoginWithInvalidCredentials(t *testing.T) {
	page := nep.NewPage()

	page.Navigate("https://example.com/login")
	page.FindElement("#username").SendKeys("wronguser")
	page.FindElement("#password").SendKeys("wrongpass")
	page.FindElement(".login-btn").Click()

	// 等待错误提示
	page.WaitForVisible(".error-message")
	errText := page.FindElement(".error-message").GetText()
	if errText == "" {
		t.Error("未显示错误信息")
	}
}
