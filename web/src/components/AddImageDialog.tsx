import { useCallback, useEffect, useRef, useState, type DragEvent } from 'react'
import {
  imageFromClipboard,
  imageFromDataTransfer,
  readImageFromClipboard,
} from '../lib/clipboard'
import { Clipboard, ImageIcon, Loader, Upload, X } from './Icons'

export type ImageSource = 'upload' | 'paste'

/**
 * Small modal that turns an image file, a drag-and-drop, a clipboard paste
 * (⌘V / Ctrl+V) or a click on "Paste image" into a single upload callback.
 */
export default function AddImageDialog({
  open,
  onClose,
  onFile,
}: {
  open: boolean
  onClose: () => void
  onFile: (file: File, source: ImageSource) => void
}) {
  const fileRef = useRef<HTMLInputElement>(null)
  const panelRef = useRef<HTMLDivElement>(null)
  const [error, setError] = useState('')
  const [reading, setReading] = useState(false)
  const [dragging, setDragging] = useState(false)

  useEffect(() => {
    if (!open) return
    setError('')
    setDragging(false)
    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    panelRef.current?.focus()
    return () => {
      document.body.style.overflow = previousOverflow
    }
  }, [open])

  const submit = useCallback(
    (file: File, source: ImageSource) => {
      onFile(file, source)
      onClose()
    },
    [onFile, onClose],
  )

  // Raw image data on the clipboard arrives as an image file item, so Ctrl+V
  // works even when the clipboard has no filename or file path.
  useEffect(() => {
    if (!open) return
    const onPaste = (event: ClipboardEvent) => {
      const file = imageFromClipboard(event)
      if (!file) {
        if (event.clipboardData?.types.length) {
          setError('That clipboard does not contain an image. Copy an image and try again.')
        }
        return
      }
      event.preventDefault()
      submit(file, 'paste')
    }
    window.addEventListener('paste', onPaste)
    return () => window.removeEventListener('paste', onPaste)
  }, [open, submit])

  useEffect(() => {
    if (!open) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [open, onClose])

  const pasteFromClipboard = async () => {
    setError('')
    setReading(true)
    try {
      const file = await readImageFromClipboard()
      if (!file) {
        setError('No image on the clipboard. Copy an image first, then try again.')
        return
      }
      submit(file, 'paste')
    } catch {
      setError('The browser blocked clipboard access. Press ⌘V / Ctrl+V instead.')
    } finally {
      setReading(false)
    }
  }

  const drop = (event: DragEvent<HTMLElement>) => {
    event.preventDefault()
    setDragging(false)
    const file = imageFromDataTransfer(event.dataTransfer)
    if (!file) {
      setError('Drop an image file (png, jpeg, webp or gif).')
      return
    }
    submit(file, 'upload')
  }

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-end justify-center bg-ink-950/80 p-4 backdrop-blur-sm sm:items-center"
      onMouseDown={onClose}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="add-image-title"
        tabIndex={-1}
        className="panel w-full max-w-lg p-6 shadow-2xl focus:outline-none"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 id="add-image-title" className="text-lg font-bold text-white">
              Add an image
            </h2>
            <p className="mt-1 text-sm text-zinc-500">
              Upload a file, drop it below, or paste with ⌘V / Ctrl+V.
            </p>
          </div>
          <button type="button" onClick={onClose} className="btn-icon size-8" aria-label="Close">
            <X className="size-4" />
          </button>
        </div>

        <button
          type="button"
          onClick={() => fileRef.current?.click()}
          onDragEnter={(event) => {
            event.preventDefault()
            setDragging(true)
          }}
          onDragOver={(event) => {
            event.preventDefault()
            setDragging(true)
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={drop}
          className={`mt-5 flex w-full cursor-pointer flex-col items-center gap-2 rounded-2xl border border-dashed px-6 py-10 text-center transition ${
            dragging
              ? 'border-acid-400/70 bg-acid-400/[0.08]'
              : 'border-white/15 bg-ink-900/60 hover:border-acid-400/40 hover:bg-white/[0.03]'
          }`}
        >
          <span className="grid size-12 place-items-center rounded-2xl border border-acid-400/20 bg-acid-400/10 text-acid-400">
            <ImageIcon className="size-6" />
          </span>
          <span className="text-sm font-semibold text-white">
            {dragging ? 'Drop the image here' : 'Drag an image here'}
          </span>
          <span className="text-xs text-zinc-500">
            png, jpeg, webp or gif · or click to choose a file
          </span>
        </button>

        <div className="mt-3 grid gap-2 sm:grid-cols-2">
          <button
            type="button"
            onClick={() => fileRef.current?.click()}
            className="btn-secondary w-full py-2.5"
          >
            <Upload className="size-4" />
            Choose file
          </button>
          <button
            type="button"
            onClick={() => void pasteFromClipboard()}
            disabled={reading}
            className="btn-secondary w-full py-2.5"
          >
            {reading ? <Loader className="size-4" /> : <Clipboard className="size-4" />}
            {reading ? 'Reading clipboard…' : 'Paste image'}
          </button>
        </div>

        {error && (
          <p className="mt-4 rounded-xl border border-hot-500/30 bg-hot-500/10 px-3 py-2 text-xs text-hot-300">
            {error}
          </p>
        )}

        <p className="mt-4 text-[11px] leading-relaxed text-zinc-600">
          Pasting sends the image itself — no need to save it to a file first. Duplicate images reuse
          the existing template.
        </p>

        <input
          ref={fileRef}
          type="file"
          accept="image/png,image/jpeg,image/webp,image/gif"
          className="hidden"
          onChange={(event) => {
            const file = event.target.files?.[0]
            event.target.value = ''
            if (file) submit(file, 'upload')
          }}
        />
      </div>
    </div>
  )
}
