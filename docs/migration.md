# 从 SSPanel、V2Board 和 Xboard 迁移

迁移插件将用户、现有访问权益、余额、套餐和订阅转换为 PxBoard 数据。未转换的旧表与原字段保留在迁移快照和归档中，不会作为新业务自动运行。

共享迁移入口和官方插件当前均处于 `0.0.1` 开发版本。安装包含此功能的最新核心和插件源构建；插件接口版本为 `1.0`。

## 安装与选择来源

| 来源 | 插件 ID | 主要数据表 |
| --- | --- | --- |
| SSPanel | `import-sspanel` | `user`、`product` / `shop` / `new_shop`、`link` |
| V2Board | `import-v2board` | `v2_user`、`v2_plan`，节点按协议分表 |
| Xboard | `import-xboard` | `v2_user`、`v2_plan`，可选统一节点表与机器表 |

在插件中心安装对应来源的插件。`access`、`commerce`、`subscriptions` 是必要依赖；勾选“同时导入节点和机器”时还需要安装并启用 `nodes`。只有管理员可以创建任务、预览、提交和查看原始归档。

安装并启用迁移插件后，在后台“数据迁移”选择来源面板；各来源共用操作界面，任务和权限仍分别由对应插件管理。创建任务时选择快照文件或只读 MySQL 数据库，再填写对应信息。

推广返佣由独立 `affiliate` 插件提供，兑换功能由独立 `redeem` 插件提供；迁移用户、套餐和钱包余额不要求安装这两个插件。需要原推广关系和佣金时，先安装并启用 `affiliate`，再在新任务中选择“同时导入推广关系与佣金”。选择后目标插件缺失或停用会被核心预览检查阻断。

不同分支可能修改字段含义、密码算法或套餐结构。预览报告中的阻断问题必须处理完毕才能提交；不能识别的关键数据不会用默认值代替后继续导入。该功能不承诺适配所有历史分支。

Xboard / V2Board 新建迁移任务默认选择导入后台入口和订阅地址。旧站明确设置的后台入口会在商业模式使用；目标已有不同的自定义入口或订阅地址时，预览会阻止覆盖。提交时路由和用户数据在同一事务中写入。未明确设置后台入口时请手动设置。订阅域名可包含多个地址和路径前缀；动态地址无法安全转换时会阻止导入。请将每个订阅域名的 DNS 和反向代理指向新面板，并放行订阅路径。已完成的迁移不要重新导入：在后台的订阅地址设置中填写旧站地址，并在站点设置中填写旧后台入口即可。

## 导入范围

默认转换以下内容：

- 用户邮箱、名称、角色、停用状态、创建时间、原登录密码编码和已识别的动态验证码密钥。
- 连接 UUID、SSPanel 独立连接密码、限速、设备数限制、分组、访问等级和到期时间。
- 当前流量额度、已用上传及下载流量、重置方式和重置时区。
- 用户钱包余额和个人折扣；选择推广导入后，另行转换 V2Board / Xboard 可用佣金余额、推荐关系和个人推广规则。
- 套餐名称、价格、周期、流量、分组、限速和原套餐内容。
- 原订阅令牌。

节点和机器是可选项。历史订单、支付配置、优惠码、充值卡、审计规则、旧页面设置及其他未转换内容只归档，不会重放扣款、开通权益或自动启用。

推广导入默认关闭。源用户存在非零可用佣金时，未选择推广导入会明确阻断，不能把尚可提取的金额只放进归档。没有可用佣金时可以只迁移主要账户业务，原推广关系、规则及历史购买标记留在原始归档中，后续安装推广插件不会自动恢复这些数据。

SSPanel 原生已结算返利包含在用户钱包余额中，不再重复生成推广佣金。选择推广导入时，转换 `ref_by` 关系及已激活/已过期订单的购买历史；原返利日志仍只归档。

