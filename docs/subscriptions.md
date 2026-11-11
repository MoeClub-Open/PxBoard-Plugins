# 订阅链接

完整客户端配置、默认分流、模板编辑、预览和旧站转换见[订阅模板](subscription-templates.md)。格式支持 `aerion`（也接受 `xbclient`）、`clash`（也接受 `mihomo`、`clashmeta`、`meta`）、`sing-box`（也接受 `singbox`）和 `uri`。

用户可在“我的订阅”创建多条独立链接，分别命名、复制、重置或删除。重置后旧地址立即失效，同时解除该链接的客户端绑定；删除只影响这一条链接。原有订阅链接保持有效。通用地址根据客户端选择格式，普通浏览器默认得到 Mihomo 配置；也可使用 `format=clash`、`format=sing-box` 或 `format=uri` 指定格式。

创建时可选择绑定首次使用的客户端。该链接首次由客户端成功更新后记录请求标识中的客户端名称和主版本，允许次版本变化；其他名称或主版本的请求会被拒绝。重置链接可重新绑定。

后台“订阅与客户端 → 订阅访问”可设置浏览器访问时隐藏订阅，默认开启。按 User-Agent 关键词匹配，默认匹配 `Mozilla/`；每行可设置一个关键词，匹配不区分大小写。匹配时返回普通 404 页面，不返回订阅内容；关闭后未绑定客户端的链接可在浏览器中下载。开启客户端绑定的链接仍按关键词屏蔽浏览器。修改关键词后立即对新旧链接生效，请避免填入常用订阅客户端的标识。

客户端标识可以伪造，同一客户端在多台设备上也可能使用相同标识，绑定和浏览器判断不能证明设备身份或阻止他人冒用；部分客户端使用浏览器式标识时可能被隐藏。请像密码一样保护订阅地址，泄露后及时重置。

订阅验证默认关闭，可复用独立人机验证插件；启用后需要为每条链接单独验证，详情见[人机验证](captcha.md)。未验证的链接不显示地址，服务端也拒绝拉取；验证通过后客户端不需要每次更新都验证。

接口统一通过 `POST /rpc/v1/subscriptions/{method}` 调用：`links.list`（传入 `{"mine":true}`）列出自己的链接；`links.create` 提交 `name` 和 `bindUserAgent`，只返回记录 ID；`links.rotate` 与 `links.delete` 传入 `{"id":"..."}` 执行重置或删除。访问其他用户的链接需要管理员权限。第三方客户端拉取订阅继续使用 `GET /s/{token}`（同时保留 `/api/v1/extensions/subscriptions/s/{token}`）。

## 已登录客户端

Web 前台与原生客户端统一使用核心会话调用 `POST /rpc/v1/subscriptions/client.profile`（也可放入 `POST /rpc/v1/$batch` 与账户、插件、钱包信息合并请求）。传入可选 `knownRevision` 时，配置未变化直接返回 `unchanged: true`；完整响应包含 `revision`、`blockReason`（`""`、`no_plan`、`expired`、`traffic_exceeded`、`suspended`）、`account`、`nodes`、`routing`、`links` 与 `settings`。接口要求登录，始终返回 HTTP 200 且只包含当前账号有权使用的节点；即使套餐到期或暂无节点，账户资料与权益状态也会正常返回。

它不创建公开订阅令牌，不触发首次客户端绑定，也不改变已有链接的人机验证状态。公开链接继续使用前述验证和绑定规则。客户端身份由核心会话保护；登录协议见主项目 `docs/native-clients.md`。Aerion 输出在保留生效 Mihomo 分流与协议转换的同时，会自动移除二进制 `.mrs` 等外部 `rule-providers` 并清理悬空 `RULE-SET` 引用（将 `cn-domain` 规则集转换为内置 `DOMAIN-SUFFIX,cn` 规则），避免原生客户端同步中断。

## Aerion 与 Mihomo 分离

XBClient 使用 `format=aerion`，公开链接也接受 `flag=aerion`，未指定格式时可由 Aerion/XBClient 的 User-Agent 自动选择。Aerion 独立控制协议支持，复用现有 YAML 模板和标准协议字段构造；Flux 仅通过这个适配器输出。显式 `format=clash` 或 `flag=meta` 始终选择普通 Mihomo，声明 `types=flux` 或 Flux 能力头也不会将自定义协议混入其中。

`types` 中明确列出的协议优先于能力头，排除 Flux 时仍会过滤；`types=all` 使用所选适配器的协议范围。被过滤的协议通过 `X-PxBoard-Skipped-Protocols` 返回。模板仍在现有 Mihomo YAML 模板编辑器中统一维护，无需复制规则、代理组或模板记录。
