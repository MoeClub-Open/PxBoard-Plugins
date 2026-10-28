import { DatabaseSync } from 'node:sqlite'
import { resolve } from 'node:path'

const args = process.argv.slice(2)
const index = args.indexOf('--database')
if (index < 0 || !args[index + 1]) throw new Error('请使用 --database 指定面板数据库')
const apply = args.includes('--apply')
const database = new DatabaseSync(resolve(args[index + 1]), { readOnly: !apply })
database.exec('PRAGMA busy_timeout=5000')
try {
  if (apply) database.exec('BEGIN IMMEDIATE')
  const originals = new Map<string, string>()
  const archives = database.prepare(`
    SELECT namespace, json_extract(data,'$.runSource') AS source,
      CAST(json_extract(data,'$.row.id') AS TEXT) AS templateId,
      json_extract(data,'$.row') AS content
    FROM documents WHERE namespace IN ('import-xboard','import-v2board')
      AND collection='archive' AND json_extract(data,'$.table')='v2_gift_card_template'
  `).all()
  for (const row of archives) {
    originals.set(`${row.namespace}\0${row.source}\0${row.templateId}`, String(row.content))
  }
  const rows = database.prepare(`
    SELECT id, data, json_extract(data,'$.source.system') AS system,
      json_extract(data,'$.source.sourceId') AS source,
      CAST(json_extract(data,'$.sourceContent.template_id') AS TEXT) AS templateId,
      json_extract(data,'$.sourceTemplate') AS content
    FROM documents WHERE namespace='redeem' AND collection='codes'
      AND json_type(data,'$.sourceTemplate')='object'
  `).all()
  const update = apply ? database.prepare(`
    UPDATE documents SET data=json_remove(data,'$.sourceTemplate')
    WHERE namespace='redeem' AND collection='codes' AND id=? AND data=?
  `) : undefined
  let matched = 0
  for (const row of rows) {
    const original = originals.get(`import-${row.system}\0${row.source}\0${row.templateId}`)
    if (original === undefined || original !== row.content) continue
    matched++
    if (update && update.run(row.id, row.data).changes !== 1) {
      throw new Error('兑换码在清理期间发生变化，操作已回滚')
    }
  }
  if (apply) database.exec('COMMIT')
  console.log(JSON.stringify({ apply, duplicateCopies: matched, preservedUnmatched: rows.length - matched }))
} catch (error) {
  if (apply) database.exec('ROLLBACK')
  throw error
} finally {
  database.close()
}
