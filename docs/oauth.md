# 第三方登录

`oauth` 是可选官方插件，当前版本为 `0.0.1`，要求当前同版本 PxBoard 核心。支持 Google、GitHub 和 LinuxDO；不依赖商业插件，不随 Xboard 数据导入插件自动安装或转换绑定数据。

默认登录后进入用户前台 `/user`。从 `/admin` 管理入口发起登录时保留所选管理页面，普通用户仍不能访问管理页面。账户绑定完成后返回 `/user/account`。

已有绑定的登录和账户绑定可以独立使用。创建新账号还需要安装、启用“注册”插件并打开其注册开关，同时打开对应登录服务的“允许新账号注册”。未安装注册插件、注册插件停用或无法读取注册政策时，第三方新账号注册关闭；已有账号仍可用原绑定登录。两个插件没有强制安装依赖。

## 配置登录服务

在插件中心安装并启用“第三方登录”，打开后台“第三方登录”，添加对应服务。

| 配置 | 含义 |
| --- | --- |
| 登录服务 | Google、GitHub 或 LinuxDO |
| 显示名称 | 登录页和账户页使用的名称 |
| 面板地址 | 用户实际访问的 HTTPS 根地址，例如 `https://panel.example.com` |
| 客户端 ID、客户端密钥 | 提供方应用的服务端凭据 |
| 已登记回调地址 | 可选；复用同域名下已有回调时填写，否则使用插件生成的地址 |
| 启用 | 在登录和账户绑定入口显示 |
| 允许新账号注册 | 默认关闭；还需注册插件允许注册，已有绑定登录不受注册开关影响 |

保存后将列表中的“授权回调地址”登记到提供方应用。默认地址为：

```text
https://panel.example.com/api/v1/extensions/oauth/callback/<登录服务配置ID>
```

生产环境必须使用 HTTPS。本机开发可使用 `http://127.0.0.1:<端口>` 或 `http://localhost:<端口>`。配置的面板地址应与核心公开地址和浏览器访问地址一致。