三来源的原邀请码和兑换码目前均仅归档，不能直接在 PxBoard 使用；需要在 `redeem` 中按可表达的规则重新创建。模板共享使用次数、跨模板个人限用、随机奖励等规则不能用单码次数替代，也不会静默放宽后启用。已迁移用户余额与套餐权益不依赖这些原码再次兑换。原版外的 OAuth、客户端扩展仍按各自模块单独接入。

SSPanel 商品默认停售并标记为待核对，完整商品内容、原记录和购买限制仍然保留。V2Board / Xboard 分别保留新购、显示及续费开关；隐藏套餐不会出现在公开目录，原套餐用户可依照原续费开关续费。限售规则尚不能直接执行时，对应套餐停售并标记待核对。核对原规则、期限和访问权益后，再编辑套餐并决定是否上架。

迁移器针对各来源面板的原生数据结构。自建客户端鉴权、OAuth 绑定等外部扩展不会自动转换或启用，其原表仍完整归档；这些扩展应通过各自的目标模块单独接入，并明确核对用户对应关系。

## 准备数据

先保留源站完整数据库备份及配置文件，并备份目标 PxBoard 的整个数据目录。迁移快照用于数据转换，不是可直接恢复源面板的 SQL 备份；它不包含数据库索引、触发器或源程序文件。

最后一次导出之前，停止源站继续接收注册、购买和余额变更，等待正在开通的订单及节点流量回报处理完成。导出后的源站新增变更不会进入已经冻结的快照。

预览发现 V2Board / Xboard 的开通中订单，或 SSPanel 的待激活订单时会阻断，避免把已付款但尚未发放的权益遗漏在迁移边界之外。旧待付款订单只归档，不会成为可继续支付的 PxBoard 订单。

以下内容通常不在数据库内，需要单独确认：

| 内容 | 需要处理的事项 |
| --- | --- |
| 源面板时区 | 填写实际时区，例如 `Asia/Shanghai`；用于解释无时区的日期及流量重置时间 |
| SSPanel 全局密码算法和盐 | 从源配置取得 `pwdMethod` 和 `salt` 等对应值，不能从不可逆密码编码中推导 |
| V2Board / Xboard 系统重置方式 | 套餐设置为“跟随系统”时，需要填写源站实际的系统设置 |
| 证书、私钥和证书路径 | 文件通常位于节点服务器，不在面板数据库；迁移引用后仍需核对路径与文件 |
| Redis、缓存和会话 | 不作为登录会话迁移；用户迁移后重新登录 |
| 支付与邮件的外部设置 | 按 PxBoard 的渠道和回调地址重新配置，再决定启用 |

## 提供数据库或快照

每个任务只选择一种来源方式。

### 直接连接数据库

创建任务时填写“只读 MySQL 连接”、来源标识和源数据库时区。“只读 MySQL 连接”使用 Go MySQL 驱动的连接字符串格式：

```text
px_migration:password@tcp(db.example.com:3306)/panel_database?charset=utf8mb4&tls=true
```

连接从运行迁移插件的服务器发起。数据库账号应只具备读取源数据库的权限。导出使用只读、可重复读取的事务；目前要求源数据表使用 InnoDB。遇到其他存储引擎时，会停止导出，可先将数据库备份恢复到单独的 InnoDB 数据库再读取。

### 先导出快照

在本仓库构建导出工具：

```sh
go build -o bin/export-migration ./cmd/export-migration
```

Linux 示例：

```sh
export PX_MIGRATION_DSN='px_migration:password@tcp(127.0.0.1:3306)/panel_database?charset=utf8mb4'
./bin/export-migration \
  --source xboard \
  --source-id old-production \
  --timezone Asia/Shanghai \
  --out /srv/migration/old-production.json
unset PX_MIGRATION_DSN
```

PowerShell 示例：

```powershell
$env:PX_MIGRATION_DSN = 'px_migration:password@tcp(127.0.0.1:3306)/panel_database?charset=utf8mb4'
.\bin\export-migration.exe --source sspanel --source-id old-production --timezone Asia/Shanghai --out C:\Migration\old-production.json
Remove-Item Env:PX_MIGRATION_DSN
```

