# 推广与佣金

`affiliate` 是独立可选插件，依赖 `commerce 1.4.0` 或更新版本，以及 PxBoard 核心 `0.4.0` 或更新版本。自用、分享和商业模式都不会默认安装它。公益站点不安装或停用此插件即可关闭推广、佣金和提现入口，套餐、余额、节点和兑换码仍分别由对应插件提供。

## 启用与配置

在插件中心安装并启用“推广返佣”，再打开该插件的设置页，配置默认返佣比例、佣金冻结天数及是否只对首次购买返佣。用户在前台“推广返佣”复制自己的注册链接，查看佣金与结算申请。

个人推广资料可以覆盖默认比例与返佣方式：

| 字段 | 含义 |
| --- | --- |
| `commissionPercent` | 留空或 `null` 跟随设置；`0` 表示此用户不返佣；正数为个人百分比 |
| `commissionMode` | `system` 跟随设置、`recurring` 持续返佣、`firstPurchase` 仅首次购买 |
| `hasPriorPurchase` | 保留迁移前已经购买过套餐的事实，避免再次作为首购发佣 |

推荐关系由注册事件建立，同一新用户已有关系时不会被覆盖，也不能推荐自己。邮箱注册和第三方注册通过 `account.registered` 事件提供 `userId`、可选 `referral` 和 `registeredAt`，事件来源必须是对应注册插件。

## 订单与退款

佣金只依据商业插件确认完成的套餐订单产生，余额充值不产生推广佣金。退款会撤回相应佣金；事件重复投递或退款事件先到，不会重新发放同一笔佣金。首次购买会核对已有购买记录和迁移前的购买标记。

冻结期结束后，佣金才可用于结算。待审批申请占用可用佣金，撤回或拒绝后释放。退款导致佣金不足时，不能继续审批超额申请；已经结算的部分会从后续可用佣金中抵扣。

余额结算通过商业插件的幂等内部入账接口完成。入账中断后重试同一申请不会重复增加余额。现金提现需要运营方在外部完成打款，再登记流水；插件不会自动转账到外部账户。

## 关闭功能

停用 `affiliate` 后，推广页面、接口和新的佣金处理都停止。已有推荐关系、佣金及申请记录保留，重新启用后仍可读取。停用期间的新订单不会在以后启用时补算推广奖励。

## 从旧商业插件拆出数据

已有 PxBoard `commerce 1.3.x` 的站点，先让站点进入维护状态，停止接收新增交易并备份完整数据目录。更新核心和商业插件、安装 `affiliate` 后，停用商业与推广插件并停止面板。确认服务已停止，再在官方插件仓库使用 Node.js 24 或更新版本执行：

```sh
node tools/migrate-commerce-affiliate.ts --database /var/lib/pxboard/pxboard.db
```

默认仅检查，输出记录数量与金额汇总，不写入数据库。确认无冲突后执行：

```sh
node tools/migrate-commerce-affiliate.ts --database /var/lib/pxboard/pxboard.db --apply
```

工具写入前会在数据库旁的 `pxboard.db.affiliate-backups/` 新建一致数据库备份；Linux 上目录权限为 `0700`、文件权限为 `0600`，Windows 使用所在目录的系统访问权限。随后在一个事务中完成以下搬移：

| 原位置 | 新位置 |
| --- | --- |
| `commerce/commissions` | `affiliate/commissions`，保留记录 ID、金额和状态 |
| `commerce/referrals` | `affiliate/referrals`，保留推荐关系 |
| `commerce/withdrawals` | `affiliate/withdrawals`，保留申请与打款记录 |
| `commerce/customers` 的返佣字段 | `affiliate/profiles`，按原用户标识建立资料；既有购买事实同时保留在商业资料中 |
| `commerce/settings` 的返佣设置 | `affiliate/settings` |

个人折扣和其他商业数据留在 `commerce`，旧返佣规则字段与推广集合在成功搬移后移除。`hasPriorPurchase` 是通用购买事实，会复制给推广资料并继续留在商业资料中，因此以后才安装推广插件也能识别老用户；重复执行不会重写已拆出的资料。旧商业插件的个人比例 `0` 原本表示跟随设置，升级时会转换为空值，保持原有含义。

若旧余额结算已经写入 `commission-<申请ID>` 账本，但申请仍处于 `approving`，工具会核对用户和金额并将申请标记为已审批，保留原账本，不再次入账。账本不一致或新位置已有不同内容时，整批拒绝，不覆盖财务数据。相同内容已存在时跳过；完整执行后再次运行没有额外写入。

完成后启动面板，在独立推广插件核对佣金、结算申请和个人资料。新版本不保留旧 `/extensions/commerce/affiliate/...` 或商业插件的提现接口。该工具只拆分旧 PxBoard 数据；外部面板的导入规则见 [迁移文档](migration.md)。

## 接口

路径前缀为 `/api/v1/extensions/affiliate`，需要登录。

| 方法与路径 | 用途 |
| --- | --- |
| `GET /summary` | 当前用户的邀请人数、佣金汇总和推广链接 |
| `GET /commissions` | 本人的佣金明细；管理员可查看全部记录 |
| `GET/POST /withdrawals` | 查看或创建结算申请 |
| `POST /withdrawals/{id}/cancel` | 本人撤回待审批申请 |
| `POST /withdrawals/{id}/approve` | 管理员审批，余额结算入账一次 |
| `POST /withdrawals/{id}/reject` | 管理员拒绝待审批申请 |
| `POST /withdrawals/{id}/paid` | 管理员登记现金打款流水 |
| `GET /settings`、`PATCH /settings` | 管理员读取和修改推广设置 |
| `GET/POST /profiles`、`PATCH /profiles/{userId}` | 管理员管理个人推广规则 |

金额均为整数分；现金收款账户和打款流水仅向有权限的用户提供。普通用户不能修改他人的推广资料或审批结算。
