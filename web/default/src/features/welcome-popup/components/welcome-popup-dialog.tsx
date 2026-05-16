/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { SafeMarkdown } from '@/components/ui/safe-markdown'
import { ScrollArea } from '@/components/ui/scroll-area'

type WelcomePopupDialogProps = {
  open: boolean
  content: string
  onClose: () => void
}

export function WelcomePopupDialog(props: WelcomePopupDialogProps) {
  const { t } = useTranslation()

  const handleClose = props.onClose

  function handleOpenChange(open: boolean): void {
    if (!open) handleClose()
  }

  return (
    <Dialog open={props.open} onOpenChange={handleOpenChange}>
      <DialogContent className='max-h-[90vh] sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('Welcome announcement')}</DialogTitle>
          <DialogDescription>
            {t(
              'This popup appears after users enter the authenticated system area.'
            )}
          </DialogDescription>
        </DialogHeader>
        <ScrollArea className='max-h-[60vh] pr-4'>
          <SafeMarkdown>{props.content}</SafeMarkdown>
        </ScrollArea>
        <DialogFooter>
          <Button onClick={handleClose} aria-label={t('Close welcome popup')}>
            {t('I understand')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