`--source` 可取 `sspanel`、`v2board` 或 `xboard`。输出文件不会覆盖已有文件。将生成的快照放到运行 PxBoard 的服务器上，在任务的“快照文件路径”填写该服务器可读取的绝对路径；这不是浏览器本地文件上传。

`sourceId` 标识一个确定的来源数据库，只使用小写字母、数字、下划线或连字符。对同一来源重试时保持不变，不要通过更换标识绕过重复导入检查。使用快照文件时，任务采用文件内的来源标识和时区。

## 预览与提交

创建任务后执行“预览”，再打开“迁移报告”。报告列出原始记录数、完整归档数、各类转换记录数、仅归档的表、需要核对的事项和阻断问题。

预览读取数据后，会在插件持久状态目录保存冻结快照。后续重试读取同一快照，不会重新读取持续变化的源数据库。数据库连接信息在快照准备成功后从任务记录中移除。

只有 `ready` 状态可以提交。提交前再次检查目标冲突和暂存记录是否完整；用户、密码信息和业务记录在同一个数据库事务中写入，失败会整体回滚。提交不覆盖目标已有用户或记录，也不会发出历史订单开通事件。

| 状态 | 含义与操作 |
| --- | --- |
| `created` | 任务已创建，尚未预览 |
| `preparing` | 正在准备或检查数据；进程中断后可重新预览 |
| `ready` | 预览完成且未发现阻断问题，可以核对报告后提交 |
| `blocked` | 有数据问题、重复内容、缺少目标插件或暂存记录不完整；查看报告后处理 |
| `importing` | 提交中；进程中断或页面未收到结果时，重新预览可恢复检查结果 |
| `committed` | 已完成；再次提交同一任务不会重复写入 |
| `failed` | 读取、连接或写入出错；查看报告，排除原因后重试预览 |

任务参数和来源选择不能在原任务内编辑。需要更正密码盐、来源文件或转换选项时，创建新任务；只要原任务未提交，暂存内容不会占用正式业务记录。已经提交的来源不能通过创建新任务再次导入同一批记录。

目标已有相同邮箱、用户标识、连接凭据、订阅令牌或访问账户时会阻断。原数字账户编号也保留，因此将多个旧面板合并到同一个目标时，编号可能冲突；`sourceId` 前缀不能消除这类编号冲突。应先确定合并方案，不要修改快照来跳过检查。

## 用户与权益字段

| 业务内容 | SSPanel | V2Board / Xboard | PxBoard 处理 |
| --- | --- | --- | --- |
| 登录密码 | `pass`，必要时使用源全局算法和盐 | `password`、`password_algo`、`password_salt` | 保留原编码；支持 bcrypt、Argon2i/id、已识别的 MD5 / SHA256 / SHA3 带盐规则；不能识别时阻断 |
| 动态验证码 | `ga_enable`、`ga_token`，或原生 `mfa_devices` 的 TOTP 设备 | 已识别的 `totp_secret` / `two_factor_secret` 等字段 | 保留密钥；缺少或无法识别有效密钥时阻断，不自动关闭验证；只有不受支持的 FIDO 等设备时阻断 |
| 连接凭据 | `uuid` 与 `passwd` 分开处理 | `uuid` | 不重置原 UUID；SSPanel 连接密码单独保留 |
| 账号状态 | `is_banned`、`is_shadow_banned`、旧 `enable` | `banned` 等字段 | 保留停用或暂停语义 |
| 余额 | `money`，单位元 | `balance`，单位分 | 精确转为整数分，作为迁移期初余额写入一次 |
| 佣金 | 已结算金额包含在原钱包余额中 | `commission_balance`，单位分 | 选择推广导入后写入 `affiliate/commissions` 作为期初可用佣金；不重放旧佣金日志；未选但金额非零会阻断 |
| 个人折扣与返佣 | 推广关系按选项转换，其余原始记录归档 | `discount`、`commission_rate`、`commission_type` | 个人折扣写入 `commerce/customers`；返佣比例、方式及购买历史写入可选 `affiliate/profiles` |
| 当前流量 | `u`、`d`、`transfer_enable`，单位字节 | 同名用户字段，单位字节 | 保留已用流量基线与额度，上传和下载原值也保留 |
| 节点权限 | `node_group`、`class`、`class_expire` | `group_id`、套餐关系 | 保留来源分组；SSPanel 普通用户包含公共组 0 与自身组，管理员保留跨组权限 |
| 限速和设备数 | `node_speedlimit`、`node_iplimit` | `speed_limit`、`device_limit` | 保存对应账户限制 |
| 到期 | `expire_in`、`class_expire` | `expired_at` | 依据源时区转为明确时间；等级到期独立保留 |
| 订阅令牌 | `link.userid`、`link.token` | `v2_user.token` | 令牌不变；Xboard / V2Board 可选择同时导入原订阅地址 |

