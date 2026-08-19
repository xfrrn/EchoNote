import type { PropsWithChildren, ReactNode } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'

interface SheetProps extends PropsWithChildren {
  open: boolean
  onOpenChange: (open: boolean) => void
  title?: ReactNode
  description?: string
}

export function Sheet({ open, onOpenChange, title, description, children }: SheetProps) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-overlay data-[state=open]:animate-[fade-in_220ms_cubic-bezier(.2,.8,.2,1)_both]" />
        <Dialog.Content
          className="safe-sides fixed inset-x-0 bottom-0 z-50 mx-auto w-full max-w-app rounded-t-xl glass-surface shadow-sheet safe-bottom outline-none data-[state=open]:animate-slide-up"
        >
          <div className="mx-auto mt-2 h-1 w-9 rounded-full bg-hairline" />
          <div className="flex min-h-11 items-center justify-between pl-4 pr-2">
            <Dialog.Title className="text-headline text-ink">{title ?? ''}</Dialog.Title>
            <Dialog.Close className="flex h-11 w-11 items-center justify-center rounded-md text-ink-secondary transition-colors duration-fast active:text-ink">
              <X size={20} strokeWidth={1.8} aria-hidden />
              <span className="sr-only">关闭</span>
            </Dialog.Close>
          </div>
          {description ? <Dialog.Description className="sr-only">{description}</Dialog.Description> : null}
          <div className="sheet-body-max overflow-y-auto pb-4">{children}</div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
