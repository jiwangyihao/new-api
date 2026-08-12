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
import { useEffect, useState, type JSX } from 'react'
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
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  GPT_ABUSE_DEFAULT_REASON,
  GPT_ABUSE_REASON_MAX_LENGTH,
} from '../constants'
import type { GPTAbuseReasonPayload, GPTAbuseUserListItem } from '../types'

type GPTAbuseClearSuspensionDialogProps = {
  open: boolean
  user: GPTAbuseUserListItem | null
  loading: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: (payload: GPTAbuseReasonPayload) => void
}

export function GPTAbuseClearSuspensionDialog(
  props: GPTAbuseClearSuspensionDialogProps
): JSX.Element {
  const { t } = useTranslation()
  const [reason, setReason] = useState(GPT_ABUSE_DEFAULT_REASON)

  useEffect(() => {
    if (props.open) setReason(GPT_ABUSE_DEFAULT_REASON)
  }, [props.open])

  const safeReason = reason.slice(0, GPT_ABUSE_REASON_MAX_LENGTH)

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('gptAbuse.dialog.clearTitle')}</DialogTitle>
          <DialogDescription>
            {props.user
              ? t('gptAbuse.dialog.clearDescriptionForUser', {
                  user: props.user.username || `#${props.user.user_id}`,
                })
              : t('gptAbuse.dialog.clearSuspensionText')}
          </DialogDescription>
        </DialogHeader>
        <div className='space-y-4'>
          <p className='text-muted-foreground text-sm'>
            {t('gptAbuse.dialog.clearSuspensionText')}
          </p>
          <div className='space-y-2'>
            <Label htmlFor='gpt-abuse-clear-reason'>{t('gptAbuse.dialog.reason')}</Label>
            <Textarea
              id='gpt-abuse-clear-reason'
              maxLength={GPT_ABUSE_REASON_MAX_LENGTH}
              value={safeReason}
              onChange={(event) => setReason(event.target.value)}
              disabled={props.loading}
            />
            <div className='text-muted-foreground text-xs'>
              {t('gptAbuse.dialog.reasonLimit', {
                count: safeReason.length,
                max: GPT_ABUSE_REASON_MAX_LENGTH,
              })}
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            disabled={props.loading}
            onClick={() => props.onOpenChange(false)}
          >
            {t('gptAbuse.actions.cancel')}
          </Button>
          <Button
            type='button'
            disabled={props.loading}
            onClick={() =>
              props.onConfirm({ reason: safeReason.trim() || GPT_ABUSE_DEFAULT_REASON })
            }
          >
            {props.loading ? t('gptAbuse.actions.submitting') : t('gptAbuse.actions.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
