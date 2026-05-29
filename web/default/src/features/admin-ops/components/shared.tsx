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
import type { ReactNode } from 'react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

export function AdminOpsCardSkeleton(props: { className?: string }) {
  return <Skeleton className={props.className ?? 'h-48 w-full'} />
}

export function AdminOpsMetric(props: {
  label: string
  value: ReactNode
  description?: ReactNode
}) {
  return (
    <div className='rounded-lg border p-3'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div className='mt-1 text-xl font-semibold'>{props.value}</div>
      {props.description != null && (
        <div className='text-muted-foreground mt-1 text-xs'>
          {props.description}
        </div>
      )}
    </div>
  )
}

export function AdminOpsPanel(props: {
  title: string
  children: ReactNode
  className?: string
}) {
  return (
    <Card className={props.className}>
      <CardHeader>
        <CardTitle>{props.title}</CardTitle>
      </CardHeader>
      <CardContent>{props.children}</CardContent>
    </Card>
  )
}

export function AdminOpsEmpty(props: { children: ReactNode }) {
  return (
    <div className='text-muted-foreground rounded-lg border border-dashed p-6 text-center text-sm'>
      {props.children}
    </div>
  )
}
