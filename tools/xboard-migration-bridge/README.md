# Xboard NodeRS 迁移桥接

这是安装到源 Xboard 的独立功能插件，用于让已经通过机器密钥认证的 NodeRS 接收迁移目标配置。PxBoard 核心和用户数据导入插件不依赖此桥接。

适用于本仓库参考的 Xboard 分支：Laravel 12、`App\Services\Plugin\PluginManager` 插件管理器，以及 `POST /api/v2/server/machine/nodes` 机器接口。NodeRS 必须包含 `agent_config.managed_configuration` 支持。旧版 NodeRS 会忽略扩展字段，无法自动迁移。

## 安装

将整个 `xboard-migration-bridge` 目录复制到服务器。以下 `/srv/Xboard` 为实际源面板根目录，PHP 使用源面板相同的版本和运行环境。

```sh
php /srv/xboard-migration-bridge/manage.php /srv/Xboard install
```

安装器把 `PxboardMigrationBridge/` 复制到源站 `plugins/`，调用原插件管理器安装并启用。已存在目标插件目录时拒绝覆盖。源 Xboard 控制器、路由文件、已有插件文件与业务表不被修改；只新增此插件的登记记录。

使用 PHP 服务的系统用户执行管理命令，使网页进程能够读取任务。容器部署在运行 Xboard 的容器中执行，并确保 `plugins/` 与 `storage/` 持久保存。使用 Octane、Swoole 或其他常驻 PHP 进程时，在安装和停用后重启相应网页服务，确保路由与中间件立即更新。

## 准备和下发

先在目标 PxBoard 完成用户、套餐、机器和节点导入，确认目标机器的节点、端口、证书与用户可用。桥接只下发 NodeRS 本地 `[panel]` 配置，不会自动导入源站数据库，也不会复制证书或私钥。

创建仅服务用户可读的目标文件，例如 `/srv/private/pxboard-machine-7.json`：

```json
{
  "panel": {
    "type": "pxboard",
    "api": "https://new-panel.example.com",
    "key": "目标 PxBoard 机器密钥",
    "machine_id": 7
  }
}
```

```sh
chmod 600 /srv/private/pxboard-machine-7.json
php /srv/xboard-migration-bridge/manage.php /srv/Xboard queue 7 /srv/private/pxboard-machine-7.json
php /srv/xboard-migration-bridge/manage.php /srv/Xboard status 7
```

`queue` 的数字是**源 Xboard 机器编号**，目标文件的 `machine_id` 是目标机器编号，两者可以不同。只为明确指定的源机器生成任务，其他机器继续得到原响应。目标地址和密钥保存在源服务器私有存储中，不通过命令参数传递。

状态为 `pending`、`applied` 或 `failed`。`status` 输出版本、状态、目标地址、机器编号及处理结果，隐藏目标密钥。失败后可修正目标文件并再次 `queue`，新任务版本递增。已有待处理任务时拒绝覆盖，以免节点执行被并发替换的目标。

NodeRS 读取任务后验证目标、回报旧流量、备份本地配置并切换，最后向原 Xboard 回报结果。源站只下发待处理版本，收到成功或失败后停止下发。切换失败时节点保留或恢复源配置；具体节点侧行为以所安装 NodeRS 版本为准。

## 鉴权与接口

桥接在原机器控制器成功返回后附加字段，保留节点列表、同步间隔和其他扩展字段。它再次核对当前请求的 `machine_id`、`token` 与启用状态，任务文件始终按认证通过的机器编号读取。

```json
{
  "nodes": [],
  "base_config": {},
  "agent_config": {
    "managed_configuration": {
      "revision": 1,
      "config": {
        "panel": {
          "type": "pxboard",
          "api": "https://new-panel.example.com",
          "key": "目标机器密钥",
          "machine_id": 7
        }
      }
    }
  }
}
```

NodeRS 使用同样的源机器凭据调用 `POST /api/v2/server/machine/configuration/status`：

```json
{"revision":1,"status":"applied","message":"configuration applied"}
```

失败使用 `failed`。不匹配的版本或冲突终态返回 409；重复确认同一终态可重试。结果消息中的源与目标机器密钥会替换为 `[redacted]`。没有面向普通用户的管理入口，任务创建仅通过拥有服务器权限的 CLI 执行。

任务位于 `storage/app/private/pxboard-migration-bridge/<源机器编号>.json`，目录权限为 0700、文件权限为 0600。该目录不应映射到公开网站；备份时按密钥资料保存。桥接文件读取故障时保留原节点响应，避免影响现有节点获取配置。

## 停用与回退

```sh
php /srv/xboard-migration-bridge/manage.php /srv/Xboard disable
```

停用后不再附加任务，新状态接口返回 404；常驻网页进程需要重启。原节点接口和已有插件继续工作。任务文件与状态保留，重新启用使用：

```sh
php /srv/xboard-migration-bridge/manage.php /srv/Xboard enable
```

要撤去插件登记，执行 `uninstall`；私有任务文件和源业务数据保留。卸载或停用源桥接不会将已经迁走的节点自动迁回。节点回退应通过已接管的 PxBoard 下发原面板配置，或在节点执行 NodeRS 的配置迁移命令；保留原机器密钥、配置备份与工作目录，核对流量回报后再完成回退。

不要在任务尚可能被节点读取时删除或覆盖待处理记录。跨机器演练只对选定机器下发，保持其他机器配置与服务不变。
