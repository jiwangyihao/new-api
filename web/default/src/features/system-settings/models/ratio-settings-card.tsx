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
import { useCallback, useEffect, useRef, useState } from 'react'
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { resetModelRatios } from '../api'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { ModelRatioForm } from './model-ratio-form'
import { ToolPriceSettings } from './tool-price-settings'
import { UpstreamRatioSync } from './upstream-ratio-sync'
import {
  formatJsonForTextarea,
  normalizeJsonString,
  validateJsonString,
} from './utils'

const modelSchema = z.object({
  ModelPrice: z.string().superRefine((value, ctx) => {
    const result = validateJsonString(value)
    if (!result.valid) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: result.message || 'Invalid JSON',
      })
    }
  }),
  ModelRatio: z.string().superRefine((value, ctx) => {
    const result = validateJsonString(value)
    if (!result.valid) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: result.message || 'Invalid JSON',
      })
    }
  }),
  CacheRatio: z.string().superRefine((value, ctx) => {
    const result = validateJsonString(value)
    if (!result.valid) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: result.message || 'Invalid JSON',
      })
    }
  }),
  CreateCacheRatio: z.string().superRefine((value, ctx) => {
    const result = validateJsonString(value)
    if (!result.valid) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: result.message || 'Invalid JSON',
      })
    }
  }),
  CompletionRatio: z.string().superRefine((value, ctx) => {
    const result = validateJsonString(value)
    if (!result.valid) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: result.message || 'Invalid JSON',
      })
    }
  }),
  ImageRatio: z.string().superRefine((value, ctx) => {
    const result = validateJsonString(value)
    if (!result.valid) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: result.message || 'Invalid JSON',
      })
    }
  }),
  AudioRatio: z.string().superRefine((value, ctx) => {
    const result = validateJsonString(value)
    if (!result.valid) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: result.message || 'Invalid JSON',
      })
    }
  }),
  AudioCompletionRatio: z.string().superRefine((value, ctx) => {
    const result = validateJsonString(value)
    if (!result.valid) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: result.message || 'Invalid JSON',
      })
    }
  }),
  ExposeRatioEnabled: z.boolean(),
  BillingMode: z.string().superRefine((value, ctx) => {
    const result = validateJsonString(value)
    if (!result.valid) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: result.message || 'Invalid JSON',
      })
    }
  }),
  BillingExpr: z.string().superRefine((value, ctx) => {
    const result = validateJsonString(value)
    if (!result.valid) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: result.message || 'Invalid JSON',
      })
    }
  }),
})

type ModelFormValues = z.infer<typeof modelSchema>

type RatioTabId = 'models' | 'tool-prices' | 'upstream-sync'

type RatioSettingsCardProps = {
  modelDefaults: ModelFormValues
  toolPricesDefault: string
  titleKey?: string
  descriptionKey?: string
  visibleTabs?: RatioTabId[]
}

