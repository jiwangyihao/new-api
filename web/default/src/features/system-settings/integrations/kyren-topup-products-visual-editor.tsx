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
import { useMemo, useState } from 'react'
import { Pencil, Plus, RefreshCw, Search, Trash2, UploadCloud } from 'lucide-react'
import { toast } from 'sonner'
import { useTranslation } from 'react-i18next'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatQuotaShort } from '@/features/wallet/lib/format'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import type { KyrenTopUpProduct } from '../types'
import { isArray } from '../utils/json-validators'
import { safeJsonParseWithValidation } from '../utils/json-parser'
import {
  KyrenTopUpProductDialog,
  type KyrenTopUpProductData,
} from './kyren-topup-product-dialog'

export const KYREN_TOPUP_PRODUCTS_CONFLICT_MESSAGE =
  'Kyren settings were updated elsewhere. Please reload and try again.'

export type KyrenTopUpProductsListResponse = {
  products: KyrenTopUpProduct[]
  version: string
}

export type KyrenTopUpProductStatus = {
  product_id?: string
  status?: string
  price?: string
  currency?: string
  price_matches?: boolean
  currency_matches?: boolean
  version?: string
}

export type KyrenTopUpProductSyncMode =
  | 'create_or_update'
  | 'create_new'
  | 'update_existing'

export type KyrenTopUpProductSyncResponse = KyrenTopUpProductsListResponse & {
  product_id: string
  status: string
  price: string
  currency: string
  synced: boolean
  local_error?: string
}

export type KyrenTopUpProductsState = {
  products: KyrenTopUpProduct[]
  version: string
  statuses: Record<string, KyrenTopUpProductStatus>
}

type KyrenApiResponse<T> = {
  success: boolean
  message?: string
  data: T
}

type KyrenTopUpProductsVisualEditorProps = {
  products: KyrenTopUpProduct[]
  version: string
  statuses?: Record<string, KyrenTopUpProductStatus>
  onChange: (products: KyrenTopUpProduct[]) => void
  onVersionChange?: (version: string) => void
  onStatusesChange?: (statuses: Record<string, KyrenTopUpProductStatus>) => void
  onRefetch?: () => Promise<KyrenTopUpProductsListResponse>
}

type SaveKyrenTopUpProductsStateParams = {
  products: KyrenTopUpProduct[]
  version: string
  request?: (
    products: KyrenTopUpProduct[],
    version: string
  ) => Promise<KyrenTopUpProductsListResponse>
  refetch: () => Promise<KyrenTopUpProductsListResponse>
  notifyConflict: (message: string) => void
}

const amountPattern = /^\d+(?:\.\d{1,2})?$/

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isKyrenTopUpProduct(value: unknown): value is KyrenTopUpProduct {
  if (!isRecord(value)) return false
  return (
    typeof value.id === 'string' &&
    typeof value.name === 'string' &&
    typeof value.amount === 'string' &&
    typeof value.currency === 'string' &&
    typeof value.quota === 'number' &&
    typeof value.enabled === 'boolean' &&
    (value.description === undefined || typeof value.description === 'string') &&
    (value.product_id === undefined || typeof value.product_id === 'string')
  )
}

function parseKyrenTopUpProductsFromJson(
  value: string,
  arrayErrorMessage: string
): KyrenTopUpProduct[] {
  const parsed = safeJsonParseWithValidation<unknown[]>(value, {
    fallback: [],
    validator: isArray,
    validatorMessage: arrayErrorMessage,
    context: 'kyren top-up products',
  })
  return parsed.filter(isKyrenTopUpProduct)
}

export function parseKyrenTopUpProducts(value: string): KyrenTopUpProduct[] {
  return parseKyrenTopUpProductsFromJson(
    value,
    'Kyren top-up products must be a JSON array'
  )
}

export function normalizeKyrenAmountString(value: string): string | null {
  const trimmed = value.trim()
  if (!amountPattern.test(trimmed)) return null
  const numeric = Number.parseFloat(trimmed)
  if (!Number.isFinite(numeric) || numeric < 0.01) return null
  return numeric.toFixed(2)
}

