import { existsSync } from 'node:fs'
import { isAbsolute } from 'node:path'
import { parseArgs } from 'node:util'
import { DatabaseSync } from 'node:sqlite'

const { values } = parseArgs({ options: { database: { type: 'string' }, apply: { type: 'boolean', default: false } } })
if (!values.database || !isAbsolute(values.database) || !existsSync(values.database)) throw new Error('请指定 --database 数据库绝对路径；默认仅预览，写入前停止面板服务')
const db = new DatabaseSync(values.database, { readOnly: !values.apply })
type Document = { id: string; data: string }
try {
  db.exec(values.apply ? 'BEGIN IMMEDIATE' : 'BEGIN')
  const accounts = db.prepare("SELECT id,data FROM documents WHERE namespace='access' AND collection='accounts'").all() as Document[]
  const users = new Map<string, string>()
  for (const record of accounts) {
    const account = JSON.parse(record.data)
    if (['xboard', 'v2board'].includes(account.source?.system)) users.set(`${account.source.sourceId}:${account.source.recordId}`, account.userId)
  }
  const archived = db.prepare("SELECT id,data FROM documents WHERE collection='archive' AND namespace IN ('import-xboard','import-v2board') AND json_extract(data,'$.table')='v2_user'").all() as Document[]
  const existing = db.prepare("SELECT 1 FROM documents WHERE namespace='access' AND collection='preferences' AND id=?")
  const insert = values.apply ? db.prepare("INSERT INTO documents VALUES('access','preferences',?,?,?)") : undefined
  let missing = 0
  for (const record of archived) {
    const archive = JSON.parse(record.data)
    const userId = users.get(`${archive.runSource}:${archive.row.id}`)
    if (!userId || existing.get(userId)) continue
    const enabled = (value: unknown) => value === undefined || value === null || (value === true || value === 1 || value === '1')
    const data = { id: userId, expireReminder: enabled(archive.row.remind_expire), trafficReminder: enabled(archive.row.remind_traffic) }
    insert?.run(userId, JSON.stringify(data), new Date().toISOString())
    users.delete(`${archive.runSource}:${archive.row.id}`)
    missing++
  }
  db.exec(values.apply ? 'COMMIT' : 'ROLLBACK')
  process.stdout.write(JSON.stringify({ applied: values.apply, preferences: missing }) + '\n')
} catch (error) {
  if (db.isTransaction) db.exec('ROLLBACK')
  throw error
} finally { db.close() }
