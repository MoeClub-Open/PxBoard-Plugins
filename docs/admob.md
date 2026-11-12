# 激励广告

`admob` 是可选官方插件，当前版本为 `0.0.1`，用于校验 Google AdMob 激励广告服务端验证（SSV）签名并发放套餐权益或账户余额。客户端自身的开屏广告开关、激励广告单元 ID 与在线支付展示开关均固化在客户端构建参数中，面板仅负责服务端签名校验与账本入账。

## 后台配置与回调登记

安装并启用 `admob` 插件后，在后台“运营 → 激励广告”配置发奖规则：

- **套餐激励**：开启后可设置赠送有效天数、赠送流量（GB）、节点分组、设备数与速率限制，以及是否仅限当前无有效套餐的用户领取（默认开启；关闭时对已有套餐用户顺延有效期并叠加流量，不覆盖原有节点分组）。
- **余额激励**：需同时启用 `commerce` 插件；按分配置单次奖励金额，校验通过后写入用户钱包流水。
- **广告单元校验**：选填；填写完整单元 ID 或尾号后，仅接受匹配的 AdMob 单元回调。
- **SSV 签名密钥**：留空时首次读取自动生成随机密钥，用于签发短期 `custom_data` 令牌。
- **SSV 回调地址**：页面提供只读一键复制的完整回调地址，可直接填入 Google AdMob 控制台。

在 Google AdMob 控制台对应激励广告单元的“服务端验证（SSV）”回调地址填写：

```text
https://panel.example.com/api/v1/extensions/admob/ssv
```

AdMob 控制台发送测试验证请求时，插件在通过 Google 官方公钥 ECDSA-SHA256 验签后直接返回 HTTP 200，不执行实际发奖。

## 客户端交互流程

| 过程 / 回调 | 权限及用途 |
| --- | --- |
| `POST /rpc/v1/admob/reward.prepare` | 登录用户展示广告前获取 `planSsvUserId` / `planSsvCustomData` 或 `pointsSsvUserId` / `pointsSsvCustomData` |
| `POST /rpc/v1/admob/reward.pending` | 客户端观看完成后登记待入账记录；若 SSV 回调已先行入账则直接返回 `credited = true` 与 `rewardContent` |
| `GET /api/v1/extensions/admob/ssv` | Google AdMob 服务器异步回调；校验 Google 公钥 ECDSA 签名、时间戳与 `custom_data` HMAC，按 `transaction_id` 幂等发奖 |
| `POST /rpc/v1/admob/reward.history` | 登录用户查询最近激励发奖记录 |
