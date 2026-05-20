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
import { useEffect, useState } from 'react'
import { Loader2, Trophy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { updateUserProfile } from '../../api'
import type { UserProfile } from '../../types'

type RankingsDisplayTabProps = {
  profile: UserProfile | null
  onUpdate: () => void
}

const MAX_RANKINGS_DISPLAY_NAME_LENGTH = 20

export function RankingsDisplayTab(props: RankingsDisplayTabProps) {
  const { t } = useTranslation()
  const [displayName, setDisplayName] = useState('')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    setDisplayName(props.profile?.rankings_display_name ?? '')
  }, [props.profile])

  const handleSave = async () => {
    const normalized = displayName.trim()
    if ([...normalized].length > MAX_RANKINGS_DISPLAY_NAME_LENGTH) {
      toast.error(t('Ranking display name cannot exceed 20 characters'))
      return
    }

    try {
      setLoading(true)
      const response = await updateUserProfile({
        rankings_display_name: normalized,
      })
      if (response.success) {
        toast.success(t('Ranking display name updated successfully'))
        props.onUpdate()
      } else {
        toast.error(
          response.message || t('Failed to update ranking display name')
        )
      }
    } catch (_error) {
      toast.error(t('Failed to update ranking display name'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className='space-y-4 sm:space-y-5'>
      <div className='bg-muted/40 rounded-lg border p-4'>
        <div className='flex items-start gap-3'>
          <Trophy className='mt-0.5 size-4 shrink-0 text-amber-500' />
          <div className='space-y-1'>
            <h3 className='text-foreground text-sm font-semibold'>
              {t('Ranking display name')}
            </h3>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Leave empty to stay anonymous on the free-plan token leaderboard.'
              )}
            </p>
          </div>
        </div>
      </div>

      <div className='space-y-1.5'>
        <Label htmlFor='rankingsDisplayName'>{t('Display on rankings')}</Label>
        <Input
          id='rankingsDisplayName'
          className='h-9'
          maxLength={MAX_RANKINGS_DISPLAY_NAME_LENGTH}
          value={displayName}
          onChange={(event) => setDisplayName(event.target.value)}
          placeholder={t('Leave empty to stay anonymous')}
        />
        <p className='text-muted-foreground text-xs'>
          {t(
            'Only this name can appear publicly; your account identifier is never shown by default.'
          )}
        </p>
      </div>

      <div className='flex justify-end'>
        <Button type='button' onClick={handleSave} disabled={loading}>
          {loading && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
          {t('Save ranking name')}
        </Button>
      </div>
    </div>
  )
}
