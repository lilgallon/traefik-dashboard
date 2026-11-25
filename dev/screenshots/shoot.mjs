// README screenshots, taken from the local environment (make dev).
// Run with `make screenshots`: writes PNG files to docs/screenshots/.
import { chromium } from 'playwright'

const base = process.env.DASHBOARD_URL ?? 'http://localhost:18080'
const out = process.env.OUT ?? '../../docs/screenshots'

// Wait until the backfill is over and the page shows data.
async function waitReady(page) {
  await page.waitForFunction(() => document.querySelectorAll('section.card').length > 2, null, { timeout: 60_000 })
  await page.waitForTimeout(2500)
}

async function open(browser, { theme, window, hash = '', width = 1440, height = 900 }) {
  const ctx = await browser.newContext({
    viewport: { width, height },
    deviceScaleFactor: 2,
    locale: 'fr-FR',
    timezoneId: 'Europe/Paris',
    colorScheme: theme,
  })
  await ctx.addInitScript(([t, w]) => {
    localStorage.setItem('td-theme', t)
    localStorage.setItem('td-window', w)
  }, [theme, window])
  const page = await ctx.newPage()
  await page.goto(base + '/' + hash)
  await waitReady(page)
  return { ctx, page }
}

// CrowdSec bans scan bursts as they arrive (one every 90 s): wait for a few bans
// so the CrowdSec panel is not empty (MIN_BANS=0 to skip).
const minBans = Number(process.env.MIN_BANS ?? 3)
for (let waited = 0; ; waited += 15) {
  let bans = 0
  try {
    const r = await fetch(`${base}/api/summary?window=24h`)
    bans = (await r.json()).crowdsec?.bans?.length ?? 0
  } catch {}
  if (bans >= minBans) break
  if (waited >= 900) {
    console.log(`only ${bans} CrowdSec ban(s) after 15 min, taking screenshots anyway`)
    break
  }
  if (waited % 60 === 0) console.log(`waiting for CrowdSec bans: ${bans}/${minBans}`)
  await new Promise((res) => setTimeout(res, 15_000))
}

const browser = await chromium.launch()
const shots = [
  // Red: an unknown IP got a success in the last 24 hours.
  { file: 'overview-light.png', theme: 'light', window: '24h' },
  // Green: nothing unknown in the last hour.
  { file: 'overview-dark.png', theme: 'dark', window: '1h' },
  { file: 'ip-detail.png', theme: 'light', window: '24h', hash: '#ip=216.160.83.66' },
  { file: 'crowdsec.png', theme: 'light', window: '24h', selector: 'section[aria-labelledby="cs-title"]' },
  { file: 'ips.png', theme: 'dark', window: '7d', selector: 'section.card:has(.segmented[aria-label="Filtrer les adresses"])' },
]
for (const s of shots) {
  const { ctx, page } = await open(browser, s)
  const path = `${out}/${s.file}`
  if (s.selector) {
    await page.locator(s.selector).screenshot({ path })
  } else {
    await page.screenshot({ path })
  }
  console.log('écrit', path)
  await ctx.close()
}
await browser.close()
