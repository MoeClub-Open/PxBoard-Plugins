import { DatabaseSync } from 'node:sqlite'
import { parseArgs } from 'node:util'

type Row = Record<string, any>
type APIResult = { data?: Row; error?: { message?: string } }

const { values } = parseArgs({
  options: {
    panel: { type: 'string' },
    db: { type: 'string' },
    'migration-id': { type: 'string', default: 'redeem-separation-v1' },
    apply: { type: 'boolean', default: false },
    help: { type: 'boolean', default: false },
  },
})

const usage = `用法：node tools/redeem-migrate/migrate.ts --panel https://panel.example.com --db /private/pxboard.db [--apply]
默认只检查。管理员访问密钥通过 PX_REDEEM_ADMIN_TOKEN 提供。
读取停止写入后的完整 SQLite 备份，不修改旧数据库或删除旧记录。
`
if (values.help || !values.panel || !values.db || !process.env.PX_REDEEM_ADMIN_TOKEN) {
  process.stdout.write(usage)
  process.exit(values.help ? 0 : 1)
}
const panel = new URL(values.panel)
if (panel.username || panel.password || panel.search || panel.hash || !['', '/'].includes(panel.pathname)
  || panel.protocol !== 'https:' && !(panel.protocol === 'http:' && ['localhost', '127.0.0.1', '[::1]'].includes(panel.hostname))) {
  throw new Error('面板地址必须是 HTTPS 根地址，本机开发可使用 HTTP')
}

const database = new DatabaseSync(values.db, { readOnly: true })
function records(namespace: string, collection: string, prefix?: string): Row[] {
  const statement = database.prepare('SELECT data FROM documents WHERE namespace=? AND collection=?'
    + (prefix ? ' AND id LIKE ?' : '') + ' ORDER BY id')
  return (prefix ? statement.all(namespace, collection, prefix + '%') : statement.all(namespace, collection))
    .map((row) => JSON.parse(String(row.data)))
}
function text(row: Row, key: string, fallback = ''): string {
  const value = row[key]
  if (value === null || value === undefined) return fallback
  if (typeof value !== 'string') throw new Error(`旧记录 ${row.id || collectionName(row)} 的 ${key} 不是字符串`)
  return value
}
function collectionName(row: Row): string { return row.inviteId ? 'sharing.claims' : '兑换码' }
function number(row: Row, key: string, fallback = 0): number {
  const value = row[key] ?? fallback
  if (typeof value !== 'number' || !Number.isFinite(value)) throw new Error(`旧记录 ${row.id || collectionName(row)} 的 ${key} 不是有效数值`)
  return value
}
function date(row: Row, key: string, fallback = ''): string {
  const value = text(row, key, fallback)
  if (value && (!Number.isFinite(Date.parse(value)) || !/(?:Z|[+-]\d\d:\d\d)$/i.test(value))) {
    throw new Error(`旧记录 ${row.id || collectionName(row)} 的 ${key} 缺少有效时区`)
  }
  return value
}
function id(row: Row): string {
  const value = text(row, 'id')
  if (!value) throw new Error('旧记录缺少 ID')
  return value
}
const epoch = '1970-01-01T00:00:00Z'
const invites = records('sharing', 'invites')
const oldClaims = records('sharing', 'claims')
const gifts = records('commerce', 'gifts')
const giftLedger = new Map(records('commerce', 'ledger', 'gift-').map((row) => [id(row), row]))
database.close()

