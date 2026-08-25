import { spawn } from 'node:child_process'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const services = [
  ['api', 'go', ['run', './cmd/api'], path.join(root, 'apps/server')],
  ['mcp', process.execPath, [path.join(root, 'apps/mcp/dist/index.js')], root],
]

let stopping = false
const children = services.map(([name, command, args, cwd]) => {
  const child = spawn(command, args, { cwd, stdio: 'inherit' })
  child.on('error', (error) => {
    console.error(`[${name}] failed to start: ${error.message}`)
    shutdown(1)
  })
  child.on('exit', (code, signal) => {
    if (!stopping) {
      console.error(`[${name}] stopped (${signal ?? `exit ${code}`})`)
      shutdown(code || 1)
    }
  })
  return child
})

function shutdown(code) {
  if (stopping) return
  stopping = true
  process.exitCode = code
  for (const child of children) {
    if (child.exitCode !== null || child.signalCode !== null) continue
    if (process.platform === 'win32') {
      spawn('taskkill', ['/pid', String(child.pid), '/t', '/f'], { stdio: 'ignore' })
    } else {
      child.kill('SIGTERM')
    }
  }
}

process.once('SIGINT', () => shutdown(0))
process.once('SIGTERM', () => shutdown(0))
