# NodeRS 配置管理与面板迁移

节点插件 1.3.0 增加 NodeRS 本地面板配置的下发与结果记录。需要使用实现 `managedConfiguration` 的 NodeRS；旧版 NodeRS 不会读取配置任务，任务会保持等待状态。节点的协议、监听、证书和用户配置仍通过原有节点接口更新。

## 创建 NodeRS 节点

先在“机器”添加服务器，并在节点表单中选择对应机器。新节点默认使用 NodeRS 和 AnyTLS；填写连接地址、连接端口、分组和流量倍率。连接端口提供给客户端，节点监听端口用于服务器实际监听；后者留空时使用连接端口。端口转发场景可分别填写两个端口。

AnyTLS 使用 TCP 和 TLS。证书方式支持：

- HTTP 验证签发：填写 TLS 域名或签发域名；域名需要解析到节点服务器，并让验证请求能访问服务器的 TCP 80 端口。未单独填写签发域名时使用 TLS 域名或连接地址。
- DNS 验证签发：选择 Cloudflare 并填写 API 令牌，或选择阿里云 DNS 并填写 AccessKey ID 与 Secret。凭据需要具备相应域名的 DNS 记录修改权限。
- 节点本地文件：填写节点服务器上的证书和私钥路径，文件应可被 NodeRS 进程读取。

签发配置保存到既有 `certConfig`，监听端口保存到 `serverPort`。已有的证书高级选项、协议参数和父子节点关系会在普通编辑时保留；修改部分证书字段只合并对应字段。NodeRS 当前会在启动或节点重建时检查签发与续期，尚未实现常驻进程的定时证书刷新，不应依赖此表单实现无人值守续期。

Mieru 可选择 TCP 或 UDP，不需要 TLS 证书。修改传输方式时会同时更新已有协议配置中对应的传输值，其余协议参数保留。Xray 和 sing-box 继续使用各自已有的协议范围及文件证书配置。

## 节点设置与订阅显示

节点插件 1.4.0 需要 PxBoard 核心 0.4.0 或更新版本。在管理员“节点设置”中，可关闭“向用户展示节点状态”；默认开启。关闭后普通用户的状态入口会隐藏，直接访问状态页面或接口也会返回不可用。管理员监控、机器心跳、节点接入、流量上报和订阅继续运行。

节点编辑中的“在订阅中显示”和“启用服务”分别控制发布与运行。关闭订阅显示不会停止已经运行的节点；停用服务不会启动另一份监听。已有的共用节点关系保留：可见的子节点仍可使用已启用父节点的运行状态，子节点自身保持停用以避免重复监听。

管理员接口以 `/api/v1/extensions/nodes` 为前缀：`GET /settings` 返回单行设置列表，`PATCH /settings` 接收 `{"userStatusEnabled":false}`，管理表单直接保存到同一路径。普通用户不能读取或修改设置。`GET /_capabilities` 仅允许核心内部调用，返回 `{"data":{"portal.status":true}}` 或对应的 `false`，供核心控制页面入口；业务接口仍会独立检查开关。

## 在后台迁移

先在目标面板导入或创建机器、节点及用户，确认对应机器拥有正确的节点和账户。机器编号可以与源面板不同，连接凭据及节点监听端口应与现有服务一致。

在源 PxBoard 的“机器”列表选择“迁移面板”，填写目标面板类型、完整地址、目标机器编号和机器密钥。提交后在“配置变更”查看结果：

| 状态 | 含义 |
| --- | --- |
| 等待节点处理 | 已保存，等待 NodeRS 读取 |
| 已应用 | NodeRS 已验证目标并保存配置，完成接管 |
| 应用失败 | NodeRS 保留或恢复原配置；查看处理结果，修正目标后再次下发 |

失败任务可以“重新下发”，生成新的配置版本。此操作沿用上一次的目标参数；需要改地址或密钥时，在机器列表重新填写“迁移面板”。已有待处理任务时不能覆盖，防止节点执行中的目标被替换。