const claimsByInvite = new Map<string, Row[]>()
for (const claim of oldClaims) {
  const invite = text(claim, 'inviteId')
  const list = claimsByInvite.get(invite) || []
  list.push(claim)
  claimsByInvite.set(invite, list)
}
const codes: Row[] = []
const claims: Row[] = []
const sourceInvites = new Map<string, Row>()
for (const invite of invites) {
  const key = id(invite), limit = number(invite, 'limit'), used = number(invite, 'used')
  const days = number(invite, 'days'), quota = number(invite, 'quotaGiB')
  if (limit < 1 || used < 0 || !Number.isInteger(used) || days < 1 || quota < 0) {
    throw new Error(`旧兑换码 ${key} 的次数、期限或流量无法完整转换`)
  }
  if (typeof invite.enabled !== 'boolean') throw new Error(`旧兑换码 ${key} 缺少启用状态`)
  sourceInvites.set(key, invite)
  codes.push({
    id: 'sharing:' + key, code: text(invite, 'code'), name: text(invite, 'name', '访问权益兑换码'),
    kind: 'access', limit: Math.ceil(limit), perUserLimit: 1,
    historicalUses: Math.max(0, used - (claimsByInvite.get(key)?.length || 0)),
    enabled: invite.enabled, expiresAt: date(invite, 'expiresAt'),
    createdAt: date(invite, 'createdAt', epoch), durationUnit: 'day', durationValue: days,
    quotaGiB: quota, groups: text(invite, 'groups'),
    source: { namespace: 'sharing', collection: 'invites', id: key },
  })
}
for (const claim of oldClaims) {
  const key = id(claim), inviteId = text(claim, 'inviteId'), userId = text(claim, 'userId')
  const invite = sourceInvites.get(inviteId)
  if (!invite || !userId) throw new Error(`旧兑换记录 ${key} 缺少兑换码或用户`)
  const status = text(claim, 'status')
  if (!['', 'granted', 'pending'].includes(status)) throw new Error(`旧兑换记录 ${key} 的状态无法识别`)
  const pending = status === 'pending'
  let grant = claim.grant
  if (pending && (!grant || typeof grant !== 'object' || Array.isArray(grant)
    || grant.userId !== userId || typeof grant.grantId !== 'string' || !grant.grantId)) {
    throw new Error(`待处理兑换记录 ${key} 缺少原始权益请求，不能生成新的发放标识`)
  }
  if (!grant || typeof grant !== 'object' || Array.isArray(grant)) {
    grant = {
      userId, grantId: 'invite-' + inviteId + '-' + userId,
      durationUnit: 'day', durationValue: invite.days, quotaGiB: invite.quotaGiB, groups: invite.groups,
    }
  }
  const createdAt = date(claim, 'createdAt', epoch)
  claims.push({
    id: 'sharing:' + key, codeId: 'sharing:' + inviteId, userId,
    name: text(invite, 'name', '访问权益兑换码'), kind: 'access',
    summary: `${invite.days} 天 / ${invite.quotaGiB} GB`, status: pending ? 'pending' : 'redeemed',
    reward: { kind: 'access', grant }, createdAt,
    ...(!pending ? { redeemedAt: date(claim, 'redeemedAt', createdAt) } : {}),
    source: { namespace: 'sharing', collection: 'claims', id: key },
  })
}
for (const gift of gifts) {
  const key = id(gift), amount = number(gift, 'amount'), status = text(gift, 'status')
  if (!Number.isSafeInteger(amount) || amount <= 0) throw new Error(`旧余额兑换码 ${key} 的金额无效`)
  if (!['unused', 'used', 'redeeming', 'expired', 'disabled'].includes(status)) {
    throw new Error(`旧余额兑换码 ${key} 的状态无法识别`)
  }
  const userId = text(gift, 'userId'), ledger = giftLedger.get('gift-' + key)
  if (ledger && (ledger.userId !== userId || ledger.amount !== amount)) {
    throw new Error(`旧余额兑换码 ${key} 与已有余额账本不一致`)
  }
  if (status === 'unused' && ledger) throw new Error(`旧余额兑换码 ${key} 显示未用但已有入账，需先核对`)
  const name = text(gift, 'name', '余额兑换码'), createdAt = date(gift, 'createdAt', epoch)
  codes.push({
    id: 'commerce:' + key, code: text(gift, 'code'), name, kind: 'balance',
    amount, limit: 1, perUserLimit: 1, historicalUses: 0,
    enabled: !['expired', 'disabled'].includes(status), expiresAt: date(gift, 'expiresAt'), createdAt,
    source: { namespace: 'commerce', collection: 'gifts', id: key },
  })
  if (status === 'used' || status === 'redeeming' || ledger) {
    if (!userId) throw new Error(`已绑定的旧余额兑换码 ${key} 缺少用户，不能重新开放兑换`)
    const redeemedAt = date(gift, 'usedAt', ledger ? date(ledger, 'createdAt', createdAt) : createdAt)
    claims.push({
      id: 'commerce:' + key, codeId: 'commerce:' + key, userId, name, kind: 'balance',
      summary: (amount / 100).toFixed(2), status: ledger ? 'redeemed' : 'pending', createdAt,
      reward: { kind: 'balance', amount, operationId: 'legacy-gift-' + key, reason: '兑换码兑换' },
      ...(ledger ? { redeemedAt } : {}),
      source: { namespace: 'commerce', collection: 'gifts', id: key },
    })
  }
}
const codeIds = codes.map((row) => row.id)
const codeValues = new Set<string>(), seenClaims = new Set<string>()
for (const code of codes) {
  if (!code.code.trim() || codeValues.has(code.code)) throw new Error('源数据存在空兑换码或重复兑换码，未执行导入')
  codeValues.add(code.code)
}
for (const claim of claims) {
  if (seenClaims.has(claim.id)) throw new Error('源数据存在重复兑换记录，未执行导入')
  seenClaims.add(claim.id)
}