流量额度为 0 表示没有可用额度，不会变成无限流量。PxBoard 使用独立的 `unlimited` 标志表达不限流量，导入账户默认不启用此标志。

V2Board / Xboard 的 `expired_at=null` 表示长期；数值 `0` 表示已到期，两者分别保留，不会把已到期用户变成长期账户。

流量重置保留日历规则和时区。SSPanel 使用 `auto_reset_day`、`auto_reset_bandwidth` 等字段；V2Board / Xboard 支持每月 1 日、按到期日每月、从不、每年 1 月 1 日、按到期日每年五种方式。源字段提供 `next_reset_at` 时保留下一次重置时间。套餐跟随系统设置却未提供实际系统重置方式时，预览会阻断。

个人折扣在创建新套餐订单时从原价扣减；目标订单保留折扣金额。返佣方式为“跟随系统”时，使用独立推广插件的比例及“仅首次购买返佣”选项，需要与源站实际设置核对。V2Board / Xboard 原个人 `commission_rate` 为零或未设置时表示继承，因此不写 `commissionPercent`；新推广插件显式 `commissionPercent=0` 则表示不返佣，两者不能混淆。源站全局推广比例、冻结周期及其他系统设置不会自动覆盖目标设置。原始订单仍只归档，购买历史标记只用于防止给老用户再次发放首次购买佣金。

### 已完成旧版 PxBoard 导入的目标

已经使用旧版插件完成导入的目标，不重新导入用户或重复写入期初余额。使用 [一次性推广数据转换工具](../tools/migrate-commerce-affiliate.ts) 把旧商业插件中的推广数据搬到独立插件；工具需要 Node.js 24 或更新版本。

| 旧位置 | 新位置 | 转换规则 |
| --- | --- | --- |
| `commerce/referrals` | `affiliate/referrals` | 原标识及全部字段原样保留 |
| `commerce/commissions` | `affiliate/commissions` | 原佣金标识、金额、状态、来源和时间不变，不重复入账 |
| `commerce/withdrawals` | `affiliate/withdrawals` | 原记录保留；已存在同金额钱包账本的处理中钱包转账改为已批准，防止再次入账 |
| `commerce/customers` 的推广字段 | `affiliate/profiles`，标识为 `userId` | 移出 `commissionPercent`、`commissionMode`、`hasPriorPurchase`，保留个人折扣及其他商业字段；旧零个人比例转换为继承 |
| `commerce/settings` 的推广字段 | `affiliate/settings/settings` | 仅搬 `commissionPercent`、`commissionHoldDays`、`commissionFirstPurchaseOnly` |

先检查，再停止目标 PxBoard 并执行写入：

```sh
node tools/migrate-commerce-affiliate.ts --database /srv/pxboard/data/pxboard.db
node tools/migrate-commerce-affiliate.ts --database /srv/pxboard/data/pxboard.db --apply
```