export function validateKyrenTopUpProducts(
  products: KyrenTopUpProduct[]
): KyrenTopUpProduct[] {
  const seenIds = new Set<string>()
  return products.map((product) => {
    const id = product.id.trim()
    if (!id) {
      throw new Error('Kyren top-up product id is required')
    }
    if (seenIds.has(id)) {
      throw new Error('Duplicate Kyren top-up product ID')
    }
    seenIds.add(id)

    const name = product.name.trim()
    if (!name) {
      throw new Error('Product name is required')
    }

    const currency = product.currency.trim().toUpperCase()
    if (currency !== 'CNY') {
      throw new Error('Kyren top-up products only support CNY')
    }

    const amount = normalizeKyrenAmountString(product.amount)
    if (amount === null) {
      throw new Error('Amount must be at least 0.01 CNY')
    }

    if (!Number.isFinite(product.quota) || product.quota <= 0) {
      throw new Error('Quota must be at least 1')
    }

    return {
      id,
      name,
      description: product.description?.trim() ?? '',
      product_id: product.product_id?.trim() ?? '',
      amount,
      currency: 'CNY',
      quota: Math.trunc(product.quota),
      enabled: product.enabled,
    }
  })
}

function unwrapKyrenData<T>(response: KyrenApiResponse<T>): T {
  if (!response.success) {
    throw new Error(response.message || 'Request failed')
  }
  return response.data
}

export async function fetchKyrenTopUpProducts(): Promise<KyrenTopUpProductsListResponse> {
  const res = await api.get<KyrenApiResponse<KyrenTopUpProductsListResponse>>(
    '/api/payment/kyren/topup-products',
    { disableDuplicate: true } as Record<string, unknown>
  )
  return unwrapKyrenData(res.data)
}

export async function putKyrenTopUpProducts(
  products: KyrenTopUpProduct[],
  version: string
): Promise<KyrenTopUpProductsListResponse> {
  const res = await api.put<KyrenApiResponse<KyrenTopUpProductsListResponse>>(
    '/api/payment/kyren/topup-products',
    { products: validateKyrenTopUpProducts(products), version },
    { skipErrorHandler: true } as Record<string, unknown>
  )
  return unwrapKyrenData(res.data)
}

export async function fetchKyrenTopUpProductStatus(
  id: string
): Promise<KyrenTopUpProductStatus> {
  const res = await api.get<KyrenApiResponse<KyrenTopUpProductStatus>>(
    `/api/payment/kyren/topup-products/${encodeURIComponent(id)}/status`,
    { disableDuplicate: true } as Record<string, unknown>
  )
  return unwrapKyrenData(res.data)
}

export async function syncKyrenTopUpProduct(
  id: string,
  mode: KyrenTopUpProductSyncMode = 'create_or_update'
): Promise<KyrenTopUpProductSyncResponse> {
  const res = await api.post<KyrenApiResponse<KyrenTopUpProductSyncResponse>>(
    `/api/payment/kyren/topup-products/${encodeURIComponent(id)}/sync`,
    { mode }
  )
  return unwrapKyrenData(res.data)
}

export async function saveKyrenTopUpProductsState(
  params: SaveKyrenTopUpProductsStateParams
): Promise<{
  conflicted: boolean
  state: KyrenTopUpProductsListResponse
}> {
  const request = params.request ?? putKyrenTopUpProducts
  try {
    const state = await request(params.products, params.version)
    return { conflicted: false, state }
  } catch (error) {
    const response = (error as { response?: { status?: number } }).response
    if (response?.status === 409) {
      params.notifyConflict(KYREN_TOPUP_PRODUCTS_CONFLICT_MESSAGE)
      const state = await params.refetch()
      return { conflicted: true, state }
    }
    throw error
  }
}

export async function syncKyrenTopUpProductState(
  state: KyrenTopUpProductsState,
  productId: string,
  options: {
    mode?: KyrenTopUpProductSyncMode
    request?: (
      productId: string,
      mode: KyrenTopUpProductSyncMode
    ) => Promise<KyrenTopUpProductSyncResponse>
  } = {}
): Promise<KyrenTopUpProductsState> {
  const mode = options.mode ?? 'create_or_update'
  const request = options.request ?? syncKyrenTopUpProduct
  const response = await request(productId, mode)
  return {
    products: response.products,
    version: response.version,
    statuses: {
      ...state.statuses,
      [productId]: {
        product_id: response.product_id,
        status: response.status,
        price: response.price,
        currency: response.currency,
        price_matches: true,
        currency_matches: response.currency === 'CNY',
      },
    },
  }
}

