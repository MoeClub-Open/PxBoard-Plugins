import { chmodSync, existsSync, mkdirSync } from 'node:fs'
import { isDeepStrictEqual, parseArgs } from 'node:util'
import { isAbsolute, join } from 'node:path'
import { DatabaseSync } from 'node:sqlite'

type RecordData = Record<string, any>
type Document = { collection: string; id: string; data: string; updated_at: string }
type Move = Document & { value: RecordData }
type Update = { collection: string; id: string; value: RecordData }

const { values } = parseArgs({ options: {
  database: { type: 'string' }, apply: { type: 'boolean', default: false },
  help: { type: 'boolean', default: false },
} })
if (values.help || !values.database) {
  process.stdout.write('用法：node tools/migrate-commerce-affiliate.ts --database /绝对路径/pxboard.db [--apply]\n默认仅检查。写入前停止 PxBoard；--apply 会先备份，再将旧商业插件的推广数据搬入独立 affiliate 插件。\n')
  process.exit(values.help ? 0 : 1)
}
const database = values.database
if (!isAbsolute(database) || !existsSync(database)) throw new Error('请提供已经存在的数据库绝对路径')

const profileFields = ['commissionMode', 'commissionPercent', 'hasPriorPurchase']
const settingFields = ['commissionPercent', 'commissionHoldDays', 'commissionFirstPurchaseOnly']
const movedCollections = ['commissions', 'referrals', 'withdrawals']

function buildPlan(db: DatabaseSync) {
  const old = db.prepare("SELECT collection,id,data,updated_at FROM documents WHERE namespace='commerce' AND collection IN ('commissions','referrals','withdrawals','customers','settings')").all() as Document[]
  const lookup = db.prepare("SELECT data FROM documents WHERE namespace='affiliate' AND collection=? AND id=?")
  const ledger = db.prepare("SELECT data FROM documents WHERE namespace='commerce' AND collection='ledger' AND id=?")
  const moves: Move[] = [], updates: Update[] = [], deletes: Document[] = []
  let existing = 0, completedTransfers = 0, commissionCents = 0, withdrawalCents = 0
  const byKey = new Map<string, RecordData>()
  const add = (collection: string, id: string, value: RecordData, source: Document, unchanged = false) => {
    const key = `${collection}\0${id}`
    const prior = byKey.get(key) || (() => {
      const target = lookup.get(collection, id) as { data: string } | undefined
      return target ? JSON.parse(target.data) as RecordData : undefined
    })()
    if (prior) {
      if (!isDeepStrictEqual(prior, value)) throw new Error(`目标 ${collection} 已有不同内容，整批未写入`)
      existing++
    } else {
      moves.push({ collection, id, value, data: unchanged ? source.data : JSON.stringify(value), updated_at: source.updated_at })
      byKey.set(key, value)
    }
  }
  for (const row of old) {
    const value: RecordData = JSON.parse(row.data)
    if (movedCollections.includes(row.collection)) {
      let unchanged = true
      if (row.collection === 'commissions') commissionCents += Number(value.amount || 0)
      if (row.collection === 'withdrawals') {
        withdrawalCents += Number(value.amount || 0)
        if (value.method === 'wallet' && value.status === 'approving') {
          const entry = ledger.get(`commission-${row.id}`) as { data: string } | undefined
          if (entry) {
            const credited = JSON.parse(entry.data)
            if (credited.userId !== value.userId || credited.amount !== value.amount) throw new Error('旧佣金转账与账本不一致，需先核对；整批未写入')
            value.status = 'approved'
            unchanged = false
            completedTransfers++
          }
        }
      }
      add(row.collection, row.id, value, row, unchanged)
      deletes.push(row)
      continue
    }
    const fields = row.collection === 'customers' ? profileFields : settingFields
    if (!fields.some((field) => Object.hasOwn(value, field))) continue
    const movingRules = row.collection === 'customers' && ['commissionMode', 'commissionPercent'].some((field) => Object.hasOwn(value, field))
    if (row.collection === 'customers' && !movingRules && lookup.get('profiles', value.userId)) continue
    const target: RecordData = row.collection === 'customers'
      ? { id: value.userId, userId: value.userId }
      : { id: 'settings' }
    if (row.collection === 'customers' && typeof value.userId !== 'string') throw new Error('旧推广资料缺少用户标识，整批未写入')
    for (const field of fields) {
      if (!Object.hasOwn(value, field)) continue
      // The old per-user rule treated zero as inheritance, unlike explicit zero in affiliate.
      if (row.collection !== 'customers' || field !== 'commissionPercent' || typeof value[field] === 'number' && value[field] > 0) target[field] = value[field]
      if (row.collection !== 'customers' || field !== 'hasPriorPurchase') delete value[field]
    }
    if (row.collection === 'customers') {
      for (const field of ['createdAt', 'source']) if (Object.hasOwn(value, field)) target[field] = value[field]
    }
    add(row.collection === 'customers' ? 'profiles' : 'settings', target.id, target, row)
    if (row.collection !== 'customers' || movingRules) updates.push({ collection: row.collection, id: row.id, value })
  }
  return { moves, updates, deletes, summary: {
    recordsToMove: moves.length, matchingTargetRecords: existing, sourceRecordsToRemove: deletes.length,
    sourceRecordsToUpdate: updates.length, completedWalletTransfers: completedTransfers,
    commissionRecordCents: commissionCents, withdrawalRecordCents: withdrawalCents,
  } }
}

function report(summary: ReturnType<typeof buildPlan>['summary'], applied: boolean, backup?: string) {
  process.stdout.write(JSON.stringify({ applied, ...summary, ...(backup ? { backup } : {}) }, null, 2) + '\n')
}

const inspect = new DatabaseSync(database, { readOnly: true })
let plan: ReturnType<typeof buildPlan>
try {
  inspect.exec('BEGIN')
  plan = buildPlan(inspect)
  inspect.exec('ROLLBACK')
} finally { inspect.close() }
if (!values.apply || plan.moves.length + plan.updates.length + plan.deletes.length === 0) {
  report(plan.summary, false)
  process.exit(0)
}

process.umask(0o077)
const backups = database + '.affiliate-backups'
mkdirSync(backups, { recursive: true, mode: 0o700 })
chmodSync(backups, 0o700)
const backup = join(backups, `before-affiliate-${new Date().toISOString().replaceAll(/[:.]/g, '-')}-${process.pid}.db`)
const db = new DatabaseSync(database)
try {
  db.exec('PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON')
  db.exec(`VACUUM INTO '${backup.replaceAll("'", "''")}'`)
  chmodSync(backup, 0o600)
  db.exec('BEGIN IMMEDIATE')
  plan = buildPlan(db)
  const insert = db.prepare("INSERT INTO documents(namespace,collection,id,data,updated_at) VALUES('affiliate',?,?,?,?)")
  const update = db.prepare("UPDATE documents SET data=?,updated_at=? WHERE namespace='commerce' AND collection=? AND id=?")
  const remove = db.prepare("DELETE FROM documents WHERE namespace='commerce' AND collection=? AND id=?")
  for (const row of plan.moves) insert.run(row.collection, row.id, row.data, row.updated_at)
  const now = new Date().toISOString()
  for (const row of plan.updates) update.run(JSON.stringify(row.value), now, row.collection, row.id)
  for (const row of plan.deletes) remove.run(row.collection, row.id)
  db.exec('COMMIT')
  report(plan.summary, true, backup)
} catch (error) {
  if (db.isTransaction) db.exec('ROLLBACK')
  throw error
} finally { db.close() }
