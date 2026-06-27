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
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { deleteChannelGroup, getChannelGroups } from '../api'
import type { ChannelGroup } from '../types'
import {
  CHANNEL_GROUPS_QUERY_KEY,
  ChannelGroupMutateDrawer,
} from './channel-group-mutate-drawer'

function billingModeLabelKey(mode: string): string {
  if (mode === 'usage_tokens') return 'Usage tokens'
  if (mode === 'fixed_request') return 'Fixed per request'
  return 'Inherit from channel'
}

export function ChannelGroupsTable() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [currentRow, setCurrentRow] = useState<ChannelGroup | undefined>(
    undefined
  )
  const [deleteTarget, setDeleteTarget] = useState<ChannelGroup | null>(null)

  const { data, isFetching } = useQuery({
    queryKey: CHANNEL_GROUPS_QUERY_KEY,
    queryFn: getChannelGroups,
  })

  const groups = data?.data ?? []

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteChannelGroup(id),
    onSuccess: (result) => {
      if (result.success) {
        toast.success(t('Channel group deleted successfully'))
        queryClient.invalidateQueries({ queryKey: CHANNEL_GROUPS_QUERY_KEY })
      } else {
        toast.error(result.message || t('Operation failed'))
      }
    },
    onError: () => toast.error(t('Operation failed')),
    onSettled: () => setDeleteTarget(null),
  })

  const openCreate = () => {
    setCurrentRow(undefined)
    setDrawerOpen(true)
  }

  const openEdit = (group: ChannelGroup) => {
    setCurrentRow(group)
    setDrawerOpen(true)
  }

  return (
    <div className='space-y-4'>
      <div className='flex justify-end'>
        <Button onClick={openCreate}>
          <Plus className='size-4' />
          {t('Create Channel Group')}
        </Button>
      </div>

      <div className='rounded-lg border'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Name')}</TableHead>
              <TableHead>{t('Description')}</TableHead>
              <TableHead>{t('Billing Mode')}</TableHead>
              <TableHead>{t('Member Channels')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {groups.length === 0 && !isFetching ? (
              <TableRow>
                <TableCell
                  colSpan={6}
                  className='text-muted-foreground text-center'
                >
                  {t('No channel groups yet.')}
                </TableCell>
              </TableRow>
            ) : (
              groups.map((group) => (
                <TableRow key={group.id}>
                  <TableCell className='font-medium'>
                    {group.is_default ? (
                      <span className='flex items-center gap-2'>
                        {t('Default Group')}
                        <Badge variant='secondary'>{t('Default')}</Badge>
                      </span>
                    ) : (
                      group.name
                    )}
                  </TableCell>
                  <TableCell className='text-muted-foreground max-w-xs truncate'>
                    {group.description}
                  </TableCell>
                  <TableCell>
                    {t(billingModeLabelKey(group.credit_billing_mode))}
                  </TableCell>
                  <TableCell>
                    {group.is_default && group.channel_ids.length === 0
                      ? t('All channels')
                      : group.channel_ids.length}
                  </TableCell>
                  <TableCell>
                    <Badge variant={group.enabled ? 'default' : 'secondary'}>
                      {group.enabled ? t('Enabled') : t('Disabled')}
                    </Badge>
                  </TableCell>
                  <TableCell className='text-right'>
                    <div className='flex justify-end gap-1'>
                      <Button
                        variant='ghost'
                        size='icon'
                        onClick={() => openEdit(group)}
                        aria-label={t('Edit')}
                      >
                        <Pencil className='size-4' />
                      </Button>
                      {!group.is_default && (
                        <Button
                          variant='ghost'
                          size='icon'
                          onClick={() => setDeleteTarget(group)}
                          aria-label={t('Delete')}
                        >
                          <Trash2 className='size-4' />
                        </Button>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      <ChannelGroupMutateDrawer
        open={drawerOpen}
        onOpenChange={setDrawerOpen}
        currentRow={currentRow}
      />

      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(v) => !v && setDeleteTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Delete Channel Group?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'This will permanently delete the channel group. This action cannot be undone.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (deleteTarget) deleteMutation.mutate(deleteTarget.id)
              }}
            >
              {t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
