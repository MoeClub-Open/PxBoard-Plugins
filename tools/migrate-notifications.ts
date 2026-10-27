import { chmodSync, existsSync, mkdirSync } from 'node:fs'
import { isAbsolute, join } from 'node:path'
import { parseArgs } from 'node:util'
import { DatabaseSync } from 'node:sqlite'

const { values } = parseArgs({ options: { database: { type: 'string' }, apply: { type: 'boolean', default: false }, help: { type: 'boolean', default: false } } })
if (values.help || !values.database) {
  process.stdout.write('用法：node tools/migrate-notifications.ts --database /绝对路径/pxboard.db [--apply]\n默认只预览；写入前停止面板服务。旧邮件记录保留，请升级用户服务并安装邮件通知插件后再启动。\n')
  process.exit(values.help ? 0 : 1)
}
const database = values.database
if (!isAbsolute(database) || !existsSync(database)) throw new Error('请提供已存在的数据库绝对路径')
type Row = { collection: string; id: string; data: string; updated_at: string }
function inspect(db: DatabaseSync) {
  const records = db.prepare("SELECT collection,id,data,updated_at FROM documents WHERE namespace='support' AND collection IN ('mail','notifications') ORDER BY collection,id").all() as Row[]
  const migrated = !!db.prepare("SELECT 1 FROM documents WHERE namespace='notifications' AND collection='migrations' AND id='support-mail'").get()
  const guard = !!db.prepare("SELECT 1 FROM documents WHERE namespace='support' AND collection='migrations' AND id='notifications'").get()
  const lookup = db.prepare("SELECT data FROM documents WHERE namespace='notifications' AND collection=? AND id=?")
  const missing: Row[] = []
  for (const row of migrated ? [] : records) {
    const target = lookup.get(row.collection, row.id) as { data: string } | undefined
    if (!target) missing.push(row)
    else if (target.data !== row.data) throw new Error(`目标邮件记录 ${row.collection}/${row.id} 已存在且内容不同，请先核对`)
  }
  const pending = db.prepare("SELECT d.event_id FROM deliveries d JOIN events e ON e.id=d.event_id WHERE d.plugin_id='support' AND d.done=0 AND e.type IN ('order.fulfilled','ticket.replied','auth.reset-requested') ORDER BY d.event_id").all()
  return { missing, pending: pending.length, sourceCount: records.length, migrated, guard }
}
const reader = new DatabaseSync(database, { readOnly: true })
let plan: ReturnType<typeof inspect>
try { reader.exec('BEGIN'); plan = inspect(reader); reader.exec('ROLLBACK') } finally { reader.close() }
if (!values.apply || (!plan.missing.length && !plan.pending && plan.guard)) {
  process.stdout.write(JSON.stringify({ applied: false, sourceRecords: plan.sourceCount, recordsToCopy: plan.missing.length, pendingEvents: plan.pending }) + '\n')
  process.exit(0)
}
process.umask(0o077)
const directory = database + '.notification-backups'
mkdirSync(directory, { recursive: true, mode: 0o700 })
chmodSync(directory, 0o700)
const backup = join(directory, `before-notifications-${new Date().toISOString().replaceAll(/[:.]/g, '-')}-${process.pid}.db`)
const db = new DatabaseSync(database)
try {
  db.exec('PRAGMA busy_timeout=5000')
  db.exec(`VACUUM INTO '${backup.replaceAll("'", "''")}'`)
  chmodSync(backup, 0o600)
  db.exec('BEGIN IMMEDIATE')
  plan = inspect(db)
  const insert = db.prepare("INSERT INTO documents(namespace,collection,id,data,updated_at) VALUES('notifications',?,?,?,?)")
  for (const row of plan.missing) insert.run(row.collection, row.id, row.data, row.updated_at)
  if (!plan.migrated) {
    const now = new Date().toISOString()
    db.prepare("INSERT INTO documents(namespace,collection,id,data,updated_at) VALUES('notifications','migrations','support-mail',?,?)").run(JSON.stringify({ id: 'support-mail', createdAt: now }), now)
  }
  if (!plan.guard) {
    const now = new Date().toISOString()
    db.prepare("INSERT INTO documents(namespace,collection,id,data,updated_at) VALUES('support','migrations','notifications',?,?)").run(JSON.stringify({ id: 'notifications', createdAt: now }), now)
  }
  db.exec("INSERT OR IGNORE INTO deliveries(event_id,plugin_id,attempts,error,next_at,done) SELECT d.event_id,'notifications',d.attempts,d.error,d.next_at,0 FROM deliveries d JOIN events e ON e.id=d.event_id WHERE d.plugin_id='support' AND d.done=0 AND e.type IN ('order.fulfilled','ticket.replied','auth.reset-requested')")
  db.exec("UPDATE deliveries SET done=1,error='' WHERE plugin_id='support' AND done=0 AND event_id IN (SELECT id FROM events WHERE type IN ('order.fulfilled','ticket.replied','auth.reset-requested'))")
  db.exec('COMMIT')
  process.stdout.write(JSON.stringify({ applied: true, sourceRecords: plan.sourceCount, copied: plan.missing.length, pendingEvents: plan.pending, backup }) + '\n')
} catch (error) {
  if (db.isTransaction) db.exec('ROLLBACK')
  throw error
} finally { db.close() }