export function RatioSettingsCard({
  modelDefaults,
  toolPricesDefault,
  titleKey = 'Pricing Ratios',
  descriptionKey = 'Configure model, caching, and tool pricing used for billing',
  visibleTabs = ['models', 'tool-prices', 'upstream-sync'],
}: RatioSettingsCardProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const queryClient = useQueryClient()
  const [confirmOpen, setConfirmOpen] = useState(false)

  const resetMutation = useMutation({
    mutationFn: resetModelRatios,
    onSuccess: (data) => {
      if (data.success) {
        toast.success(t('Model prices reset successfully'))
        queryClient.invalidateQueries({ queryKey: ['system-options'] })
        setConfirmOpen(false)
      } else {
        toast.error(data.message || t('Failed to reset model ratios'))
      }
    },
    onError: (error: Error) => {
      toast.error(error.message || t('Failed to reset model ratios'))
    },
  })

  const modelNormalizedDefaults = useRef({
    ModelPrice: normalizeJsonString(modelDefaults.ModelPrice),
    ModelRatio: normalizeJsonString(modelDefaults.ModelRatio),
    CacheRatio: normalizeJsonString(modelDefaults.CacheRatio),
    CreateCacheRatio: normalizeJsonString(modelDefaults.CreateCacheRatio),
    CompletionRatio: normalizeJsonString(modelDefaults.CompletionRatio),
    ImageRatio: normalizeJsonString(modelDefaults.ImageRatio),
    AudioRatio: normalizeJsonString(modelDefaults.AudioRatio),
    AudioCompletionRatio: normalizeJsonString(
      modelDefaults.AudioCompletionRatio
    ),
    ExposeRatioEnabled: modelDefaults.ExposeRatioEnabled,
    BillingMode: normalizeJsonString(modelDefaults.BillingMode),
    BillingExpr: normalizeJsonString(modelDefaults.BillingExpr),
  })


  const modelForm = useForm<ModelFormValues>({
    resolver: zodResolver(modelSchema),
    mode: 'onChange',
    defaultValues: {
      ...modelDefaults,
      ModelPrice: formatJsonForTextarea(modelDefaults.ModelPrice),
      ModelRatio: formatJsonForTextarea(modelDefaults.ModelRatio),
      CacheRatio: formatJsonForTextarea(modelDefaults.CacheRatio),
      CreateCacheRatio: formatJsonForTextarea(modelDefaults.CreateCacheRatio),
      CompletionRatio: formatJsonForTextarea(modelDefaults.CompletionRatio),
      ImageRatio: formatJsonForTextarea(modelDefaults.ImageRatio),
      AudioRatio: formatJsonForTextarea(modelDefaults.AudioRatio),
      AudioCompletionRatio: formatJsonForTextarea(
        modelDefaults.AudioCompletionRatio
      ),
      BillingMode: formatJsonForTextarea(modelDefaults.BillingMode),
      BillingExpr: formatJsonForTextarea(modelDefaults.BillingExpr),
    },
  })


  useEffect(() => {
    modelNormalizedDefaults.current = {
      ModelPrice: normalizeJsonString(modelDefaults.ModelPrice),
      ModelRatio: normalizeJsonString(modelDefaults.ModelRatio),
      CacheRatio: normalizeJsonString(modelDefaults.CacheRatio),
      CreateCacheRatio: normalizeJsonString(modelDefaults.CreateCacheRatio),
      CompletionRatio: normalizeJsonString(modelDefaults.CompletionRatio),
      ImageRatio: normalizeJsonString(modelDefaults.ImageRatio),
      AudioRatio: normalizeJsonString(modelDefaults.AudioRatio),
      AudioCompletionRatio: normalizeJsonString(
        modelDefaults.AudioCompletionRatio
      ),
      ExposeRatioEnabled: modelDefaults.ExposeRatioEnabled,
      BillingMode: normalizeJsonString(modelDefaults.BillingMode),
      BillingExpr: normalizeJsonString(modelDefaults.BillingExpr),
    }

    modelForm.reset({
      ...modelDefaults,
      ModelPrice: formatJsonForTextarea(modelDefaults.ModelPrice),
      ModelRatio: formatJsonForTextarea(modelDefaults.ModelRatio),
      CacheRatio: formatJsonForTextarea(modelDefaults.CacheRatio),
      CreateCacheRatio: formatJsonForTextarea(modelDefaults.CreateCacheRatio),
      CompletionRatio: formatJsonForTextarea(modelDefaults.CompletionRatio),
      ImageRatio: formatJsonForTextarea(modelDefaults.ImageRatio),
      AudioRatio: formatJsonForTextarea(modelDefaults.AudioRatio),
      AudioCompletionRatio: formatJsonForTextarea(
        modelDefaults.AudioCompletionRatio
      ),
      BillingMode: formatJsonForTextarea(modelDefaults.BillingMode),
      BillingExpr: formatJsonForTextarea(modelDefaults.BillingExpr),
    })
  }, [modelDefaults, modelForm])


  const saveModelRatios = useCallback(
    async (values: ModelFormValues) => {
      const normalized = {
        ModelPrice: normalizeJsonString(values.ModelPrice),
        ModelRatio: normalizeJsonString(values.ModelRatio),
        CacheRatio: normalizeJsonString(values.CacheRatio),
        CreateCacheRatio: normalizeJsonString(values.CreateCacheRatio),
        CompletionRatio: normalizeJsonString(values.CompletionRatio),
        ImageRatio: normalizeJsonString(values.ImageRatio),
        AudioRatio: normalizeJsonString(values.AudioRatio),
        AudioCompletionRatio: normalizeJsonString(values.AudioCompletionRatio),
        ExposeRatioEnabled: values.ExposeRatioEnabled,
        BillingMode: normalizeJsonString(values.BillingMode),
        BillingExpr: normalizeJsonString(values.BillingExpr),
      }

      const apiKeyMap: Record<string, string> = {
        BillingMode: 'billing_setting.billing_mode',
        BillingExpr: 'billing_setting.billing_expr',
      }

      const updates = (
        Object.keys(normalized) as Array<keyof ModelFormValues>
      ).filter(
        (key) => normalized[key] !== modelNormalizedDefaults.current[key]
      )

      for (const key of updates) {
        const apiKey = apiKeyMap[key as string] || (key as string)
        await updateOption.mutateAsync({ key: apiKey, value: normalized[key] })
      }
    },
    [updateOption]
  )


  const handleResetRatios = useCallback(() => {
    setConfirmOpen(true)
  }, [])

  const { mutate: resetMutate } = resetMutation
  const handleConfirmReset = useCallback(() => {
    resetMutate()
  }, [resetMutate])

  const tabLabels: Record<RatioTabId, string> = {
    models: 'Model prices',
    'tool-prices': 'Tool prices',
    'upstream-sync': 'Upstream price sync',
  }
  const tabsGridClass =
    {
      1: 'grid-cols-1',
      2: 'grid-cols-2',
      3: 'grid-cols-3',
      4: 'grid-cols-4',
    }[visibleTabs.length] ?? 'grid-cols-4'
  const defaultTab = visibleTabs[0] ?? 'models'

  const renderTabContent = (tab: RatioTabId) => {
    if (tab === 'models') {
      return (
        <ModelRatioForm
          form={modelForm}
          onSave={saveModelRatios}
          onReset={handleResetRatios}
          isSaving={updateOption.isPending}
          isResetting={resetMutation.isPending}
        />
      )
    }
    if (tab === 'tool-prices') {
      return <ToolPriceSettings defaultValue={toolPricesDefault} />
    }
    return (
      <UpstreamRatioSync
        modelRatios={{
          ModelPrice: modelDefaults.ModelPrice,
          ModelRatio: modelDefaults.ModelRatio,
          CompletionRatio: modelDefaults.CompletionRatio,
          CacheRatio: modelDefaults.CacheRatio,
          CreateCacheRatio: modelDefaults.CreateCacheRatio,
          ImageRatio: modelDefaults.ImageRatio,
          AudioRatio: modelDefaults.AudioRatio,
          AudioCompletionRatio: modelDefaults.AudioCompletionRatio,
          'billing_setting.billing_mode': modelDefaults.BillingMode,
          'billing_setting.billing_expr': modelDefaults.BillingExpr,
        }}
      />
    )
  }

  return (
    <SettingsSection title={t(titleKey)} description={t(descriptionKey)}>
      {visibleTabs.length === 1 ? (
        renderTabContent(defaultTab)
      ) : (
        <Tabs defaultValue={defaultTab} className='space-y-6'>
          <TabsList className={`grid w-full ${tabsGridClass}`}>
            {visibleTabs.map((tab) => (
              <TabsTrigger key={tab} value={tab}>
                {t(tabLabels[tab])}
              </TabsTrigger>
            ))}
          </TabsList>

          {visibleTabs.map((tab) => (
            <TabsContent key={tab} value={tab}>
              {renderTabContent(tab)}
            </TabsContent>
          ))}
        </Tabs>
      )}

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('Reset all model prices?')}
        desc={t(
          'This will clear custom pricing ratios and revert to upstream defaults.'
        )}
        destructive
        isLoading={resetMutation.isPending}
        handleConfirm={handleConfirmReset}
        confirmText={t('Reset')}
      />
    </SettingsSection>
  )
}