写入前生成独立数据库备份，变更在一个事务中完成。目标已有不同内容时整批拒绝；目标已有相同内容时不重复创建，重复运行不会重复发放余额或佣金。用户标识、访问账户、套餐、订阅和钱包账本不搬家。完成后安装并启用 `affiliate`，核对佣金和提现；旧商业推广路由不再保留兼容入口。

旧 PxBoard 的 `sharing.invites/claims` 和 `commerce.gifts` 使用[独立兑换升级工具](../tools/redeem-migrate/migrate.ts)，操作方式和字段转换见[兑换升级文档](redeem-migration.md)。它保留旧领取对应的发放标识，并核对已有余额账本，避免重放已兑换奖励；不会读取或启用源面板原兑换码。两种来源不能混用。

## 套餐字段与金额单位

| 来源字段 | 处理方式 |
| --- | --- |
| V2Board `month_price`、`quarter_price`、`year_price` 等 | 原值为整数分；保留为同一套餐的可选周期 |
| Xboard `prices.monthly`、`prices.yearly` 等 | 原值为元；精确转换为整数分，保留为同一套餐的可选周期 |
| `onetime` / `onetime_price` | 长期选项，`durationUnit=permanent` |
| `reset_traffic` / `reset_price` | 保留为同套餐的重置流量选项 |
| SSPanel `price` | 原值为元，精确转为整数分 |
| SSPanel `content` / `content_extra` 与 `limit` | 保存可识别的期限、流量、等级等字段，同时保留原完整内容与限制 |

套餐通过 `periods` 保存多个选项，每个选项保留原周期标识、价格、销售和续费状态及对应的权益规则。期限采用 `durationUnit` 和 `durationValue`：`month` 表示日历月，`day` 表示天，`permanent` 表示长期。月付不会一律转成 30 天。导入保留原套餐的 `planFamilyId`，便于保持续费后的账户权益。已用旧版本拆分的套餐可使用[多周期套餐升级工具](plan-period-migration.md)合并；升级工具保留原始套餐归档，不更改已有订单和账户权益。

V2Board / Xboard 的 `sell`（旧版无此字段时使用 `show`）、`show`、`renew` 分别对应 `enabled`、`visible`、`renewable`。原套餐和节点的排序值一并保留，订阅按节点排序输出。未提供续费开关的旧版默认允许续费；停售和隐藏不会阻止原套餐明确允许的续费。

用户表的 `transfer_enable` 是字节；V2Board / Xboard 套餐表对应流量通常为 GB。不要在准备数据时预先统一乘除。适配器按来源分别转换，无法精确表示的金额会阻断，而不是四舍五入后继续。

SSPanel 商品的不同分支开通逻辑可能不同，因此默认停售。商品的 `sourceRecord`、`sourceContent` 和购买限制用于核对；没有定价的 V2Board / Xboard 套餐也会保留为停售记录。停售不撤回已从用户表导入的现有余额、流量和到期权益。

## 可选节点导入

可选节点转换面向 NodeRS 接入，原记录始终归档。V2Board / Xboard 可读取统一 `v2_server` 表或旧协议分表；存在 Xboard 机器表时保留机器编号和密钥，没有机器记录时按主机生成机器，生成的密钥在同一任务重试中保持稳定。

Xboard 的节点启用与订阅显示分别保存为 `enabled` 和 `visible`。通过 `parent_id` 共用监听服务的节点保留目标 `parentId`：未启用的子节点可以继续作为 IPv6 或中转订阅入口，但不会重复下发给 NodeRS 启动监听。子节点状态使用父节点心跳。隐藏但启用的节点仍运行，只从订阅中隐藏。

AnyTLS 等协议使用对象形式的 `protocol_settings.tls` 时，保留完整 TLS 设置，并提取其中的证书域名；旧 `tls_settings` 结构也会读取。Mieru 取 `transport` 的 TCP/UDP 设置，不把无用的 TLS 或 WebSocket 字段当成实际传输。原证书申请配置随节点保存，已有证书文件和申请缓存仍在节点本机，需要接入前核对。

