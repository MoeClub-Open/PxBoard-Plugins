# PxBoard 官方插件

独立于主项目发布的业务插件与节点代理。需要同级 `../PxBoard` 公开 SDK；本地模块通过 `replace` 引用。

| 插件 | 内容 |
| --- | --- |
| access | 访问权益、配额、周期流量 |
| nodes | 节点、接入令牌、配置同步 |
| subscriptions | Clash、sing-box、通用订阅 |
| sharing | 分享与邀请注册 |
| commerce | 套餐、订单、余额、优惠、返佣 |
| payments | Stripe 与易支付 |
| audit | 流量审计、阻断与账户暂停 |
| support | 工单、公告、文档与邮件 |
| import-sspanel | 从 SSPanel 导入用户、权益、余额、套餐和订阅 |
| import-v2board | 从 V2Board 导入用户、权益、余额、套餐和订阅 |
| import-xboard | 从 Xboard 导入用户、权益、余额、套餐和订阅 |

插件声明在 `plugins/<id>/manifest.json`。每个 `cmd/<id>` 仅导入该插件的业务包，自用产物不会包含商业插件。

```sh
go build -o bin/nodes ./cmd/nodes
go build -o bin/px-agent ./cmd/agent
```

商业接口、金额与期限、订单重试及推广申请规则见[商业运营文档](docs/commerce.md)。完整功能边界、外部验证限制、节点接入方式见同级主仓库 `docs/`。批量发布由独立 `PxBoard-Registry` 仓库执行。

迁移插件要求核心 0.2.0+，支持读取只读 MySQL 数据库或由 `cmd/export-migration` 生成的完整快照。默认转换用户与套餐等主要数据，节点和机器可选；提交前提供预览与冲突检查，未转换的旧表保留归档。使用方法、字段单位、套餐核对及恢复操作见 [迁移文档](docs/migration.md)。
