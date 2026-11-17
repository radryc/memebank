const imageTypes = ['image/png', 'image/jpeg', 'image/webp', 'image/gif'] as const

const extensionByType: Record<string, string> = {
  'image/png': 'png',
  'image/jpeg': 'jpg',
  'image/webp': 'webp',
  'image/gif': 'gif',
}

/** Decodes a base64 data: URL into a File, if it is a supported image. */
function fileFromDataURL(source: string | undefined): File | null {
  if (!source) return null
  const match = /^data:(image\/(?:png|jpeg|webp|gif));base64,([a-z0-9+/=\s]+)$/i.exec(source.trim())
  if (!match) return null
  try {
    const binary = atob(match[2].replace(/\s/g, ''))
    const bytes = new Uint8Array(binary.length)
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
    const type = match[1].toLowerCase()
    return new File([bytes], `clipboard.${extensionByType[type] ?? 'png'}`, { type })
  } catch {
    return null
  }
}

/** Some apps put an <img src="data:image/..."> tag on the clipboard. */
function fileFromHTML(html: string | undefined): File | null {
  if (!html) return null
  const match = /<img[^>]+src=["']([^"']+)["']/i.exec(html)
  return match ? fileFromDataURL(match[1]) : null
}

/** Returns the first image file contained in a clipboard or drop payload. */
export function imageFromDataTransfer(data: DataTransfer | null): File | null {
  if (!data) return null
  for (const item of Array.from(data.items ?? [])) {
    if (item.kind === 'file' && item.type.startsWith('image/')) {
      const file = item.getAsFile()
      if (file) return file
    }
  }
  for (const file of Array.from(data.files ?? [])) {
    if (file.type.startsWith('image/')) return file
  }
  // Fallbacks for clipboards that expose the image as a data URL rather than
  // as a file item (common with screenshot and rich-text tools).
  return fileFromDataURL(data.getData('text/plain')) ?? fileFromHTML(data.getData('text/html'))
}

export function imageFromClipboard(event: ClipboardEvent): File | null {
  return imageFromDataTransfer(event.clipboardData)
}

/**
 * Reads an image directly from the async clipboard. Used by the Paste button.
 * Returns null when the clipboard has no supported image, and throws when the
 * browser cannot expose clipboard contents (unsupported or permission denied);
 * callers should then suggest pressing ⌘V / Ctrl+V.
 */
export async function readImageFromClipboard(): Promise<File | null> {
  if (typeof navigator === 'undefined' || !navigator.clipboard?.read) {
    throw new Error('clipboard reads are not supported in this browser')
  }

  const items = await navigator.clipboard.read()
  for (const item of items) {
    const type = imageTypes.find((candidate) => item.types.includes(candidate))
    if (!type) continue
    const blob = await item.getType(type)
    const extension = extensionByType[type] ?? 'png'
    return new File([blob], `clipboard.${extension}`, { type })
  }
  return null
}
