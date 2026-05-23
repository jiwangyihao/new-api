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
import React, { useState, useMemo, useCallback } from 'react'
import { ChevronsUpDown, Check, CpuIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useIsMobile } from '@/hooks/use-mobile'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from '@/components/ui/drawer'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'

interface ModelOption {
  label: string
  value: string
  category?: string
  description?: string
}

interface ModelSelectorProps {
  selectedModel: string
  models: ModelOption[]
  onModelChange: (value: string) => void
  className?: string
  disabled?: boolean
}

const ModelTriggerButton = React.forwardRef<
  React.ComponentRef<typeof Button>,
  React.ComponentPropsWithoutRef<typeof Button> & {
    currentLabel: string
    triggerClassName?: string
    isDisabled?: boolean
  }
>(({ currentLabel, triggerClassName, isDisabled, ...props }, ref) => (
  <Button
    ref={ref}
    variant='outline'
    role='combobox'
    size='sm'
    disabled={isDisabled}
    className={cn(
      'flex h-8 items-center gap-2 border px-3 font-medium',
      'justify-center p-0 sm:w-auto sm:justify-start sm:px-3',
      'w-8',
      'bg-background text-foreground',
      'hover:bg-accent transition-colors',
      'focus:!ring-0 focus:!outline-none',
      'shadow-none',
      triggerClassName
    )}
    {...props}
  >
    <CpuIcon className='text-muted-foreground block size-4 sm:hidden' />
    <span className='text-muted-foreground sm:text-foreground hidden truncate text-xs sm:block'>
      {currentLabel}
    </span>
    <ChevronsUpDown className='text-muted-foreground hidden h-4 w-4 opacity-50 sm:block' />
  </Button>
))

ModelTriggerButton.displayName = 'ModelTriggerButton'

/**
 * Model Selector Component
 * Styled following Scira's form-component design patterns
 */
export const ModelSelector: React.FC<ModelSelectorProps> = React.memo(
  ({ selectedModel, models, onModelChange, className, disabled = false }) => {
    const { t } = useTranslation()
    const [open, setOpen] = useState(false)
    const [searchQuery, setSearchQuery] = useState('')
    const isMobile = useIsMobile()

    const currentModel = useMemo(
      () => models.find((m) => m.value === selectedModel),
      [models, selectedModel]
    )

    // Group models by category
    const groupedModels = useMemo(
      () =>
        models.reduce(
          (acc, model) => {
            const category = model.category || t('Other')
            if (!acc[category]) {
              acc[category] = []
            }
            acc[category].push(model)
            return acc
          },
          {} as Record<string, ModelOption[]>
        ),
      [models, t]
    )

    // Filter models by search query
    const filteredModels = useMemo(() => {
      if (!searchQuery.trim()) return groupedModels

      const query = searchQuery.toLowerCase()
      const filtered: Record<string, ModelOption[]> = {}

      Object.entries(groupedModels).forEach(([category, categoryModels]) => {
        const matches = categoryModels.filter(
          (m) =>
            m.label.toLowerCase().includes(query) ||
            m.value.toLowerCase().includes(query) ||
            m.description?.toLowerCase().includes(query)
        )
        if (matches.length > 0) {
          filtered[category] = matches
        }
      })

      return filtered
    }, [groupedModels, searchQuery])

    const handleModelChange = useCallback(
      (value: string) => {
        onModelChange(value)
        setOpen(false)
        setSearchQuery('')
      },
      [onModelChange]
    )

    // Shared command content
    const renderModelCommandContent = () => (
      <Command
        className={cn(
          isMobile
            ? 'h-full flex-1 rounded-lg border-0 bg-transparent'
            : 'rounded-lg'
        )}
        filter={() => 1}
        shouldFilter={false}
      >
        {!isMobile && (
          <CommandInput
            placeholder={t('Search models...')}
            className='h-9'
            value={searchQuery}
            onValueChange={setSearchQuery}
          />
        )}
        <CommandEmpty>{t('No model found.')}</CommandEmpty>
        <CommandList
          className={isMobile ? '!max-h-full flex-1 p-2' : 'max-h-[300px]'}
        >
          {Object.keys(filteredModels).length === 0 ? (
            <div className='text-muted-foreground px-3 py-6 text-xs'>
              {t('No model found.')}
            </div>
          ) : (
            Object.entries(filteredModels).map(
              ([category, categoryModels], categoryIndex) => (
                <CommandGroup key={category}>
                  {categoryIndex > 0 && (
                    <div className='border-border my-1 border-t' />
                  )}
                  <div
                    className={cn(
                      'text-muted-foreground px-2 py-1 font-medium',
                      isMobile ? 'text-xs' : 'text-[10px]'
                    )}
                  >
                    {t('{{category}} Models', { category })}
                  </div>
                  {categoryModels.map((model) => (
                    <CommandItem
                      key={model.value}
                      value={model.value}
                      onSelect={handleModelChange}
                      className={cn(
                        'mb-0.5 flex items-center justify-between rounded-lg px-2 py-1.5 text-xs',
                        'transition-all duration-200',
                        'hover:bg-accent',
                        'data-[selected=true]:bg-accent'
                      )}
                    >
                      <div className='flex min-w-0 flex-1 items-center gap-1'>
                        <div
                          className={cn(
                            'truncate font-medium',
                            isMobile ? 'text-sm' : 'text-[11px]'
                          )}
                        >
                          <span className='inline'>{model.label}</span>
                        </div>
                        <Check
                          className={cn(
                            'h-4 w-4 flex-shrink-0',
                            selectedModel === model.value
                              ? 'opacity-100'
                              : 'opacity-0'
                          )}
                        />
                      </div>
                    </CommandItem>
                  ))}
                </CommandGroup>
              )
            )
          )}
        </CommandList>
      </Command>
    )

    return (
      <>
        {isMobile ? (
          <Drawer open={open} onOpenChange={setOpen}>
            <DrawerTrigger asChild>
              <ModelTriggerButton
                currentLabel={currentModel?.label || t('Model')}
                triggerClassName={className}
                isDisabled={disabled}
                aria-expanded={open}
              />
            </DrawerTrigger>
            <DrawerContent className='flex max-h-[80vh] min-h-[60vh] flex-col'>
              <DrawerHeader className='flex-shrink-0 pb-4'>
                <DrawerTitle className='flex items-center gap-2 text-left text-lg font-medium'>
                  {t('Select Model')}
                </DrawerTitle>
              </DrawerHeader>
              <div className='flex min-h-0 flex-1 flex-col'>
                {renderModelCommandContent()}
              </div>
            </DrawerContent>
          </Drawer>
        ) : (
          <Popover open={open} onOpenChange={setOpen}>
            <PopoverTrigger
              render={
                <ModelTriggerButton
                  currentLabel={currentModel?.label || t('Model')}
                  triggerClassName={className}
                  isDisabled={disabled}
                  aria-expanded={open}
                />
              }
            />
            <PopoverContent
              className='bg-popover z-40 w-[90vw] max-w-[20em] rounded-lg border p-0 !shadow-none sm:w-[20em]'
              align='start'
              side='bottom'
              sideOffset={4}
              collisionPadding={8}
            >
              {renderModelCommandContent()}
            </PopoverContent>
          </Popover>
        )}
      </>
    )
  }
)

ModelSelector.displayName = 'ModelSelector'
