import { readFile } from 'node:fs/promises'
import { parseArgs } from 'node:util'

const { values } = parseArgs({
  options: {
    panel: { type: 'string' },
    input: { type: 'string' },
    apply: { type: 'boolean', default: false },
    help: { type: 'boolean', default: false },
  },
})

const usage = '用法：node tools/oauth-bindings/import.ts --panel https://panel.example.com --input bindings.json [--apply]，管理员访问密钥通过 PX_OAUTH_ADMIN_TOKEN 提供\n'
if (values.help || !values.panel || !values.input || !process.env.PX_OAUTH_ADMIN_TOKEN) {
  process.stdout.write(usage)
  process.exit(values.help ? 0 : 1)
}
const panel = new URL(values.panel)
if (panel.protocol !== 'https:' && !(panel.protocol === 'http:' && ['localhost', '127.0.0.1', '[::1]'].includes(panel.hostname))) {
  throw new Error('面板地址必须使用 HTTPS，本机开发可使用 HTTP')
}
const bindings: unknown = JSON.parse(await readFile(values.input, 'utf8'))
if (!Array.isArray(bindings)) throw new Error('输入需为登录绑定 JSON 数组')
const response = await fetch(new URL('/api/v1/extensions/oauth/bindings/import', panel), {
  method: 'POST',
  headers: {
    Authorization: `Bearer ${process.env.PX_OAUTH_ADMIN_TOKEN}`,
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({ bindings, dryRun: !values.apply }),
})
const result = await response.json() as { data?: unknown; error?: { message?: string } }
if (!response.ok) throw new Error(result.error?.message || `请求失败：${response.status}`)
process.stdout.write(JSON.stringify(result.data, null, 2) + '\n')