export function getKyrenTopUpProductStatusAlerts(
  status: KyrenTopUpProductStatus | undefined
): string[] {
  if (!status) return []
  const alerts: string[] = []
  if (status.status === 'ARCHIVED') {
    alerts.push('Kyren top-up product is archived')
  }
  if (status.price_matches === false) {
    alerts.push('Kyren top-up product price mismatch')
  }
  if (status.currency_matches === false) {
    alerts.push('Kyren top-up product currency mismatch')
  }
  return alerts
}

function formatKyrenPrice(amount: string): string {
  return `¥${amount}`
}

function getStatusVariant(status: KyrenTopUpProductStatus | undefined) {
  if (!status?.status) return 'neutral'
  if (status.status === 'ACTIVE') return 'success'
  if (status.status === 'ARCHIVED') return 'danger'
  return 'warning'
}

function mergeStatuses(
  current: Record<string, KyrenTopUpProductStatus>,
  productId: string,
  status: KyrenTopUpProductStatus
): Record<string, KyrenTopUpProductStatus> {
  return { ...current, [productId]: status }
}

export function KyrenTopUpProductsVisualEditor(
  props: KyrenTopUpProductsVisualEditorProps
) {
  const { t } = useTranslation()
  const [searchText, setSearchText] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editData, setEditData] = useState<KyrenTopUpProductData | null>(null)
  const [loadingKey, setLoadingKey] = useState<string | null>(null)

  const statuses = props.statuses ?? {}
  const products = useMemo(
    () => validateKyrenTopUpProducts(props.products),
    [props.products]
  )
  const filteredProducts = useMemo(() => {
    if (!searchText) return products
    const lowerSearch = searchText.toLowerCase()
    return products.filter(
      (product) =>
        product.name.toLowerCase().includes(lowerSearch) ||
        product.id.toLowerCase().includes(lowerSearch) ||
        (product.product_id ?? '').toLowerCase().includes(lowerSearch)
    )
  }, [products, searchText])

  const applyServerState = (
    state: KyrenTopUpProductsListResponse,
    nextStatuses = statuses
  ) => {
    props.onChange(validateKyrenTopUpProducts(state.products))
    props.onVersionChange?.(state.version)
    props.onStatusesChange?.(nextStatuses)
  }

  const handleSave = (data: KyrenTopUpProductData) => {
    const normalized = validateKyrenTopUpProducts([data])[0]
    const updatedProducts = editData
      ? products.map((product) =>
          product.id === editData.id ? normalized : product
        )
      : [...products, normalized]
    props.onChange(validateKyrenTopUpProducts(updatedProducts))
  }

  const handleDelete = (product: KyrenTopUpProductData) => {
    props.onChange(products.filter((item) => item.id !== product.id))
  }

  const handleRefreshStatus = async (product: KyrenTopUpProductData) => {
    setLoadingKey(`status:${product.id}`)
    try {
      const status = await fetchKyrenTopUpProductStatus(product.id)
      const nextStatuses = mergeStatuses(statuses, product.id, status)
      props.onStatusesChange?.(nextStatuses)
      if (status.version) {
        props.onVersionChange?.(status.version)
      }
      toast.success(t('Kyren top-up product status refreshed'))
    } catch (error) {
      const message = error instanceof Error ? error.message : t('Request failed')
      toast.error(message)
    } finally {
      setLoadingKey(null)
    }
  }

  const handleSync = async (
    product: KyrenTopUpProductData,
    mode: KyrenTopUpProductSyncMode
  ) => {
    setLoadingKey(`sync:${mode}:${product.id}`)
    try {
      const nextState = await syncKyrenTopUpProductState(
        { products, version: props.version, statuses },
        product.id,
        { mode }
      )
      applyServerState(nextState, nextState.statuses)
      toast.success(t('Kyren top-up product synced'))
    } catch (error) {
      const message = error instanceof Error ? error.message : t('Request failed')
      toast.error(message)
    } finally {
      setLoadingKey(null)
    }
  }

  const handleRefetch = async () => {
    if (!props.onRefetch) return
    setLoadingKey('refetch')
    try {
      const state = await props.onRefetch()
      applyServerState(state)
      toast.success(t('Kyren top-up products refreshed'))
    } catch (error) {
      const message = error instanceof Error ? error.message : t('Request failed')
      toast.error(message)
    } finally {
      setLoadingKey(null)
    }
  }

  const handleEdit = (product: KyrenTopUpProductData) => {
    setEditData(product)
    setDialogOpen(true)
  }

  const handleAdd = () => {
    setEditData(null)
    setDialogOpen(true)
  }

  const renderStatusDetails = (product: KyrenTopUpProductData) => {
    const status = statuses[product.id]
    const alerts = getKyrenTopUpProductStatusAlerts(status)
    if (!status) {
      return <span className='text-muted-foreground text-xs'>{t('Not refreshed')}</span>
    }
    return (
      <div className='space-y-1 text-xs'>
        <StatusBadge
          label={status.status ? t(status.status) : t('Unbound')}
          variant={getStatusVariant(status)}
          copyable={false}
        />
        <div className='text-muted-foreground flex flex-wrap gap-x-2 gap-y-1'>
          <span>
            {t('Price')}: {status.price || '-'}
          </span>
          <span>
            {t('Currency')}: {status.currency || '-'}
          </span>
          <span>
            {t('Price matches')}: {status.price_matches ? t('Yes') : t('No')}
          </span>
          <span>
            {t('Currency matches')}: {status.currency_matches ? t('Yes') : t('No')}
          </span>
        </div>
        {alerts.length > 0 && (
          <div className='space-y-1'>
            {alerts.map((alert) => (
              <div
                key={alert}
                className='rounded-md border border-destructive/40 bg-destructive/10 px-2 py-1 text-destructive'
              >
                {t(alert)}
              </div>
            ))}
          </div>
        )}
      </div>
    )
  }

  const renderActions = (product: KyrenTopUpProductData) => (
    <div className='flex flex-wrap justify-end gap-2'>
      <Button
        type='button'
        variant='ghost'
        size='sm'
        onClick={(event) => {
          event.preventDefault()
          event.stopPropagation()
          handleRefreshStatus(product)
        }}
        disabled={loadingKey === `status:${product.id}`}
      >
        <RefreshCw className='h-4 w-4' />
        <span className='sr-only'>{t('Refresh status')}</span>
      </Button>
      <Button
        type='button'
        variant='ghost'
        size='sm'
        onClick={(event) => {
          event.preventDefault()
          event.stopPropagation()
          handleSync(product, 'create_or_update')
        }}
        disabled={loadingKey === `sync:create_or_update:${product.id}`}
      >
        <UploadCloud className='h-4 w-4' />
        <span className='sr-only'>{t('Sync Kyren product')}</span>
      </Button>
      <Button
        type='button'
        variant='ghost'
        size='sm'
        onClick={(event) => {
          event.preventDefault()
          event.stopPropagation()
          handleEdit(product)
        }}
      >
        <Pencil className='h-4 w-4' />
        <span className='sr-only'>{t('Edit')}</span>
      </Button>
      <Button
        type='button'
        variant='ghost'
        size='sm'
        onClick={(event) => {
          event.preventDefault()
          event.stopPropagation()
          handleDelete(product)
        }}
      >
        <Trash2 className='h-4 w-4' />
        <span className='sr-only'>{t('Delete')}</span>
      </Button>
      <Button
        type='button'
        variant='outline'
        size='sm'
        onClick={(event) => {
          event.preventDefault()
          event.stopPropagation()
          handleSync(product, 'create_new')
        }}
        disabled={loadingKey === `sync:create_new:${product.id}`}
      >
        {t('Create new')}
      </Button>
    </div>
  )

  return (
    <div className='space-y-4' data-testid='kyren-topup-products-editor'>
      <div className='flex flex-col gap-3 sm:flex-row sm:items-center'>
        <div className='relative flex-1'>
          <Search className='text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4' />
          <Input
            placeholder={t('Search Kyren top-up products...')}
            value={searchText}
            onChange={(event) => setSearchText(event.target.value)}
            className='pl-9'
          />
        </div>
        <div className='flex flex-col gap-2 sm:flex-row'>
          {props.onRefetch && (
            <Button
              type='button'
              variant='outline'
              onClick={(event) => {
                event.preventDefault()
                event.stopPropagation()
                handleRefetch()
              }}
              disabled={loadingKey === 'refetch'}
              className='flex-1 sm:flex-none'
            >
              <RefreshCw className='h-4 w-4 sm:mr-2' />
              <span>{t('Refresh')}</span>
            </Button>
          )}
          <Button
            type='button'
            onClick={(event) => {
              event.preventDefault()
              event.stopPropagation()
              handleAdd()
            }}
            className='flex-1 sm:flex-none'
          >
            <Plus className='h-4 w-4 sm:mr-2' />
            <span>{t('Add Kyren top-up product')}</span>
          </Button>
        </div>
      </div>

      {filteredProducts.length === 0 ? (
        <div className='text-muted-foreground rounded-lg border border-dashed p-8 text-center text-sm'>
          {searchText
            ? t('No Kyren top-up products match your search')
            : t(
                'No Kyren top-up products configured. Click "Add Kyren top-up product" to get started.'
              )}
        </div>
      ) : (
        <div className='rounded-md border'>
          <div className='hidden md:block'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Name')}</TableHead>
                  <TableHead>{t('Local product ID')}</TableHead>
                  <TableHead>{t('Kyren product ID')}</TableHead>
                  <TableHead>{t('Amount')}</TableHead>
                  <TableHead>{t('Quota')}</TableHead>
                  <TableHead>{t('Status')}</TableHead>
                  <TableHead className='text-right'>{t('Actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredProducts.map((product) => (
                  <TableRow
                    key={product.id}
                    className={cn(
                      getKyrenTopUpProductStatusAlerts(statuses[product.id])
                        .length > 0 && 'bg-destructive/5'
                    )}
                  >
                    <TableCell className='font-medium'>
                      <div>{product.name}</div>
                      {!product.enabled && (
                        <StatusBadge
                          label={t('Disabled')}
                          variant='neutral'
                          copyable={false}
                        />
                      )}
                    </TableCell>
                    <TableCell>
                      <code className='bg-muted rounded px-1.5 py-0.5 text-xs'>
                        {product.id}
                      </code>
                    </TableCell>
                    <TableCell>
                      <code className='bg-muted rounded px-1.5 py-0.5 text-xs'>
                        {product.product_id || t('Unbound')}
                      </code>
                    </TableCell>
                    <TableCell>
                      <span className='font-mono text-sm'>
                        {formatKyrenPrice(product.amount)} CNY
                      </span>
                    </TableCell>
                    <TableCell>
                      <span className='font-mono text-sm'>
                        {formatQuotaShort(product.quota)}
                      </span>
                    </TableCell>
                    <TableCell>{renderStatusDetails(product)}</TableCell>
                    <TableCell className='text-right'>
                      {renderActions(product)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>

          <div className='divide-y md:hidden'>
            {filteredProducts.map((product) => (
              <div key={product.id} className='space-y-3 p-4'>
                <div className='flex items-start justify-between gap-3'>
                  <div className='min-w-0 flex-1'>
                    <div className='font-medium'>{product.name}</div>
                    <code className='bg-muted rounded px-1.5 py-0.5 text-xs'>
                      {product.id}
                    </code>
                  </div>
                  {!product.enabled && (
                    <StatusBadge
                      label={t('Disabled')}
                      variant='neutral'
                      copyable={false}
                    />
                  )}
                </div>
                <div className='space-y-2 text-sm'>
                  <div className='flex items-center gap-2'>
                    <span className='text-muted-foreground min-w-24'>
                      {t('Kyren product ID')}:
                    </span>
                    <code className='bg-muted rounded px-1.5 py-0.5 text-xs'>
                      {product.product_id || t('Unbound')}
                    </code>
                  </div>
                  <div className='flex items-center gap-2'>
                    <span className='text-muted-foreground min-w-24'>
                      {t('Amount')}:
                    </span>
                    <span className='font-mono'>
                      {formatKyrenPrice(product.amount)} CNY
                    </span>
                  </div>
                  <div className='flex items-center gap-2'>
                    <span className='text-muted-foreground min-w-24'>
                      {t('Quota')}:
                    </span>
                    <span className='font-mono'>
                      {formatQuotaShort(product.quota)}
                    </span>
                  </div>
                </div>
                {renderStatusDetails(product)}
                {renderActions(product)}
              </div>
            ))}
          </div>
        </div>
      )}

      <KyrenTopUpProductDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onSave={handleSave}
        editData={editData}
      />
    </div>
  )
}
