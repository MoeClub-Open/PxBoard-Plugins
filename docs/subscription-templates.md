# 订阅模板

模板由 `subscriptions` 插件保存。Mihomo、sing-box 分别选择默认模板，普通用户通过同一订阅地址自动获取对应格式。通用 URI 订阅保持节点链接列表，不混入客户端配置。

## 默认内容

| 内容 | Mihomo | sing-box |
| --- | --- | --- |
| 代理入口 | 本机 mixed 端口 7890 | 本机 mixed 端口 7890 |
| 节点选择 | 手动、自动测速、故障转移 | 手动、自动测速 |
| DNS | Fake IP、代理服务器解析、境内直连解析、境外加密解析 | 带 `type` 的加密 DNS、独立节点域名解析器、DNS 路由 |
| 分流 | 局域网与中国域名/IP 直连，其余经过所选代理 | 同类分流，并支持 direct/global 模式 |
| 规则集 | MetaCubeX 的中国域名和 IP 规则集 | SagerNet 的中国域名和 IP 规则集 |
| 状态 | 保存选择及 Fake IP | 持久缓存 |

Mihomo 模板包含默认关闭的 TUN 设置，可在模板中按需启用。sing-box 默认本地代理入口，客户端可接管或自行添加 TUN 入站；不默认要求设备获得创建虚拟网卡的权限。监听只绑定本机。规则集由客户端下载并缓存，面板不代理下载这些资源。

当前内置格式已通过 Mihomo 1.19.33、sing-box 1.14.4 的原生配置检查。sing-box 使用当前 DNS 和路由结构，不在每次请求时切换回已废弃字段。自定义规则集、DNS 和其他客户端字段会保留，但仍需遵守所用客户端版本的格式。

## 管理

后台“订阅与客户端 → 订阅模板”可以添加、编辑、删除、预览和校验模板，并为每种格式设置默认模板。默认模板在列表中操作一次；“订阅内容”设置订阅名称、面板地址及 1–168 小时的刷新周期。名称留空时跟随品牌插件，未设置品牌时使用 PxBoard。

当前默认模板不能直接删除、停用或更改格式；先选择另一模板即可。恢复内置只替换指定模板的内容，不更换订阅令牌。模板保存即进行语法与引用检查，停用模板可保存未通过校验的草稿，启用前必须修正；修改默认模板内容后对下一次更新生效。预览使用示例节点和示例凭据，不读取真实用户的连接信息。面板检查 JSON/YAML、分组与策略引用、规则集引用、筛选正则和循环依赖；完整客户端配置语义仍由对应客户端检查。

订阅输出添加 `profile-title`、`profile-update-interval`、`subscription-userinfo`、下载文件名及可选 `profile-web-page-url`。订阅域名自动添加直连规则，避免代理失效时无法更新订阅。浏览器隐藏、首次客户端绑定及可选人机验证在生成模板之前校验，所有格式共用同一安全流程。

## 动态节点

模板根部的 `proxies`（Mihomo）或 `outbounds`（sing-box）保留静态项，并注入当前账户有权限且该格式支持的节点。模板没有获取其他账户节点的入口。动态节点重名或与模板策略组重名时自动加后缀，组内引用同步使用新名称。

在 Mihomo 的 `proxy-groups[].proxies` 或 sing-box 的组 `outbounds` 数组中使用字符串 `${nodes}`，展开为动态节点名称。组可以带 `include` 和 `exclude` 正则筛选；这些是模板字段，输出时移除，不交给客户端。Go 正则及常用 `/表达式/i` 写法可用，不支持的表达式在保存或导入时报告。

```yaml
proxies: []
proxy-groups:
  - name: 代理选择
    type: select
    proxies: ["${nodes}", DIRECT]
  - name: 日本
    type: select
    proxies: ["${nodes}"]
    include: "日本|Japan|JP"
    exclude: "高倍率"
    fallback: 代理选择
rules:
  - MATCH,代理选择
```

`fallback` 用于筛选后组为空的情况，可以是现有节点、另一策略组、内置直连项，也可用正则或按顺序尝试的数组。回退不能形成组间循环。没有匹配节点且没有有效回退时，生成会报错，不返回悬空引用或偷偷改为直连。Mihomo 原生 `use`、`include-all` 等规则和提供者设置可继续使用。`${siteName}` 在字符串值中替换为当前订阅名称，不以字符串拼接方式注入 YAML/JSON。

## 旧站模板

新建 Xboard / V2Board 迁移任务可以选择导入订阅模板。数据库中的 `clashmeta`、`clash`、`singbox` 会转换并保留原始内容；Mihomo 优先选择原 `clashmeta`，sing-box 选择原 `singbox`。Surge、Surfboard 等未提供输出的格式只归档。快照不含模板表时使用内置模板，文件型模板需要单独提交。

转换将旧自动填充分组改为显式节点占位符，并转换常见正则；sing-box 的旧 DNS 地址、`block` / `dns` 特殊出站和入站嗅探字段分别转成当前 DNS、路由 action 及域名解析器配置。不能转换或校验未通过的模板保留为停用草稿，并保存原因；只有有效模板会被选择为默认，其余格式使用内置模板。模板问题不阻断用户和套餐迁移。目标已有同名配置时不覆盖。

已经完成用户迁移的站点可以通过管理员接口单独搬入模板，不重新导入用户。`POST /api/v1/extensions/subscriptions/templates/import-xboard` 接收：

```json
{"sourceId":"old-site","activate":false,"templates":[{"name":"clashmeta","content":"原 YAML 文本"},{"name":"singbox","content":"原 JSON 文本"}]}
```

先保持 `activate:false`，预览并在实际客户端检查，再在后台设为默认。相同来源及名称的原始内容发生变化时接口拒绝覆盖；使用新来源标识可以并存。原始内容仅在管理员模板数据中保留，迁移快照与归档继续遵循原备份要求。

## 接口

接口均相对 `/api/v1/extensions/subscriptions`，需要管理员：

| 接口 | 用途 |
| --- | --- |
| `GET/POST /templates` | 列表和新建 |
| `PATCH/DELETE /templates/{id}` | 编辑和删除 |
| `GET /templates/options?format=clash` | 默认模板选择项 |
| `POST /templates/{id}/preview` | 示例节点预览 |
| `POST /templates/{id}/validate` | 语法和引用校验 |
| `POST /templates/{id}/default` | 设为该格式默认 |
| `POST /templates/{id}/reset` | 恢复内置内容 |
| `POST /templates/preview` | 提交 `format`、`content` 预览未保存内容 |
| `GET/PATCH /template-settings` | 默认模板和客户端信息 |
| `POST /templates/import-xboard` | 单独导入旧模板 |
