package login_test

import (
	"testing"

	"github.com/xxx/midscene"
)

// TestLogin 测试用户登录流程
func TestLogin(t *testing.T) {
	page := midscene.NewPage()

	// 打开登录页面
	page.Goto("https://example.com/login")

	// 输入用户名
	ai.Action("在用户名输入框中输入 testuser")

	// 输入密码
	ai.Action("在密码输入框中输入 testpass123")

	// 点击登录按钮
	ai.Action("点击登录按钮")

	// 等待跳转到仪表盘
	ai.Assert("仪表盘页面已出现")

	// 验证登录成功
	text := ai.Query("获取欢迎信息的文本")
	if text == "" {
		t.Error("登录后未显示欢迎信息")
	}
}

// TestLoginWithInvalidCredentials 测试无效凭证登录
func TestLoginWithInvalidCredentials(t *testing.T) {
	page := midscene.NewPage()

	page.Goto("https://example.com/login")
	ai.Action("在用户名输入框中输入 wronguser")
	ai.Action("在密码输入框中输入 wrongpass")
	ai.Action("点击登录按钮")

	// 等待错误提示
	ai.Assert("错误信息可见")
	errText := ai.Query("获取错误信息的文本")
	if errText == "" {
		t.Error("未显示错误信息")
	}
}
