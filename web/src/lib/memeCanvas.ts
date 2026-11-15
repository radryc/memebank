import type { CaptionLayout, MemeStyle } from './types'
import { CAPTION_SIZE_DEFAULT, CAPTION_SIZE_MAX, CAPTION_SIZE_MIN } from './types'

const FONT_STACK = `Anton, "Arial Black", Impact, "Helvetica Neue", sans-serif`
const MAX_EXPORT_SIDE = 1600
const LINE_HEIGHT_RATIO = 1.08
const MAX_BLOCK_HEIGHT_RATIO = 0.42

export function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.crossOrigin = 'anonymous'
    img.onload = () => resolve(img)
    img.onerror = () => reject(new Error('Could not load that image'))
    img.src = src
  })
}

/** Wait for the display font so canvas text matches what the user sees. */
export async function ensureFonts(): Promise<void> {
  try {
    await Promise.all([document.fonts.load(`400 96px Anton`), document.fonts.ready])
  } catch {
    // Drawing falls back to Arial Black / Impact.
  }
}

function fontFor(size: number): string {
  return `400 ${size}px ${FONT_STACK}`
}

function wrapText(ctx: CanvasRenderingContext2D, text: string, maxWidth: number): string[] {
  const lines: string[] = []
  for (const paragraph of text.split('\n')) {
    const words = paragraph.trim().split(/\s+/).filter(Boolean)
    if (words.length === 0) {
      lines.push('')
      continue
    }
    let line = words[0]
    for (let i = 1; i < words.length; i += 1) {
      const candidate = `${line} ${words[i]}`
      if (ctx.measureText(candidate).width <= maxWidth) {
        line = candidate
      } else {
        lines.push(line)
        line = words[i]
      }
    }
    lines.push(line)
  }
  return lines
}

/** One laid-out caption block, in canvas pixels. */
export interface CaptionBlock {
  lines: string[]
  size: number
  lineHeight: number
  centerX: number
  topY: number
  height: number
  width: number
}

export interface CaptionBoxes {
  top: CaptionBlock | null
  bottom: CaptionBlock | null
}

function clampCaptionBox(
  centerX: number,
  centerY: number,
  blockWidth: number,
  blockHeight: number,
  width: number,
  height: number,
): [number, number] {
  const halfW = blockWidth / 2
  const halfH = blockHeight / 2
  const x =
    blockWidth >= width ? width / 2 : Math.min(width - halfW, Math.max(halfW, centerX))
  const y =
    blockHeight >= height ? halfH : Math.min(height - halfH, Math.max(halfH, centerY))
  return [x, y - halfH]
}

function newCaptionBlock(
  ctx: CanvasRenderingContext2D,
  text: string,
  width: number,
  height: number,
  layout: CaptionLayout | undefined,
  size: number,
  maxWidth: number,
  isTop: boolean,
): CaptionBlock {
  ctx.font = fontFor(size)
  const lines = wrapText(ctx, text, maxWidth)
  const widest = lines.reduce((max, line) => Math.max(max, ctx.measureText(line).width), 0)
  const blockHeight = lines.length * size * LINE_HEIGHT_RATIO
  const lineHeight = size * LINE_HEIGHT_RATIO
  if (layout) {
    const [centerX, topY] = clampCaptionBox(
      layout.x * width,
      layout.y * height,
      widest,
      blockHeight,
      width,
      height,
    )
    return { lines, size, lineHeight, centerX, topY, height: blockHeight, width: widest }
  }
  const pad = height * 0.028
  return {
    lines,
    size,
    lineHeight,
    centerX: width / 2,
    topY: isTop ? pad : height - pad - blockHeight,
    height: blockHeight,
    width: widest,
  }
}

/** Mirrors the Go auto-fit loop. Positioned captions keep their own size. */
function fitCaptionSize(
  ctx: CanvasRenderingContext2D,
  topText: string,
  bottomText: string,
  width: number,
  height: number,
  style: MemeStyle,
): number {
  const maxWidth = width * 0.92
  const min = Math.max(12, width * 0.032)
  let size = Math.max(min, width * CAPTION_SIZE_DEFAULT * style.fontScale)

  for (let guard = 0; guard < 40; guard += 1) {
    ctx.font = fontFor(size)
    let fits = true
    const check = (text: string, layout?: CaptionLayout) => {
      if (layout || !text) return
      const lines = wrapText(ctx, text, maxWidth)
      if (lines.length * size * LINE_HEIGHT_RATIO > height * MAX_BLOCK_HEIGHT_RATIO) {
        fits = false
        return
      }
      for (const line of lines) {
        if (ctx.measureText(line).width > width * 0.94) {
          fits = false
          return
        }
      }
    }
    check(topText, style.top)
    check(bottomText, style.bottom)
    if (fits) return size
    if (size <= min) return size
    size = Math.max(min, size - Math.max(1, Math.round(size * 0.055)))
  }
  return size
}

