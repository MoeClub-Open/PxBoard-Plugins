# NodeRS 配置管理与面板迁移

节点插件支持 NodeRS 本地面板配置的下发与结果记录。使用原仓库的 [NodeRS v0.1.50 或更新版本](https://github.com/MoeclubM/NodeRS/releases)，同一安装包包含 Xboard、PxBoard 统一 `/rpc/v1` 接口与 `managedConfiguration` 支持；此前安装组织适配版的节点，按 [发布源切换说明](https://github.com/MoeclubM/NodeRS/blob/main/docs/zh/install.md#从组织适配版升级) 完成一次升级。旧版 NodeRS 不会读取配置任务，任务会保持等待状态。

## 创建 NodeRS 节点

先在“服务器管理”添加服务器，并在节点表单中选择所属服务器。新节点默认使用 NodeRS 和 AnyTLS；填写连接地址、连接端口、分组和流量倍率。连接端口提供给客户端，节点监听端口用于服务器实际监听；后者留空时使用连接端口。端口转发场景可分别填写两个端口。

在“节点管理”的“节点分组”中创建和重命名分组，再在节点、访问账户、套餐或兑换码中选择。更改名称不会更改现有关联；关闭分组的新增选择不会撤销已经分配的权限，默认分组始终可用。用户只能查看自己账户可用的分组。已从旧面板导入但未建立名称的分组可以使用 [迁移工具](migration.md#恢复已导入的节点分组名称) 恢复。

AnyTLS 使用 TCP 和 TLS。证书方式支持：

- HTTP 验证签发：填写 TLS 域名或签发域名；域名需要解析到节点服务器，并让验证请求能访问服务器的 TCP 80 端口。未单独填写签发域名时使用 TLS 域名或连接地址。
- DNS 验证签发：选择 Cloudflare 并填写 API 令牌，或选择阿里云 DNS 并填写 AccessKey ID 与 Secret。凭据需要具备相应域名的 DNS 记录修改权限。
- 节点本地文件：填写节点服务器上的证书和私钥路径，文件应可被 NodeRS 进程读取。

签发配置保存到既有 `certConfig`，监听端口保存到 `serverPort`。已有的证书高级选项、协议参数和父子节点关系会在普通编辑时保留；修改部分证书字段只合并对应字段。NodeRS 当前会在启动或节点重建时检查签发与续期，尚未实现常驻进程的定时证书刷新，不应依赖此表单实现无人值守续期。

Mieru 可选择 TCP 或 UDP，不需要 TLS 证书。修改传输方式时会同时更新已有协议配置中对应的传输值，其余协议参数保留。Xray 和 sing-box 继续使用各自已有的协议范围及文件证书配置。

## 服务器监控与订阅显示

状态监控以服务器（`machines`）为单位，汇总服务器心跳、CPU、内存、磁盘、交换空间与所属运行节点的去重在线账户数；已离线或停用的服务器不会继续展示旧负载。在管理员“服务器监控”底部的“用户前台展示”设置中，可关闭“向用户展示服务器状态”；默认开启。关闭后普通用户的状态入口会隐藏，直接访问状态页面或接口也会返回不可用。管理员还可以选择用户看到的地区、在线状态、运行节点数、CPU、内存、磁盘、交换空间、在线账户数和最近心跳；未选择的项目不会从用户接口返回，普通用户仅能查看包含自己可用节点的启用服务器。

节点编辑中的“在订阅中显示”和“启用独立服务”分别控制发布与运行。关闭订阅显示不会停止已经运行的节点；停用独立服务不会启动另一份监听。已有的共用节点关系保留：可见的子节点使用主节点的服务，子节点自身不单独启动，避免重复监听。节点列表只显示主节点，点击“连接地址”可查看或修改它的附加地址；订阅会为每个可见地址生成独立入口，并使用主节点的协议与认证配置；地址的分组仍须同时满足主节点和地址自身的访问条件。

管理员接口统一通过 `POST /rpc/v1/nodes/{method}` 调用：`settings.list` 返回单行设置列表，`settings.update` 接收 `{"userStatusEnabled":false}`。普通用户不能读取或修改设置，前台监控读取 `nodes/status.list` 与 `nodes/status.display`。`/_capabilities` 仅允许核心运行时内部调用，返回 `{"data":{"portal.status":true}}` 或对应的 `false`，供核心控制页面入口；业务接口仍会独立检查开关。

## 在后台迁移

先在目标面板导入或创建服务器、节点及用户，确认对应服务器拥有正确的节点和账户。服务器编号可以与源面板不同，连接凭据及节点监听端口应与现有服务一致。

在源 PxBoard 的“服务器管理”列表选择“迁移面板”，填写目标面板类型、完整地址、目标服务器编号和目标服务器密钥。提交后在“配置变更”查看结果：

| 状态 | 含义 |
| --- | --- |
| 等待服务器处理 | 已保存，等待 NodeRS 读取 |
| 已应用 | NodeRS 已验证目标并保存配置，完成接管 |
| 应用失败 | NodeRS 保留或恢复原配置；查看处理结果，修正目标后再次下发 |

失败任务可以“重新下发”，生成新的配置版本。此操作沿用上一次的目标参数；需要改地址或密钥时，在机器列表重新填写“迁移面板”。已有待处理任务时不能覆盖，防止节点执行中的目标被替换。

配置变更只覆盖启动时指定的 NodeRS 配置文件，不接受任意文件路径和命令。NodeRS 在节点侧保存旧配置备份；证书、私钥及流量队列文件不会由面板复制或删除。迁移前保留源数据库、节点配置与工作目录，迁移后核对原用户连接、余额及流量基线。

## 管理接口

过程统一通过 `POST /rpc/v1/nodes/{method}` 调用，需要管理员会话或具有写入权限的管理员访问密钥。请求中的 `id` 是机器或任务记录 ID，区别于用于 NodeRS 认证的数字机器编号。

| 过程与参数 | 内容 |
| --- | --- |
| `machines.configuration`（含 `id`） | 下发目标面板配置 |
| `machineConfigurations.list` | 每台机器最近一次任务的版本、目标和处理结果；不返回密钥 |
| `machineConfigurations.retry`（含 `id`） | 重新下发失败任务 |

下发请求：

```json
{
  "id": "机器记录ID",
  "panelType": "pxboard",
  "api": "https://panel.example.com",
  "machineId": 42,
  "key": "目标机器密钥"
}
```

`panelType` 为 `pxboard` 或 `xboard`；地址不包含认证信息、查询参数或片段。下发成功仅表示面板已保存任务，实际切换结果由节点确认。

## NodeRS 接口

节点端与管理端、客户端共用同一套 `/rpc/v1` 协议，路径为 `POST /rpc/v1/nodes/{method}`，请求头使用 `Authorization: Bearer pxm_<machineId>.<key>`（也兼容直接传 `<key>` 并在请求体附带 `machineId`）。机器凭证角色仅允许调用 `nodes` 服务的 `server.*` 过程；停用的机器不能读取任务或回报结果。

| 过程 | 内容 |
| --- | --- |
| `server.bootstrap` | 机器握手，返回 `machineId`、名称、`pullIntervalSeconds`、`pushIntervalSeconds` 与 `capabilities` |
| `server.pull` | 合并拉取整机快照（传入 `knownRevision`），一次返回 `revision`、`unchanged`、全部节点 `config` 与 `users`、轮询间隔，以及待处理的 `managedConfiguration` |
| `server.traffic` | 上报单节点（`nodeId/batchId/accounts`）或多节点（`reports`）增量流量批次 |
| `server.telemetry` | 上报主机监控状态 `status` 及单节点或多节点在线账户列表 |
| `server.ackConfig` | 确认 `managedConfiguration` 应用成功（`applied`）或失败（`failed`） |

`server.pull` 中的 `managedConfiguration` 字段结构：

```json
{
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
```

`server.ackConfig` 请求为 `{"machineId":42,"revision":1,"status":"applied","message":"..."}`，失败使用 `failed`。状态向原面板回报，使用原机器密钥；版本不匹配返回 409。重复确认同一终态可以安全重试，已经成功的任务不能被后续失败回报改写。任务只下发给所属机器，目标密钥不出现在管理列表或处理结果中。

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
