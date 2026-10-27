# 人机验证

`captcha` 是可选插件，需要 PxBoard 0.4.5 或更新版本；任何默认使用模式均不安装此插件。安装并启用插件后，验证开关仍默认关闭，现有注册、登录和密码找回不受影响。

在插件中心安装“人机验证”，到后台“系统 → 人机验证”配置：

- 选择 Cloudflare Turnstile、hCaptcha 或 Google reCAPTCHA v2 复选框；填写该服务提供的站点密钥与服务端密钥。
- 选择保护密码登录、账户注册及请求密码重置的场景，再打开“启用人机验证”。绑定第三方账号不要求验证；选择登录或注册时，第三方登录发起授权前同样需要验证，以覆盖第三方注册。
- 浏览器直接加载所选服务的脚本，服务端向该服务的 Siteverify 接口验证一次性凭证。服务器和用户浏览器都必须能连接所选服务；reCAPTCHA 使用 `www.recaptcha.net`。
- 站点密钥公开，服务端密钥仅管理员配置，读取设置时不返回。修改其他字段时密钥留空会保留原密钥；切换服务商时必须填写新服务商的密钥。
- 服务不可用或插件运行失败时，被保护的操作会被拒绝，不会自动绕过。验证码已使用、过期或提交失败后重新验证。先在当前管理员会话中验证配置，再退出会话。

恢复访问：已登录的管理员可关闭验证开关或停用插件；如果没有可用管理员会话，在服务器停止 PxBoard 服务后，使用该实例的数据目录运行 `pxboard -data <数据目录> -disable-plugin captcha`，再启动服务。该操作只停用插件，不删除配置或账户数据。不要在面板服务仍运行时执行离线停用。

提供方配置：将生产域名加入服务商的允许域名，正式环境只使用正式密钥。服务商测试密钥不提供真实防护，不应配置在生产站点。

接口：`GET /api/v1/auth/challenge` 返回公开配置；登录、请求找回密码、声明为注册表单的插件接口和身份插件 `/authorize` 接收 `challengeToken`。没有安装或启用验证插件时，接口返回 `enabled: false`。验证插件不处理账户密码，也不记录验证凭证。

参考官方文档：[Turnstile](https://developers.cloudflare.com/turnstile/get-started/)、[hCaptcha](https://docs.hcaptcha.com/)、[reCAPTCHA v2](https://developers.google.com/recaptcha/docs/display)。
