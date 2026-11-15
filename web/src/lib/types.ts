export interface Template {
  id: string
  name: string
  imageUrl: string
  width: number
  height: number
  source: string
  animated: boolean
  createdAt: string
}

export interface CaptionLayout {
  /** Horizontal centre of the caption block, 0..1 of canvas width. */
  x: number
  /** Vertical centre of the caption block, 0..1 of canvas height. */
  y: number
  /** Font size as a fraction of canvas width. */
  size: number
}

export interface MemeStyle {
  fontScale: number
  color: string
  stroke: string
  uppercase: boolean
  outline: boolean
  top?: CaptionLayout
  bottom?: CaptionLayout
}

export const CAPTION_SIZE_MIN = 0.02
export const CAPTION_SIZE_MAX = 0.3
export const CAPTION_SIZE_DEFAULT = 0.095

export interface Meme {
  id: string
  title: string
  author: string
  imageUrl: string
  templateId?: string
  templateName?: string
  topText: string
  bottomText: string
  style: string
  width?: number
  height?: number
  score: number
  upvotes: number
  downvotes: number
  myVote: number
  animated: boolean
  createdAt: string
}

export interface Stats {
  memes: number
  votes: number
  templates: number
}

export type SortMode = 'hot' | 'new' | 'top'

export const DEFAULT_STYLE: MemeStyle = {
  fontScale: 1,
  color: '#ffffff',
  stroke: '#000000',
  uppercase: true,
  outline: true,
}

function parseCaptionLayout(raw: unknown): CaptionLayout | undefined {
  if (!raw || typeof raw !== 'object') return undefined
  const value = raw as Partial<CaptionLayout>
  const number = (input: unknown) =>
    typeof input === 'number' && Number.isFinite(input) ? input : 0
  const x = number(value.x)
  const y = number(value.y)
  const size = number(value.size)
  // Mirrors the Go sanitizer: an all-zero object means "no explicit layout".
  if (x === 0 && y === 0 && size === 0) return undefined
  return {
    x: Math.min(1, Math.max(0, x)),
    y: Math.min(1, Math.max(0, y)),
    size:
      size > 0
        ? Math.min(CAPTION_SIZE_MAX, Math.max(CAPTION_SIZE_MIN, size))
        : CAPTION_SIZE_DEFAULT,
  }
}

export function parseStyle(raw: string | undefined | null): MemeStyle {
  if (!raw) return { ...DEFAULT_STYLE }
  try {
    const parsed = JSON.parse(raw) as Partial<MemeStyle>
    return {
      fontScale:
        typeof parsed.fontScale === 'number'
          ? Math.min(1.6, Math.max(0.6, parsed.fontScale))
          : DEFAULT_STYLE.fontScale,
      color: /^#[0-9a-fA-F]{6}$/.test(parsed.color ?? '') ? parsed.color! : DEFAULT_STYLE.color,
      stroke: /^#[0-9a-fA-F]{6}$/.test(parsed.stroke ?? '') ? parsed.stroke! : DEFAULT_STYLE.stroke,
      uppercase: typeof parsed.uppercase === 'boolean' ? parsed.uppercase : DEFAULT_STYLE.uppercase,
      outline: typeof parsed.outline === 'boolean' ? parsed.outline : DEFAULT_STYLE.outline,
      top: parseCaptionLayout(parsed.top),
      bottom: parseCaptionLayout(parsed.bottom),
    }
  } catch {
    return { ...DEFAULT_STYLE }
  }
}
