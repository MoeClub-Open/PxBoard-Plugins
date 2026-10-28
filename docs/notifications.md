# 邮件通知

`notifications` 是独立的邮件投递插件。无需安装工单或商业插件即可提供密码找回；安装相应业务插件后，也会接收订单开通和工单回复通知。邮件服务不可用不会删除工单或订单。

在“邮件通知”填写 SMTP 服务器、端口、发件地址和面板公开地址。587 端口使用 STARTTLS，465 端口直接使用 TLS。服务器证书必须有效；有登录认证要求时填写用户名和密码。更新配置时密码留空会保留原值，查询配置不会返回密码。

面板公开地址用于生成找回密码链接，例如 `https://panel.example.com`。启用有效邮件配置后，登录页显示找回密码入口。找回密码邮件中的链接有效期为 30 分钟；在队列里已经过期的找回邮件不再发送。

订单以订单编号、工单回复以回复编号去重，其他通知以事件编号去重；重复收到通知不会覆盖已有记录，也不会把已发送邮件重新放入队列。邮件在后台逐封发送，失败时按逐渐延长的间隔重试，最多自动尝试 5 次，也可在投递记录中手动重试。关闭邮件通知时保留队列，重新启用后继续处理。

投递记录分页显示并可按状态筛选，不向界面返回纯文本或 HTML 正文；成功或过期后会同时移除两种正文。SMTP 接收时连接中断可能使发送端无法确认是否已送达；重试仍可能产生重复邮件。每条通知使用固定的 Message-ID，便于邮件服务识别。

其他可信插件可声明 `emit:notification.requested` 权限并发送同名事件，数据包含 `userId`、`subject` 和纯文本 `body`。邮件插件根据用户 ID 查找收件地址；重复事件以事件 ID 去重。插件启用时才接收新事件；停用期间不会补收以前未分配的事件。

## 邮件模板

在“邮件通知 → 邮件模板”编辑找回密码、订单开通、工单回复和通用通知四种模板。主题和纯文本正文分别保存；HTML 外观可编辑或留空，留空时发送纯文本。默认 HTML 使用 `{{.Content}}` 展示已渲染正文，通常只修改“正文”即可同时更新两种格式。列表提供恢复默认，不删除已生成的邮件记录。

编辑器支持按光标位置插入变量，并使用示例用户预览纯文本和 HTML。预览不读取实际用户或重置令牌，不入队、不发送邮件；HTML 预览在禁用脚本的独立框架中显示。模板语法遵循 Go `text/template` / `html/template`，例如 `{{.SiteName}}`、`{{.UserName}}`；HTML 自动转义变量内容。未知变量、空主题和正文、主题换行会阻止保存。找回密码的纯文本正文必须保留 `{{.ResetURL}}`。

| 用途 | 专用变量 |
| --- | --- |
| 找回密码 | `ResetURL`、`ExpiryMinutes` |
| 订单开通 | `OrderNumber`、`OrderID`、`PlanName` |
| 工单回复 | `TicketSubject`、`TicketID` |
| 通用通知 | `Subject`、`Message` |

共有变量包括 `SiteName`、`UserName`、`Email`、`PublicURL`、`ActionURL`、`ActionLabel`。HTML 另可使用渲染后的 `Subject` 和 `Content`。站点名称读取可选品牌插件，未安装时使用 PxBoard；通知插件不要求安装商业或用户服务插件。

邮件进入队列时确定模板和内容，修改模板只影响此后生成的邮件。重试继续使用原有内容和 Message-ID，避免同一通知因模板修改而改变。HTML 邮件使用标准 `multipart/alternative`，保留纯文本版本；UTF-8 内容使用 quoted-printable 编码。

管理员接口均相对 `/api/v1/extensions/notifications`：

| 接口 | 用途 |
| --- | --- |
| `GET/PATCH /mail` | SMTP 单例配置；GET 返回隐藏密码的单条记录数组 |
| `GET /templates` | 模板元信息列表 |
| `GET/PATCH /templates/{id}` | 读取或保存 `subject/text/html`，读取附带变量定义 |
| `POST /templates/{id}/preview` | 预览提交的内容，返回 `subject/text/html` |
| `POST /templates/{id}/reset` | 删除覆盖配置并恢复内置内容 |
| `GET /notifications?limit=12&offset=0&status=failed&q=...` | 按页读取投递记录和总数 |

## 从旧用户服务升级

旧版 `support` 保存了邮件配置、邮件记录和未完成的通知事件。核心 0.4.11 会阻止旧版用户服务在完成迁移之前升级。升级前备份完整数据目录，准备新版核心、`support` 与 `notifications` 的插件声明和对应系统的可执行文件，然后停止面板。使用 Node.js 24 或更新版本先预览，再写入：

```sh
node tools/migrate-notifications.ts --database /绝对路径/pxboard.db
node tools/migrate-notifications.ts --database /绝对路径/pxboard.db --apply
```

工具生成独立数据库备份，将配置和记录复制到新插件，并将旧用户服务尚未处理的订单、工单和找回密码事件转交新插件；旧记录不删除，已发送记录不会重新发送。若目标中已有不同内容的同名记录，工具会停止等待人工核对。

保持面板停止，用新版核心的离线安装命令安装两个插件并启用邮件通知，再启动面板：

```sh
pxboard -data /数据目录 -install-local /插件声明/support/manifest.json -binary /插件程序/support
pxboard -data /数据目录 -install-local /插件声明/notifications/manifest.json -binary /插件程序/notifications
pxboard -data /数据目录 -enable-plugin notifications
```

本地安装保留 `support` 原有启用状态；新邮件插件通过最后一条命令启用。启动后可从插件中心选择原在线来源重新安装这两个相同版本，恢复在线更新来源，然后检查配置和投递记录。迁移完成到新版插件安装完成之间不要启动旧版用户服务，否则可能重复投递或遗漏新事件。工单附件仍在原插件目录，需要与数据库一起备份。