SSPanel 识别传统 Shadowsocks、SS2022、TUIC、VMess 和 Trojan 的部分常见结构。传统 Shadowsocks 的用户独立端口会转换为独立节点，并限制为相应账户使用。连接密码、UUID 和 SS2022 派生规则分别处理。

SSPanel VMess 依据原 `security` 决定是否启用 TLS；不会对原先未加密的传输强加 TLS。常见 `ws-opts`、`grpc-opts` 等连接参数会转换为对应传输设置。SS2022 没有额外服务器密钥时，仅输出原用户派生密钥。

这不是所有旧节点配置的直接运行迁移。当前转换对传输方式、路由、回落、动态倍率和不认识的接入类型采取保守处理，遇到不能保持语义的配置会阻断。可以取消节点导入，先迁移用户与套餐，再在 PxBoard 配置节点。

接入前核对机器凭据、协议、监听端口、客户端端口、流量倍率、证书路径、私钥与服务器权限。数据库只含路径时，证书文件不会随导入复制。新生成的机器需要将 PxBoard 地址、机器编号和密钥写入 NodeRS；安装时选择 `--panel-type pxboard`。

## 迁移后核对与切换

提交完成后，核对报告中的用户和套餐数量，并抽查普通用户、停用用户、长期用户、零额度用户、含余额用户和带动态验证码用户。确认原密码能够登录，账户余额、已用流量、总额度、到期日与源站一致。

再核对各付费周期的套餐价格、期限单位、分组与上架状态。SSPanel 等待核对的商品应在确认规则后才上架，原支付配置和审计规则需要分别配置。

通用订阅地址为：

```text
https://panel.example.com/api/v1/extensions/subscriptions/s/原订阅令牌
```

通用地址根据客户端标识选择格式：识别到 sing-box 或 Hiddify 等返回 sing-box 配置，v2rayN、v2rayNG 或 Shadowrocket 等返回节点链接，其余默认返回 Mihomo 配置。需要固定格式时可在地址末尾添加 `?format=clash`、`?format=sing-box` 或 `?format=uri`。即使令牌未变，旧面板路径仍需要在客户端更新，除非管理员单独配置旧地址转发。节点接入且用户连接成功后，再完成域名或服务入口切换。保留源站备份和旧服务恢复方式，直到确认切换完成。

Mieru 支持 Mihomo（`format=clash`）和官方 `mierus://` 分享链接（`format=uri`），用户名与密码沿用原 UUID。sing-box 不支持原生 Mieru，因此混合订阅会保留可用节点并跳过 Mieru，响应头 `X-PxBoard-Skipped-Protocols` 会列出跳过的协议；若全部节点均不支持，则返回明确错误，避免生成只能直连的空代理配置。其他尚未支持的协议采用相同规则。

订阅同时保留已识别的 Reality 公钥和短 ID、TLS 域名和验证设置、WS / gRPC / HTTPUpgrade / h2 连接参数，以及 Hysteria 2 混淆和 TUIC 连接选项。XHTTP 支持 Mihomo 与通用链接，sing-box 订阅会跳过该传输。TUIC v4 无法当作 v5 使用，节点导入遇到 v4 会阻断。

## 恢复已导入的节点分组名称

新版迁移插件会建立来源分组名称，原有分组标识和关联保持不变。之前已导入的站点可使用 `tools/migrate-node-groups.ts` 从已保存的原始归档恢复名称；无法找到原名的分组显示来源和编号。原站未指定分组的账户保留原关系，不会自动分配节点。

使用 Node.js 24 或更新版本，在停止面板服务后先预览，再按需写入：

```sh
node tools/migrate-node-groups.ts --database /绝对路径/pxboard.db
node tools/migrate-node-groups.ts --database /绝对路径/pxboard.db --apply
```

