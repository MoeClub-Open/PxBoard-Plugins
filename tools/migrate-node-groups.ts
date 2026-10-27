import { chmodSync, existsSync, mkdirSync } from 'node:fs'
import { isAbsolute, join } from 'node:path'
import { parseArgs } from 'node:util'
import { DatabaseSync } from 'node:sqlite'

type Data = Record<string, any>
const { values } = parseArgs({ options: { database: { type: 'string' }, apply: { type: 'boolean', default: false }, help: { type: 'boolean', default: false } } })
if (values.help || !values.database) {
  process.stdout.write('用法：node tools/migrate-node-groups.ts --database /绝对路径/pxboard.db [--apply]\n默认只预览；写入前请停止面板。工具只新增分组名称，不修改现有节点、账户、套餐或订单。\n')
  process.exit(values.help ? 0 : 1)
}
const database = values.database
if (!isAbsolute(database) || !existsSync(database)) throw new Error('请提供已存在的数据库绝对路径')

function plan(db: DatabaseSync): Data[] {
  const records = db.prepare("SELECT id FROM documents WHERE namespace='nodes' AND collection='groups'").all() as { id: string }[]
  const existing = new Set(records.map((row) => row.id))
  const names = new Map<string, { name: string; sourceId: string }>()
  const archived = db.prepare("SELECT data FROM documents WHERE namespace LIKE 'import-%' AND collection='archive' AND json_extract(data,'$.table')='v2_server_group'").all() as { data: string }[]
  for (const { data } of archived) {
    const entry = JSON.parse(data)
    const source = String(entry.runSource || '')
    const id = String(entry.row?.id || '')
    const name = String(entry.row?.name || '').trim()
    if (!source || !id || !name) continue
    const key = `${source}:group:${id}`
    if (names.has(key) && names.get(key)?.name !== name) throw new Error(`分组 ${key} 的来源名称不一致，请先核对`)
    names.set(key, { name, sourceId: source })
  }
  const ids = new Set<string>(['default', ...names.keys()])
  const add = (value: unknown) => {
    if (typeof value === 'string') for (const id of value.split(',').map((v) => v.trim()).filter(Boolean)) ids.add(id)
  }
  const rows = db.prepare("SELECT namespace,collection,data FROM documents WHERE (namespace='nodes' AND collection='nodes') OR (namespace='access' AND collection='accounts') OR (namespace='commerce' AND collection IN ('plans','orders')) OR (namespace='redeem' AND collection IN ('codes','claims'))").all() as { namespace: string; collection: string; data: string }[]
  for (const row of rows) {
    const data = JSON.parse(row.data)
    add(data.groups)
    if (row.namespace === 'commerce' && row.collection === 'orders') add(data.plan?.groups)
    if (row.namespace === 'redeem' && row.collection === 'claims') add(data.reward?.grant?.groups)
  }
  return [...ids].sort().filter((id) => !existing.has(id)).map((id) => {
    const found = names.get(id)
    const marker = id.indexOf(':group:')
    const sourceId = found?.sourceId || (marker >= 0 ? id.slice(0, marker) : '')
    const suffix = marker >= 0 ? id.slice(marker + ':group:'.length) : ''
    const name = found?.name || (id === 'default' ? '默认分组' : marker >= 0 ? `${sourceId} · ${suffix ? '分组 ' + suffix : '未指定分组'}` : id)
    return { id, name, enabled: !(marker >= 0 && suffix === ''), ...(sourceId ? { sourceId } : {}) }
  })
}

function output(groups: Data[], applied: boolean, backup?: string) {
  process.stdout.write(JSON.stringify({ applied, groups, ...(backup ? { backup } : {}) }, null, 2) + '\n')
}
const reader = new DatabaseSync(database, { readOnly: true })
let groups: Data[]
try { reader.exec('BEGIN'); groups = plan(reader); reader.exec('ROLLBACK') } finally { reader.close() }
if (!values.apply || !groups.length) { output(groups, false); process.exit(0) }
process.umask(0o077)
const dir = database + '.group-backups'
mkdirSync(dir, { recursive: true, mode: 0o700 })
chmodSync(dir, 0o700)
const backup = join(dir, `before-groups-${new Date().toISOString().replaceAll(/[:.]/g, '-')}-${process.pid}.db`)
const db = new DatabaseSync(database)
try {
  db.exec('PRAGMA busy_timeout=5000')
  db.exec(`VACUUM INTO '${backup.replaceAll("'", "''")}'`)
  chmodSync(backup, 0o600)
  db.exec('BEGIN IMMEDIATE')
  groups = plan(db)
  const insert = db.prepare("INSERT INTO documents(namespace,collection,id,data,updated_at) VALUES('nodes','groups',?,?,?)")
  const now = new Date().toISOString()
  for (const group of groups) insert.run(group.id, JSON.stringify({ ...group, createdAt: now }), now)
  db.exec('COMMIT')
  output(groups, true, backup)
} catch (error) {
  if (db.isTransaction) db.exec('ROLLBACK')
  throw error
} finally { db.close() }