const token = process.env.PX_REDEEM_ADMIN_TOKEN
async function request(path: string, body?: Row): Promise<any> {
  const response = await fetch(new URL('/api/v1' + path, panel), {
    method: body ? 'POST' : 'GET',
    headers: { Authorization: `Bearer ${token}`, ...(body ? { 'Content-Type': 'application/json' } : {}) },
    body: body ? JSON.stringify(body) : undefined,
  })
  const result = await response.json() as APIResult
  if (!response.ok) throw new Error(result.error?.message || `请求失败：${response.status}`)
  if (result.data?.status === 'blocked' || result.data?.errors?.length) throw new Error('目标检查未通过，未完成升级')
  return result.data
}
const plugins = await request('/plugins') as Row[]
if (!plugins.some((row) => row.id === 'redeem' && row.enabled)) throw new Error('请先安装并启用兑换码插件')
if (values.apply && plugins.some((row) => row.id === 'sharing' && row.enabled)) {
  throw new Error('提交前请停用旧 sharing 插件，避免旧接口继续发放权益')
}
if (values.apply && gifts.length) {
  const commerce = plugins.find((row) => row.id === 'commerce')
  if (commerce && Number(String(commerce.version).split('.')[0]) < 2
    && (Number(String(commerce.version).split('.')[0]) < 1 || Number(String(commerce.version).split('.')[1]) < 4)) {
    throw new Error('余额兑换码提交前请先将商业插件升级到 1.4.0 或更新版本')
  }
}
const migrationId = values['migration-id']!
if (values.apply) await request('/extensions/redeem/migration/start', {
  migrationId, codeCount: codes.length, claimCount: claims.length,
})
for (const [collection, rows] of [['codes', codes], ['claims', claims]] as const) {
  for (let offset = 0; offset < rows.length; offset += 200) {
    await request('/extensions/redeem/migration/import', {
      migrationId, dryRun: !values.apply, codeIds, [collection]: rows.slice(offset, offset + 200),
    })
  }
}
if (values.apply) await request('/extensions/redeem/migration/finish', { migrationId })
process.stdout.write(JSON.stringify({
  mode: values.apply ? 'applied' : 'dry-run', migrationId,
  codes: codes.length, claims: claims.length,
  completedClaims: claims.filter((row) => row.status === 'redeemed').length,
  pendingClaims: claims.filter((row) => row.status === 'pending').length,
  historicalUses: codes.reduce((sum, row) => sum + row.historicalUses, 0),
  oldDataPreserved: true,
}, null, 2) + '\n')