/**
 * Lays out both caption blocks in canvas pixels. Shared by the renderer and
 * the editor overlay so drag handles line up with the painted text.
 */
export function layoutCaptions(
  ctx: CanvasRenderingContext2D,
  topText: string,
  bottomText: string,
  width: number,
  height: number,
  style: MemeStyle,
): CaptionBoxes {
  const upper = (value: string) => (style.uppercase ? value.toUpperCase() : value)
  const top = upper(topText).trim()
  const bottom = upper(bottomText).trim()
  if (!top && !bottom) return { top: null, bottom: null }

  const autoSize = fitCaptionSize(ctx, top, bottom, width, height, style)
  const maxWidth = width * 0.92
  return {
    top: top
      ? newCaptionBlock(
          ctx,
          top,
          width,
          height,
          style.top,
          style.top ? style.top.size * width : autoSize,
          maxWidth,
          true,
        )
      : null,
    bottom: bottom
      ? newCaptionBlock(
          ctx,
          bottom,
          width,
          height,
          style.bottom,
          style.bottom ? style.bottom.size * width : autoSize,
          maxWidth,
          false,
        )
      : null,
  }
}

/** Draws an image plus meme captions onto a canvas. */
export function renderMeme(
  canvas: HTMLCanvasElement,
  image: HTMLImageElement,
  topText: string,
  bottomText: string,
  style: MemeStyle,
): void {
  const naturalWidth = image.naturalWidth || image.width
  const naturalHeight = image.naturalHeight || image.height
  if (!naturalWidth || !naturalHeight) return

  const scale = Math.min(1, MAX_EXPORT_SIDE / Math.max(naturalWidth, naturalHeight))
  const width = Math.max(1, Math.round(naturalWidth * scale))
  const height = Math.max(1, Math.round(naturalHeight * scale))
  if (canvas.width !== width) canvas.width = width
  if (canvas.height !== height) canvas.height = height

  const ctx = canvas.getContext('2d')
  if (!ctx) return

  ctx.clearRect(0, 0, width, height)
  ctx.imageSmoothingEnabled = true
  ctx.imageSmoothingQuality = 'high'
  ctx.drawImage(image, 0, 0, width, height)

  drawCaptions(ctx, width, height, topText, bottomText, style)
}

/** Draws the caption blocks on a canvas that already contains the artwork. */
export function drawCaptions(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  topText: string,
  bottomText: string,
  style: MemeStyle,
): void {
  const boxes = layoutCaptions(ctx, topText, bottomText, width, height, style)
  if (!boxes.top && !boxes.bottom) return

  ctx.textAlign = 'center'
  ctx.textBaseline = 'top'
  ctx.lineJoin = 'round'
  ctx.miterLimit = 2
  ctx.fillStyle = style.color
  ctx.strokeStyle = style.stroke

  for (const block of [boxes.top, boxes.bottom]) {
    if (!block) continue
    ctx.font = fontFor(block.size)
    ctx.lineWidth = Math.max(2, block.size * 0.1)
    block.lines.forEach((line, index) => {
      if (!line) return
      const x = block.centerX
      const y = block.topY + index * block.lineHeight
      if (style.outline) {
        ctx.strokeText(line, x, y, width * 0.94)
      }
      ctx.fillText(line, x, y, width * 0.94)
    })
  }
}

/** Clamps a user-supplied size (fraction of canvas width). */
export function clampCaptionSize(size: number): number {
  return Math.min(CAPTION_SIZE_MAX, Math.max(CAPTION_SIZE_MIN, size))
}

export function canvasToBlob(canvas: HTMLCanvasElement): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob) resolve(blob)
      else reject(new Error('Could not export the meme image'))
    }, 'image/png')
  })
}
