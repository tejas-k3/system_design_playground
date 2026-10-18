// Shared by the browser scripts: find a Chrome/Chromium binary and the flags for a headless run.
// Set CHROME_BIN to override. On CI (CI=true) or with CHROME_NO_SANDBOX=1 the sandbox is turned off,
// which hosted Linux runners require.
import { existsSync } from 'node:fs'
import { execFileSync } from 'node:child_process'

export function chromePath() {
  if (process.env.CHROME_BIN) return process.env.CHROME_BIN
  const mac = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
  if (existsSync(mac)) return mac
  for (const name of ['google-chrome', 'google-chrome-stable', 'chromium', 'chromium-browser']) {
    try {
      return execFileSync('which', [name], { encoding: 'utf8' }).trim()
    } catch {
      /* try the next name */
    }
  }
  throw new Error('No Chrome found: install Chrome or set CHROME_BIN to a Chrome/Chromium binary')
}

export function chromeArgs(port, userDataDir) {
  const args = ['--headless=new', '--disable-gpu', `--remote-debugging-port=${port}`, `--user-data-dir=${userDataDir}`, '--no-first-run', '--disable-dev-shm-usage']
  if (process.env.CI || process.env.CHROME_NO_SANDBOX) args.push('--no-sandbox')
  args.push('about:blank')
  return args
}
