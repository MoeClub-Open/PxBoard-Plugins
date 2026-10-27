import { chmodSync, existsSync, mkdirSync } from 'node:fs'
import { isDeepStrictEqual, parseArgs } from 'node:util'
import { isAbsolute, join } from 'node:path'
import { DatabaseSync } from 'node:sqlite'

type RecordData = Record<string, any>
type Document = { id: string; data: string; updated_at: string }
type Plan = Document & { value: RecordData }
type Conversion = { primary: Plan; rows: Plan[]; value: RecordData }

const { values } = parseArgs({ options: {
  database: { type: 'string' }, apply: { type: 'boolean', default: false },
  help: { type: 'boolean', default: false },
} })
if (values.help || !values.database) {
  process.stdout.write('用法：node tools/migrate-plan-periods.ts --database /绝对路径/pxboard.db [--apply]\n默认只检查并显示合并方案；写入前停止 PxBoard。--apply 先备份数据库，再以事务合并同源周期套餐并归档原记录。\n')
  process.exit(values.help ? 0 : 1)
}
const database = values.database
if (!isAbsolute(database) || !existsSync(database)) throw new Error('请提供已存在的数据库绝对路径')

const labels: Record<string, string> = {
  monthly: '月付', quarterly: '季付', half_yearly: '半年付', yearly: '年付',
  two_yearly: '两年付', three_yearly: '三年付', onetime: '一次性', reset_traffic: '重置流量',
}
const order = Object.keys(labels)
const periodFields = ['price', 'durationUnit', 'durationValue', 'kind', 'resetPolicy', 'enabled', 'visible', 'renewable', 'reviewRequired']
const separate = new Set(['id', 'name', 'sourcePeriod', ...periodFields])

function baseName(plan: RecordData, allowPlain: boolean): string {
  const suffix = ' · ' + labels[plan.sourcePeriod]
  if (typeof plan.name !== 'string' || !plan.name.trim()) throw new Error(`套餐 ${plan.id} 缺少名称`)
  if (plan.name.endsWith(suffix)) return plan.name.slice(0, -suffix.length)
  if (allowPlain) return plan.name
  throw new Error(`套餐 ${plan.id} 名称已修改，不能自动合并`)
}

function common(plan: RecordData): RecordData {
  return Object.fromEntries(Object.entries(plan).filter(([key]) => !separate.has(key)))
}

function enabled(value: RecordData) { return value.enabled === true }
function visible(value: RecordData) { return value.visible !== false }
function renewable(value: RecordData) { return value.renewable !== false }
function reviewed(value: RecordData) { return value.reviewRequired === true }