工具在数据库旁创建独立备份，只补充缺失的分组记录，不覆盖已有名称，也不修改账户、节点、套餐或订单。多次执行不会重复创建。完成后启动面板并核对分组；迁移前仍应按部署流程备份完整数据目录。

## 快照与归档格式

快照包含来源、导出时间、时区及各表的列描述和原始记录：

```json
{
  "format": "pxboard.migration/v1",
  "source": "xboard",
  "sourceId": "old-production",
  "exportedAt": "2026-10-26T12:00:00Z",
  "timezone": "Asia/Shanghai",
  "tables": {
    "v2_user": {
      "columns": [{"name": "id", "type": "BIGINT", "nullable": false}],
      "rows": [{"id": "42", "email": "user@example.com"}]
    }
  }
}
```

上例只展示结构，不是可提交的完整用户数据。整数和小数字段导出为字符串以保留数值精度，列描述保留数据库类型；无法表示为有效 UTF-8 的原值以 `{"base64":"..."}` 保存。

核心数据目录下的 `plugins/import-<source>/state/<任务ID>.snapshot.json` 是任务冻结快照，位于插件版本目录之外。正式提交后，各原表逐行写入该迁移插件的 `archive`，列信息写入 `schemas`。没有业务转换器的表也保留原始行。原始归档不等于已激活的业务数据。

## 管理 API

以下路径以 `/api/v1/extensions/import-<source>` 为前缀，`<source>` 为 `sspanel`、`v2board` 或 `xboard`。需要管理员登录会话或有写权限的管理员访问密钥。成功响应使用 `{"data":...}`，错误使用 `{"error":{"code":"...","message":"..."}}`。

| 方法与路径 | 用途 |
| --- | --- |
| `GET /jobs` | 获取任务状态和报告摘要；不返回数据库连接、密码盐或任务密钥种子 |
| `POST /jobs` | 创建任务 |
| `POST /jobs/{id}/preview` | 异步准备、转换并检查；返回后轮询任务状态 |
| `POST /jobs/{id}/report` | 获取 `{title, details}` 形式的完整报告 |
| `POST /jobs/{id}/apply` | 异步提交已准备好的任务 |
| `GET /archive?jobId=...&table=...&offset=0` | 按表读取已提交任务的原始归档，每页 50 行，返回 `{items,total}` |

快照任务请求示例：

```json
{
  "name": "旧站数据",
  "sourcePath": "/srv/migration/old-production.json",
  "passwordMethod": "auto",
  "includeNodes": false,
  "includeAffiliate": false,
  "systemResetMethod": "1"
}
```

直接数据库任务将 `sourcePath` 换成 `dsn`，同时填写 `sourceId` 和 `timezone`。使用带全局盐的旧密码算法时再填写 `passwordSalt`；不要把这些私密请求保存到公开工单或日志。

## 备份、恢复与保密

预览和暂存不会创建正式用户或修改正式业务账户。提交发生错误时，查看报告并重试预览；若已有成功提交记录，核心会恢复为已完成状态，避免再次入账。

需要撤回已提交数据时，停止目标服务并恢复导入前的完整数据目录。当前不提供逐条自动撤回，以免误删用户在迁移后发生的新交易。目标已经对外使用时，先停止新增变更并核对差异，再决定恢复方式。

冻结快照及归档可能含密码编码、连接密码、动态验证码密钥、支付历史和其他个人信息。只向迁移管理员和服务账号开放相应目录及备份权限。数据库只读事务不会修改源站，但目标数据目录和源备份都应按私密数据管理。

卸载迁移插件不会主动清空核心中的已导入业务记录。完成迁移后可停用插件；保留必要的报告、快照和备份，并根据自己的保留要求管理这些私密文件。

从 Xboard 或 V2Board 迁移时，原站金额单位和符号属于站点设置。为避免覆盖目标站已有配置，迁移完成后请在商业插件的“金额设置”中按原站 `currency` 和 `currency_symbol` 配置；账户与套餐金额不会因此换算。
