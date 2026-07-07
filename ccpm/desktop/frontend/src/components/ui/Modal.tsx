import { useEffect, useState } from 'react'
import { cn } from '@/lib/utils'

export function Modal({
  open,
  onClose,
  title,
  children,
}: {
  open: boolean
  onClose: () => void
  title: string
  children: React.ReactNode
}) {
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose])

  if (!open) return null
  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/50" onClick={onClose} />
      <div className="relative z-10 w-[420px] max-w-full rounded-xl border border-border bg-popover p-5 shadow-xl">
        <h2 className="text-sm font-semibold">{title}</h2>
        <div className="mt-3">{children}</div>
      </div>
    </div>
  )
}

/** Prompt for a single text value (e.g. a new profile name). */
export function PromptModal({
  open,
  title,
  label,
  placeholder,
  initial,
  confirmLabel,
  onCancel,
  onConfirm,
  validate,
}: {
  open: boolean
  title: string
  label: string
  placeholder?: string
  initial?: string
  confirmLabel: string
  onCancel: () => void
  onConfirm: (value: string) => void
  validate?: (v: string) => string | null
}) {
  const [value, setValue] = useState(initial ?? '')
  useEffect(() => {
    if (open) setValue(initial ?? '')
  }, [open, initial])

  const err = validate ? validate(value.trim()) : null
  const canSubmit = value.trim().length > 0 && !err

  return (
    <Modal open={open} onClose={onCancel} title={title}>
      <label className="text-xs text-muted-foreground">{label}</label>
      <input
        autoFocus
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => e.key === 'Enter' && canSubmit && onConfirm(value.trim())}
        placeholder={placeholder}
        className="mt-1.5 w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
      {err && <p className="mt-1.5 text-xs text-destructive">{err}</p>}
      <div className="mt-4 flex justify-end gap-2">
        <GhostButton onClick={onCancel}>Cancel</GhostButton>
        <PrimaryButton disabled={!canSubmit} onClick={() => onConfirm(value.trim())}>
          {confirmLabel}
        </PrimaryButton>
      </div>
    </Modal>
  )
}

/** Confirm a destructive action; requires typing the name to enable. */
export function ConfirmModal({
  open,
  title,
  message,
  confirmLabel,
  requireText,
  onCancel,
  onConfirm,
}: {
  open: boolean
  title: string
  message: string
  confirmLabel: string
  requireText?: string
  onCancel: () => void
  onConfirm: () => void
}) {
  const [typed, setTyped] = useState('')
  useEffect(() => {
    if (open) setTyped('')
  }, [open])
  const ok = !requireText || typed === requireText

  return (
    <Modal open={open} onClose={onCancel} title={title}>
      <p className="text-sm text-muted-foreground">{message}</p>
      {requireText && (
        <input
          autoFocus
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
          placeholder={`type "${requireText}" to confirm`}
          className="mt-3 w-full rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      )}
      <div className="mt-4 flex justify-end gap-2">
        <GhostButton onClick={onCancel}>Cancel</GhostButton>
        <button
          disabled={!ok}
          onClick={onConfirm}
          className="cursor-pointer rounded-md bg-destructive px-3 py-1.5 text-xs font-medium text-destructive-foreground transition-colors hover:bg-destructive/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default disabled:opacity-50"
        >
          {confirmLabel}
        </button>
      </div>
    </Modal>
  )
}

function GhostButton({ children, onClick }: { children: React.ReactNode; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="cursor-pointer rounded-md px-3 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      {children}
    </button>
  )
}

function PrimaryButton({
  children,
  onClick,
  disabled,
}: {
  children: React.ReactNode
  onClick: () => void
  disabled?: boolean
}) {
  return (
    <button
      disabled={disabled}
      onClick={onClick}
      className={cn(
        'cursor-pointer rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-default disabled:opacity-50',
      )}
    >
      {children}
    </button>
  )
}
