// Mise en forme française des nombres, volumes et durées.

/** Espace fine insécable : milliers, et entre un nombre et son unité. */
const NNBSP = ' '

function decimal(n: number, digits: number): string {
  return n.toFixed(digits).replace(/\.?0+$/, '').replace('.', ',')
}

export function formatCount(n: number): string {
  if (!Number.isFinite(n)) return '0'
  return Math.round(n).toString().replace(/\B(?=(\d{3})+(?!\d))/g, NNBSP)
}

const UNITS = ['o', 'Ko', 'Mo', 'Go', 'To']

/** Unités en base 1000 : 1536 → « 1,5 Ko ». */
export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return `0${NNBSP}o`
  let v = n
  let i = 0
  while (v >= 1000 && i < UNITS.length - 1) {
    v /= 1000
    i++
  }
  const text = i === 0 || v >= 100 ? Math.round(v).toString() : decimal(v, 1)
  return `${text}${NNBSP}${UNITS[i]}`
}

/** Requêtes par seconde : 1,3 · 38 · 0,04. */
export function formatRps(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return '0'
  if (n < 0.01) return '< 0,01'
  if (n < 0.1) return decimal(n, 2)
  if (n < 10) return decimal(n, 1)
  return formatCount(n)
}

export function formatMs(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '—'
  if (ms < 1) return `< 1${NNBSP}ms`
  if (ms < 1000) return `${Math.round(ms)}${NNBSP}ms`
  return `${decimal(ms / 1000, 1)}${NNBSP}s`
}

/** Durée d'une requête : comme formatMs, puis « 4 min », « 2 h 05 » pour une connexion longue. */
export function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms < 60_000) return formatMs(ms)
  const min = Math.floor(ms / 60_000)
  if (min < 60) return `${min}${NNBSP}min`
  return `${Math.floor(min / 60)}${NNBSP}h${NNBSP}${String(min % 60).padStart(2, '0')}`
}

/** « il y a 12 s », « il y a 3 min ». Date absente ou invalide : « jamais ». */
export function ago(t: string, now: Date): string {
  const ms = Date.parse(t)
  if (!t || Number.isNaN(ms) || new Date(ms).getUTCFullYear() < 2000) return 'jamais'
  const s = Math.floor((now.getTime() - ms) / 1000)
  if (s < 2) return "à l'instant"
  if (s < 60) return `il y a ${s} s`
  if (s < 3600) return `il y a ${Math.floor(s / 60)} min`
  if (s < 86400) {
    const h = Math.floor(s / 3600)
    const m = Math.floor((s % 3600) / 60)
    return m === 0 ? `il y a ${h} h` : `il y a ${h} h ${String(m).padStart(2, '0')}`
  }
  return `il y a ${Math.floor(s / 86400)} j`
}

/** Temps restant : « dans 3 h 05 », « dans 12 min ». Passé ou absent : « expiré ». */
export function remaining(t: string, now: Date): string {
  const ms = Date.parse(t)
  if (!t || Number.isNaN(ms) || new Date(ms).getUTCFullYear() < 2000) return '—'
  const s = Math.floor((ms - now.getTime()) / 1000)
  if (s <= 0) return 'expiré'
  if (s < 60) return `dans ${s} s`
  if (s < 3600) return `dans ${Math.floor(s / 60)} min`
  if (s < 2 * 86400) {
    const h = Math.floor(s / 3600)
    const m = Math.floor((s % 3600) / 60)
    return m === 0 ? `dans ${h} h` : `dans ${h} h ${String(m).padStart(2, '0')}`
  }
  return `dans ${Math.floor(s / 86400)} j`
}

const two = (n: number) => String(n).padStart(2, '0')

/** Heure locale du navigateur, HH:MM:SS. */
export function formatClock(d: Date): string {
  return `${two(d.getHours())}:${two(d.getMinutes())}:${two(d.getSeconds())}`
}

/** Jour et heure locaux : « 10/11 09:05 ». */
export function formatDayTime(d: Date): string {
  return `${two(d.getDate())}/${two(d.getMonth() + 1)} ${two(d.getHours())}:${two(d.getMinutes())}`
}

let regions: Intl.DisplayNames | undefined

/** « FR » → « France ». Code inconnu : rendu tel quel. */
export function countryName(code: string): string {
  if (!code) return ''
  try {
    regions ??= new Intl.DisplayNames(['fr'], { type: 'region' })
    return regions.of(code) ?? code
  } catch {
    return code
  }
}

export function pluriel(n: number, singulier: string, pluriel: string): string {
  return `${formatCount(n)} ${n < 2 ? singulier : pluriel}`
}

export function percent(part: number, whole: number): string {
  if (whole <= 0 || part <= 0) return '0 %'
  const p = (part / whole) * 100
  if (p < 0.1) return '< 0,1 %'
  return `${decimal(p, p < 10 ? 1 : 0)} %`
}