function buildPlan(db: DatabaseSync): Conversion[] {
  const documents = db.prepare("SELECT id,data,updated_at FROM documents WHERE namespace='commerce' AND collection='plans' ORDER BY id").all() as Document[]
  const families = new Map<string, Plan[]>()
  for (const row of documents) {
    const value = JSON.parse(row.data) as RecordData
    if (value.id !== row.id) throw new Error(`套餐 ${row.id} 记录标识不一致`)
    if (typeof value.planFamilyId === 'string' && value.planFamilyId) {
      const group = families.get(value.planFamilyId) || []
      group.push({ ...row, value })
      families.set(value.planFamilyId, group)
    }
  }
  const archived = db.prepare("SELECT data FROM documents WHERE namespace='commerce' AND collection='plan_period_archive' AND id=?")
  const conversions: Conversion[] = []
  for (const [family, rows] of families) {
    if (!rows.some((row) => row.value.sourcePeriod)) continue
    if (rows.some((row) => !labels[row.value.sourcePeriod] || row.value.periods !== undefined)) {
      throw new Error(`套餐组 ${family} 包含未识别周期或已合并记录，整批停止`)
    }
    const source = rows[0].value.source
    if (!source || typeof source !== 'object' || !source.recordId || !source.sourceId || !['xboard', 'v2board'].includes(String(source.system).toLowerCase())) {
      throw new Error(`套餐组 ${family} 缺少可确认的 Xboard/V2Board 来源，整批停止`)
    }
    const names = rows.map((row) => baseName(row.value, rows.length === 1))
    if (names.some((name) => name !== names[0]) || rows.some((row) => !isDeepStrictEqual(common(row.value), common(rows[0].value)))) {
      throw new Error(`套餐组 ${family} 的共同字段不同，需先人工核对；整批未写入`)
    }
    const seen = new Set<string>()
    for (const row of rows) {
      if (seen.has(row.value.sourcePeriod)) throw new Error(`套餐组 ${family} 存在重复周期`)
      seen.add(row.value.sourcePeriod)
      const existing = archived.get(row.id) as { data: string } | undefined
      if (existing && !isDeepStrictEqual(JSON.parse(existing.data), row.value)) throw new Error(`套餐 ${row.id} 的归档与原记录冲突，整批停止`)
      if (typeof row.value.price !== 'number' || !Number.isSafeInteger(row.value.price) || row.value.price < 0) throw new Error(`套餐 ${row.id} 的价格无效`)
      if (typeof row.value.durationUnit !== 'string' || typeof row.value.durationValue !== 'number' || typeof row.value.kind !== 'string') throw new Error(`套餐 ${row.id} 的类型或期限缺失，需先人工核对`)
    }
    rows.sort((a, b) => order.indexOf(a.value.sourcePeriod) - order.indexOf(b.value.sourcePeriod))
    const primary = rows[0]
    const options = rows.map(({ id, value }) => {
      const option: RecordData = { id, label: labels[value.sourcePeriod] }
      for (const key of periodFields) if (Object.hasOwn(value, key)) option[key] = value[key]
      // A missing rule must not silently inherit the chosen primary period's rule.
      if (!Object.hasOwn(option, 'resetPolicy')) option.resetPolicy = null
      option.enabled = enabled(value)
      option.visible = visible(value)
      option.renewable = renewable(value)
      option.reviewRequired = reviewed(value)
      return option
    })
    const value: RecordData = { ...primary.value, name: names[0], periods: options,
      enabled: rows.some(({ value }) => enabled(value)),
      visible: rows.some(({ value }) => visible(value)),
      renewable: rows.some(({ value }) => renewable(value)),
      reviewRequired: rows.every(({ value }) => reviewed(value)),
    }
    delete value.sourcePeriod
    conversions.push({ primary, rows, value })
  }
  return conversions
}

function report(plan: Conversion[], applied: boolean, backup?: string) {
  process.stdout.write(JSON.stringify({ applied, families: plan.map(({ primary, rows, value }) => ({
    familyId: value.planFamilyId, planId: primary.id, name: value.name,
    periods: value.periods, sourceRecordCount: rows.length,
  })), ...(backup ? { backup } : {}) }, null, 2) + '\n')
}

const inspect = new DatabaseSync(database, { readOnly: true })
let plan: Conversion[]
try {
  inspect.exec('BEGIN')
  plan = buildPlan(inspect)
  inspect.exec('ROLLBACK')
} finally { inspect.close() }
if (!values.apply || plan.length === 0) {
  report(plan, false)
  process.exit(0)
}

process.umask(0o077)
const backups = database + '.plan-period-backups'
mkdirSync(backups, { recursive: true, mode: 0o700 })
chmodSync(backups, 0o700)
const backup = join(backups, `before-plan-periods-${new Date().toISOString().replaceAll(/[:.]/g, '-')}-${process.pid}.db`)
const db = new DatabaseSync(database)
try {
  db.exec('PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON')
  db.exec(`VACUUM INTO '${backup.replaceAll("'", "''")}'`)
  chmodSync(backup, 0o600)
  db.exec('BEGIN IMMEDIATE')
  plan = buildPlan(db)
  const archive = db.prepare("INSERT OR IGNORE INTO documents(namespace,collection,id,data,updated_at) VALUES('commerce','plan_period_archive',?,?,?)")
  const update = db.prepare("UPDATE documents SET data=?,updated_at=? WHERE namespace='commerce' AND collection='plans' AND id=?")
  const remove = db.prepare("DELETE FROM documents WHERE namespace='commerce' AND collection='plans' AND id=?")
  const now = new Date().toISOString()
  for (const conversion of plan) {
    for (const row of conversion.rows) archive.run(row.id, row.data, row.updated_at)
    update.run(JSON.stringify(conversion.value), now, conversion.primary.id)
    for (const row of conversion.rows) if (row.id !== conversion.primary.id) remove.run(row.id)
  }
  db.exec('COMMIT')
  report(plan, true, backup)
} catch (error) {
  if (db.isTransaction) db.exec('ROLLBACK')
  throw error
} finally { db.close() }
