# PxBoard 官方插件

独立于主项目发布的业务插件与节点代理。需要同级 `../PxBoard` 公开 SDK；本地模块通过 `replace` 引用。

| 插件 | 内容 |
| --- | --- |
| branding | [站点名称、Logo、网站图标](docs/branding.md) |
| registration | [账户注册与全局注册开关](docs/registration.md) |
| access | [访问权益、配额、周期流量](docs/access-redeem.md) |
| nodes | [节点、接入令牌、配置同步与状态监控](docs/noders-management.md) |
| subscriptions | [通用订阅、多链接与客户端绑定](docs/subscriptions.md) |
| redeem | [访问权益与可选余额兑换码](docs/redeem.md) |
| commerce | [套餐、订单、余额、价格优惠](docs/commerce.md) |
| affiliate | [推广、佣金与提现](docs/affiliate.md) |
| payments | Stripe 与易支付 |
| audit | 流量审计、阻断与账户暂停 |
| support | [工单、公告和文档](docs/support.md) |
| notifications | [独立邮件投递和密码找回](docs/notifications.md) |
| oauth | [Google、GitHub、LinuxDO 登录与账号绑定](docs/oauth.md) |
| captcha | [可选人机验证：Turnstile、hCaptcha、reCAPTCHA v2](docs/captcha.md) |
| import-sspanel | [从 SSPanel 导入用户、权益、余额、套餐和订阅](docs/migration.md) |
| import-v2board | [从 V2Board 导入用户、权益、余额、套餐和订阅](docs/migration.md) |
| import-xboard | [从 Xboard 导入用户、权益、余额、套餐和订阅](docs/migration.md) |

插件声明在 `plugins/<id>/manifest.json`。每个 `cmd/<id>` 仅导入该插件的业务包，自用产物不会包含商业插件。

目前官方插件统一使用开发版本 `0.0.1`；开发、构建和部署不自动提高版本号，正式上线准备完成后由项目维护者决定何时升级。插件接口版本 `apiVersion` 单独管理，不表示插件已稳定。同版本的开发更新由插件源记录发布批次，面板可以识别并安装新批次。

构建使用 Go 1.26 或更新版本。本文所述功能组合要求核心 `0.4.0` 或更新版本，各插件的准确依赖以 `manifest.json` 为准。

```sh
go build -o bin/nodes ./cmd/nodes
go build -o bin/px-agent ./cmd/agent
```

## 安装与功能开关

模式决定默认安装组合，具体功能通过对应插件启用或停用。官方插件源的默认组合如下：

| 模式 | 默认插件 |
| --- | --- |
| 自用 | branding、nodes、subscriptions |
| 公益 / 分享 | branding、registration、nodes、subscriptions、redeem、audit |
| 商业 | branding、registration、nodes、subscriptions、commerce、payments、audit、support、notifications |

依赖插件由安装器一并安装。人机验证与推广插件在所有模式下均为可选，公益站点无需安装；商业模式也可按需安装兑换码或 OAuth。注册、兑换码和推广分别由独立插件提供，商业插件只负责套餐、订单、余额与价格优惠。停用插件会关闭对应入口和接口，已有业务数据保留。

完整功能边界、模式与节点接入方式见同级主仓库 `docs/`。主项目、官方插件和官方插件源分别存放，批量发布由独立 `PxBoard-Registry` 仓库执行。

各插件职责、可选关系与共享能力见[插件架构](docs/architecture.md)。

## 数据迁移与升级

迁移插件支持读取只读 MySQL 数据库或由 `cmd/export-migration` 生成的完整快照。默认转换用户与套餐等主要数据，节点和机器可选；提交前提供预览与冲突检查，未转换的旧表保留归档。使用方法、字段单位、套餐核对及恢复操作见 [迁移文档](docs/migration.md)。第三方客户端和 OAuth 数据不走面板导入插件，OAuth 的独立手动搬入工具见 [OAuth 文档](docs/oauth.md)。

已有 PxBoard 的旧分享码、充值卡和兑换历史通过[兑换码数据升级](docs/redeem-migration.md)搬入独立兑换码插件；旧商业插件中的推广关系、佣金及提现记录通过[推广数据升级](docs/affiliate.md#从旧商业插件拆出数据)搬入独立推广插件。外部面板的原兑换码规则目前仅归档，不会自动激活。
