# 从旧分享和充值卡升级到兑换码插件

独立的 `redeem` 插件统一提供访问权益和余额兑换码。旧 `sharing` 插件的领取入口以及商业插件中的充值卡入口不再使用；普通用户先注册或登录，再在“兑换码”输入兑换码。

本工具只处理已有 PxBoard 的 `sharing.invites/claims` 和 `commerce.gifts/ledger`，不读取 Xboard、SSPanel、V2Board 数据库，也不会转换第三方客户端或 OAuth 数据。

## 备份与准备

升级需要核心 `0.4.0`、兑换码插件 `1.0.0`。余额兑换还需要商业插件 `1.4.0` 或更新版本；访问权益兑换只依赖访问账户插件。

先停止旧面板写入并备份完整数据目录，保留 SQLite 主文件及可能存在的 WAL 文件。也可以用 SQLite 的在线备份能力生成一致的备份文件。不要直接复制正在写入的 SQLite 主文件，也不要在旧接口仍能兑换时开始提交。

在目标面板安装兑换码插件并停用旧 `sharing` 插件；已经安装商业插件时，先将它升级到 `1.4.0`。工具不删除旧集合，便于核对和恢复。没有商业插件也可以保留旧余额码和兑换历史，余额发放会等待商业插件启用，不能用它兑换访问权益。管理员访问密钥需有写权限，只放在环境变量中。

需要 Node.js 24 或更新版本，使用内置 SQLite 读取能力，无需额外安装数据库包。

```sh
export PX_REDEEM_ADMIN_TOKEN='管理员访问密钥'
node tools/redeem-migrate/migrate.ts \
  --panel https://panel.example.com \
  --db /private/backup/pxboard.db
```

默认只检查。工具读取备份，验证兑换码、次数、金额、用户引用和旧记录状态，再检查目标。输出只有数量和状态，不打印兑换码或密钥。发现冲突时停止，不自动覆盖。

确认数量后，使用同一个数据库备份提交：

```sh
node tools/redeem-migrate/migrate.ts \
  --panel https://panel.example.com \
  --db /private/backup/pxboard.db \
  --apply
unset PX_REDEEM_ADMIN_TOKEN
```

提交从 `/migration/start` 开始，兑换码插件暂停新的兑换和待处理记录发放。工具每批提交 200 条，先写兑换码，再写兑换记录；最后 `/migration/finish` 校验预期数量并恢复兑换。中断后保持暂停，用同一份备份及同一个迁移标识重新执行即可继续。默认标识为 `redeem-separation-v1`，不要为绕过冲突而更换标识。

## 数据对应

| 旧记录 | 新记录与处理 |
| --- | --- |
| `sharing.invites` | `redeem.codes`，标识为 `sharing:<原ID>`；保留码值、启用状态、总次数、时长、流量、节点分组及到期时间 |
| `sharing.claims` 已发放 | `redeem.claims` 状态为 `redeemed`，不会重新调用开通接口 |
| `sharing.claims` 待处理 | 状态为 `pending`，保留完整原始发放请求及原 `grantId`；恢复后重试仍由访问账户插件去重 |
| 旧版本无 `status` 的成功记录 | 视为已兑换，只保留历史，不重新发放 |
| 旧分享已用次数大于现存记录数 | 差额写入 `historicalUses`，不会因缺少明细重新开放这些名额 |
| `commerce.gifts` 未使用 | `redeem.codes` 的余额兑换码，标识为 `commerce:<原ID>`，金额仍为整数分 |
| 已绑定用户且存在 `gift-<原ID>` 余额账本 | 标记已兑换，保留原账本，不再入账 |
| 已绑定用户但尚无对应余额账本 | 保留为待处理，使用稳定的 `legacy-gift-<原ID>` 操作标识完成入账 |
| 过期或禁用的旧码 | 保留到期时间或禁用状态，不自动恢复可兑换状态 |

旧待处理发放请求缺少原始信息、旧卡与账本金额或用户不一致、重复码值或无法识别的状态都会阻止提交。工具不会猜测受益用户、生成新的旧发放标识，或用重新兑换修复不明确的余额。

升级后核对兑换码总数、已兑换和待处理数量、无明细的历史次数，以及余额账本。保留旧数据库和原集合，直到确认发放与余额一致。目标已经产生新兑换后，如需恢复，应先停止新写入并核对新增记录，不能只恢复旧备份而丢弃新兑换。

## 管理接口

以下路径以 `/api/v1/extensions/redeem` 为前缀，均需要管理员写权限；检查请求中的 `dryRun` 默认为只读。

| 方法与路径 | 用途 |
| --- | --- |
| `POST /migration/start` | 以 `migrationId/codeCount/claimCount` 建立升级任务并暂停兑换 |
| `POST /migration/import` | 以相同标识分批检查或写入 `codes`、`claims`；只读检查可提供完整 `codeIds` 引用列表 |
| `POST /migration/finish` | 核对总数并结束升级 |

这是一次性数据升级，不是旧插件接口的兼容层。新页面、接口和功能开关均使用 `redeem` 插件。