配置变更只覆盖启动时指定的 NodeRS 配置文件，不接受任意文件路径和命令。NodeRS 在节点侧保存旧配置备份；证书、私钥及流量队列文件不会由面板复制或删除。迁移前保留源数据库、节点配置与工作目录，迁移后核对原用户连接、余额及流量基线。

## 管理接口

路径前缀为 `/api/v1/extensions/nodes`，需要管理员会话或具有写入权限的管理员访问密钥。`{id}` 是机器记录 ID，区别于用于 NodeRS 认证的数字机器编号。

| 方法与路径 | 内容 |
| --- | --- |
| `POST /machines/{id}/configuration` | 下发目标面板配置 |
| `GET /machine-configurations` | 每台机器最近一次任务的版本、目标和处理结果；不返回密钥 |
| `POST /machine-configurations/{id}/retry` | 重新下发失败任务 |

下发请求：

```json
{
  "panelType": "pxboard",
  "api": "https://panel.example.com",
  "machineId": 42,
  "key": "目标机器密钥"
}
```

`panelType` 为 `pxboard` 或 `xboard`；地址不包含认证信息、查询参数或片段。下发成功仅表示面板已保存任务，实际切换结果由节点确认。

## NodeRS 接口

以下接口使用数字机器编号，沿用 `X-Machine-Key` 认证。停用的机器不能读取任务或回报结果。握手的 `capabilities` 包含 `managedConfiguration`。

| 方法与路径 | 内容 |
| --- | --- |
| `GET /machines/{machineId}/configuration` | 获取待处理任务；没有任务或已经处理时 `data` 为 `null` |
| `POST /machines/{machineId}/configuration/status` | 确认成功或失败 |

待处理任务响应：

```json
{
  "data": {
    "revision": 1,
    "config": {
      "panel": {
        "type": "pxboard",
        "api": "https://target.example.com",
        "key": "目标机器密钥",
        "machine_id": 42
      }
    }
  }
}
```

状态请求为 `{"revision":1,"status":"applied","message":"..."}`，失败使用 `failed`。状态向原面板回报，使用原机器密钥；版本不匹配返回 409。重复确认同一终态可以安全重试，已经成功的任务不能被后续失败回报改写。任务只下发给所属机器，目标密钥不出现在管理列表或处理结果中。

## 从 Xboard 迁移

Xboard 本身没有 PxBoard 的配置接口。首次迁移可以在节点侧使用 NodeRS 的 `--migrate-panel` 命令，或安装独立的 [Xboard 迁移桥接](../tools/xboard-migration-bridge/README.md)，再使用支持它的 NodeRS。桥接通过 Xboard 原生插件机制扩展响应，不修改原控制器或其他插件。以下是它提供的协议。

原 `POST /api/v2/server/machine/nodes` 响应增加：

```json
{
  "nodes": [],
  "agent_config": {
    "managed_configuration": {
      "revision": 1,
      "config": {
        "panel": {
          "type": "pxboard",
          "api": "https://target.example.com",
          "key": "目标机器密钥",
          "machine_id": 42
        }
      }
    }
  }
}
```

其中 `nodes` 仍返回真实节点列表。扩展必须绑定当前已经通过 Xboard 机器密钥验证的机器，不能通过请求参数读取其他机器的任务。仅管理员能够写入待下发任务；不应向普通用户或公开接口提供目标机器密钥。

`POST /api/v2/server/machine/configuration/status` 接收相同的版本、状态和消息，沿用 Xboard 的 `token`、`machine_id` 认证。源站应只下发待处理任务，成功或失败后停止下发该版本。没有此扩展的原有 Xboard 响应继续正常工作。

CLI 的目标文件使用普通 NodeRS TOML 配置，包含 `[panel]` 的 `type`、`api`、`key` 和 `machine_id`。节点侧的验证、备份、最后流量回报和失败恢复规则见 NodeRS 的 `docs/zh/pxboard.md`。不能在未排空旧流量的情况下直接把旧流量队列改名为目标队列；两个面板的回报确认彼此独立。