Google 请求 `openid email profile`，GitHub 请求 `read:user user:email`，LinuxDO 请求 `openid email profile`，通过 HTTP Basic 交换授权码。LinuxDO 当前公布的 [服务配置](https://connect.linux.do/.well-known/openid-configuration) 支持这些范围和 PKCE，但不承诺 `email_verified`，不能假定它提供已验证邮箱。插件使用提供方服务端返回的稳定账号标识，不把用户名或邮箱当作第三方身份标识。客户端密钥只保存在服务端，管理查询也不返回密钥值；编辑时留空表示保留原密钥。

同时满足全局注册政策和服务注册开关时，提供方有已验证邮箱可以直接创建新账号；没有可信邮箱时会进入“完成注册”，由用户填写邮箱和至少 10 位登录密码，沿用普通注册规则，不依赖提供方是否实现邮箱验证字段。如果该邮箱已存在，插件要求用户使用原方式登录后手动绑定，不会按邮箱自动接管已有账号，也不会生成虚拟邮箱。

新账号与首次登录绑定在同一事务创建，避免注册中断留下无法登录的账号。“完成注册”的请求保存在服务端，绑定发起授权的浏览器，十分钟内有效，成功后消费；填写错误可在有效期内更正。提交时再次检查注册政策，不能通过先前打开的表单绕过注册关闭。账号和绑定已经创建、只是登录凭证签发暂时失败时，可以重试登录，不会重复创建账号；已有绑定登录优先于注册政策检查。直接创建的账号使用随机初始密码，需要密码登录时通过已配置的邮件找回功能设置；在“完成注册”填写的密码可直接用于后续登录。

授权请求可以携带 `referral` 推荐标识。只有本次实际创建的新用户会将它与注册时间保存在首次绑定中，并发布 `account.registered` 事件；事件投递失败不阻止注册或登录，后续由核心定时事件重试。同一用户的事件可能重发，接收插件应按 `userId` 去重。普通绑定、已有账号登录和手动搬入的旧绑定不会补发注册事件。解除首次绑定时仍保留最小注册来源数据，内容只有用户标识、推荐标识、注册时间和投递状态，不保留第三方账号标识。

## 登录与绑定

启用服务后，登录页显示相应按钮；注册页仅显示允许注册的服务。已登录用户在“我的账户 → 第三方登录”绑定或解除自己的账号。同一用户在同一服务配置下只能绑定一个第三方账号，同一第三方账号不能绑定多个用户。

登录使用一次性状态值、PKCE 和当前浏览器的随机证明。提供方回调只签发三分钟有效的一次性登录凭证，由核心检查账户是否停用、插件是否仍获登录授权及动态验证码后设置会话。回调凭证不能在另一浏览器直接使用。

解除最后一个第三方绑定需要输入当前登录密码；启用了动态验证的账号还需要验证码。只有第三方登录、尚未设置可用密码的用户，应先通过邮件找回设置密码。解绑不会删除面板账号、余额、套餐或访问权益。

停用插件或服务配置会隐藏对应入口并停止新的授权流程。已有绑定保留。存在绑定的服务配置不能直接删除，可先停用，待完成账号调整后再删除。

## 手动搬入已有绑定

第三方登录属于独立扩展，Xboard、SSPanel、V2Board 导入插件不会转换此类绑定。先完成用户迁移，再配置本插件，最后单独核对并搬入绑定。

准备仅包含目标字段的 JSON 数组，例如：

```json
[
  {
    "providerId": "新面板登录服务配置ID",
    "subject": "第三方账号原始稳定ID字符串",
    "userId": "osa-xboard:user:42",
    "displayName": "可选显示名称",
    "createdAt": "2026-04-17T08:00:00Z"
  }
]
```

`providerId` 是新插件的登录服务配置 ID，`subject` 是提供方账号标识，`userId` 必须精确指向已经存在的目标用户。Google 使用原 `sub`，GitHub 和 LinuxDO 使用原账号 `id`。标识需保持字符串，不经过浮点数转换；`createdAt` 可省略，填写时使用带时区的标准日期。

对于自定义 Xboard OAuth 扩展，旧表 `v2_oauth_accounts.provider` 用于人工选择目标服务配置，`provider_id` 对应 `subject`，`user_id` 对应已迁移目标用户 ID。表名与字段仅是手动核对参考，不由通用面板迁移器自动处理。

使用具有管理员写权限的访问密钥执行独立工具，需要 Node.js 24 或更新版本：

```sh
export PX_OAUTH_ADMIN_TOKEN='管理员访问密钥'
node tools/oauth-bindings/import.ts \
  --panel https://panel.example.com \
  --input /private/bindings.json
```

默认只检查，不写入。核对新增数量和重复数量后，以同一文件添加 `--apply`：

```sh
node tools/oauth-bindings/import.ts \
  --panel https://panel.example.com \
  --input /private/bindings.json \
  --apply
unset PX_OAUTH_ADMIN_TOKEN
```

所有服务和目标用户必须存在。发现跨用户冲突、同用户同服务重复身份或错误日期时，整批拒绝；检查通过后使用一个事务写入。同一文件重复执行会跳过相同绑定，不覆盖现有绑定。不创建新用户，不修改密码，也不搬入客户端或其他扩展的数据。

如需复用原提供方已登记回调，先在服务配置中填写同域名、同协议的原回调地址，再由反向代理将该路径转到本插件对应回调，保留查询参数。迁移预演时允许面板入口与登记回调使用不同端口；提供方仍会精确检查完整回调地址。授权请求和交换授权码使用同一个登记地址。完成提供方控制台调整后，可移除该临时路径，改用插件默认回调。

## 接口

以下路径以 `/api/v1/extensions/oauth` 为前缀。成功返回 `{"data":...}`。

| 方法与路径 | 权限及用途 |
| --- | --- |
| `GET /providers` | 公开的已启用服务名称、类型；`allowRegistration` 反映服务开关与全局注册政策的组合结果 |
| `POST /authorize` | 发起登录；`purpose=bind` 时需要用户会话；参数为 `providerId/purpose/proof/redirectPath`，可选 `referral` |
| `GET /callback/{provider}` | 提供方回调，仅处理服务端已保存的一次性授权状态 |
| `POST /complete-binding` | 当前用户以 `ticket/proof` 确认绑定 |
| `POST /registration` | 以待注册 `ticket/proof` 查询服务名称与到期时间，不返回提供方邮箱 |
| `POST /complete-registration` | 以 `ticket/proof/email/password` 完成新账号注册，返回核心登录凭证 |
| `GET /bindings` | 当前用户的绑定列表，含 `requiresPassword` |
| `DELETE /bindings/{id}` | 解除自己的绑定；最后一条需要请求体 `password/code` |
| `GET/POST /settings/providers` | 管理员查询或创建服务配置 |
| `PATCH/DELETE /settings/providers/{id}` | 管理员编辑或删除未使用的配置 |
| `POST /bindings/import` | 管理员人工搬入绑定；`dryRun` 默认 `true`，`bindings` 为上述数组 |
| `GET /internal/registrations` | 管理员或推广插件读取本插件创建的新账号来源，仅返回 `userId/referral/registeredAt`，不返回第三方账号标识 |

最终登录由核心 `POST /api/v1/auth/external/complete` 完成；该接口不是插件自行设置会话。插件的 `auth:login` 权限允许申请登录凭证，应仅授予可信插件。数据目录包含服务端密钥和登录绑定，备份与恢复时按面板敏感数据管理。

本插件只读调用注册插件的 `GET /internal/policy` 获取注册开关，不向注册插件写入用户绑定。`call:registration` 是可选跨插件调用权限，不会自动安装注册插件；`emit:account.registered` 用于发布本插件实际创建的新用户事件，`system.tick` 仅接受核心来源，用于重试尚未成功投递的记录。
