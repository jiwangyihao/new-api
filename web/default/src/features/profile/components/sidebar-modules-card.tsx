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
import { useCallback, useEffect, useState } from 'react'
import { LayoutDashboard } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'

type SidebarModuleConfig = {
  enabled: boolean
  [key: string]: boolean
}

type SidebarModulesConfig = Record<string, SidebarModuleConfig>

type SectionDef = {
  key: string
  title: string
  description: string
  modules: { key: string; title: string; description: string }[]
}

export function buildDefaultSidebarModulesForSectionsForTest(
  sections: SectionDef[],
  unavailableModules: Record<string, unknown> = {},
  savedConfig?: SidebarModulesConfig
): SidebarModulesConfig {
  const defaults = savedConfig ?? buildDefaultSidebarModulesForSections(sections, unavailableModules)
  return clampSidebarModulesForPermissionsForTest(defaults, unavailableModules)
}

export function clampSidebarModulesForPermissionsForTest(
  config: SidebarModulesConfig,
  unavailableModules: Record<string, unknown> = {}
): SidebarModulesConfig {
  return applySidebarModulePermissions(config, unavailableModules)
}

function buildDefaultSidebarModulesForSections(
  sections: SectionDef[],
  unavailableModules: Record<string, unknown> = {}
): SidebarModulesConfig {
  const defaults: SidebarModulesConfig = {}
  for (const sec of sections) {
    defaults[sec.key] = { enabled: true }
    for (const mod of sec.modules) {
      defaults[sec.key][mod.key] = !isSidebarModuleUnavailable(unavailableModules, sec.key, mod.key)
    }
  }
  return applySidebarModulePermissions(defaults, unavailableModules)
}

function isSidebarModuleUnavailable(
  unavailableModules: Record<string, unknown>,
  sectionKey: string,
  moduleKey: string
): boolean {
  const section = unavailableModules[sectionKey]
  if (section === false) return true
  if (section == null || typeof section !== 'object') return false
  const sectionRecord = section as Record<string, unknown>
  return sectionRecord.enabled === false || sectionRecord[moduleKey] === false
}

function applySidebarModulePermissions(
  config: SidebarModulesConfig,
  unavailableModules: Record<string, unknown> = {}
): SidebarModulesConfig {
  const next: SidebarModulesConfig = {}
  for (const [sectionKey, section] of Object.entries(config)) {
    const nextSection: SidebarModuleConfig = { ...section }
    for (const moduleKey of Object.keys(nextSection)) {
      if (moduleKey === 'enabled') continue
      if (isSidebarModuleUnavailable(unavailableModules, sectionKey, moduleKey)) {
        nextSection[moduleKey] = false
      }
    }
    const unavailableSection = unavailableModules[sectionKey]
    if (
      unavailableSection === false ||
      (typeof unavailableSection === 'object' &&
        unavailableSection !== null &&
        (unavailableSection as Record<string, unknown>).enabled === false)
    ) {
      nextSection.enabled = false
    }
    next[sectionKey] = nextSection
  }
  return next
}


export function SidebarModulesCard() {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [config, setConfig] = useState<SidebarModulesConfig>({})
  const currentUser = useAuthStore((s) => s.auth.user)
  const setUser = useAuthStore((s) => s.auth.setUser)
  const canSeeAdminModules =
    Boolean(currentUser?.role && currentUser.role >= ROLE.ADMIN) &&
    currentUser?.permissions?.sidebar_modules?.admin !== false

  const sectionDefs: SectionDef[] = [
    {
      key: 'chat',
      title: t('Chat Area'),
      description: t('Playground and chat functions'),
      modules: [
        {
          key: 'playground',
          title: t('Playground'),
          description: t('AI model testing environment'),
        },
        {
          key: 'chat',
          title: t('Chat'),
          description: t('Chat session management'),
        },
      ],
    },
    {
      key: 'console',
      title: t('Console Area'),
      description: t('Data management and log viewing'),
      modules: [
        {
          key: 'detail',
          title: t('Dashboard'),
          description: t('System data statistics'),
        },
        {
          key: 'token',
          title: t('Token Management'),
          description: t('API token management'),
        },
        {
          key: 'log',
          title: t('Usage Logs'),
          description: t('API usage records'),
        },
        {
          key: 'midjourney',
          title: t('Drawing Logs'),
          description: t('Drawing task records'),
        },
        {
          key: 'task',
          title: t('Task Logs'),
          description: t('System task records'),
        },
      ],
    },
    {
      key: 'personal',
      title: t('Personal Center Area'),
      description: t('User personal functions'),
      modules: [
        {
          key: 'topup',
          title: t('Wallet Management'),
          description: t('Balance and top-up management'),
        },
        {
          key: 'personal',
          title: t('Personal Settings'),
          description: t('Personal info settings'),
        },
      ],
    },
    {
      key: 'admin',
      title: t('Admin'),
      description: t('Manage and configure'),
      modules: [
        {
          key: 'channel',
          title: t('Channels'),
          description: t('Manage API channels and provider configurations'),
        },
        {
          key: 'models',
          title: t('Models'),
          description: t('Manage model metadata and configuration'),
        },
        {
          key: 'redemption',
          title: t('Redemption Codes'),
          description: t('Manage redemption codes for account balance top-up'),
        },
        {
          key: 'trial_code',
          title: t('Trial Codes'),
          description: t(
            'Manage manual trial codes for registration and OAuth account setup'
          ),
        },
        {
          key: 'subscription',
          title: t('Subscription Plans'),
          description: t(
            'Manage subscription plan creation, pricing and status'
          ),
        },
        {
          key: 'analytics',
          title: t('adminAnalytics.title'),
          description: t('adminAnalytics.description'),
        },
        {
          key: 'ops',
          title: t('adminOps.title'),
          description: t('adminOps.description'),
        },
        {
          key: 'trial_abuse',
          title: t('trialAbuse.title'),
          description: t('trialAbuse.description'),
        },
        {
          key: 'user',
          title: t('Users'),
          description: t('Manage users and their permissions'),
        },
        {
          key: 'setting',
          title: t('System Settings'),
          description: t('System settings'),
        },
      ],
    },
  ].filter((section) => section.key !== 'admin' || canSeeAdminModules)

  const loadConfig = useCallback(async () => {
    try {
      const res = await api.get('/api/user/self')
      if (res.data.success && res.data.data?.sidebar_modules) {
        const raw = res.data.data.sidebar_modules
        const parsed = typeof raw === 'string' ? JSON.parse(raw) : raw
        setConfig(applySidebarModulePermissions(parsed, currentUser?.permissions?.sidebar_modules))
      } else {
        setConfig(buildDefaultSidebarModulesForSections(sectionDefs, currentUser?.permissions?.sidebar_modules))
      }
    } catch {
      /* ignore */
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    loadConfig()
  }, [loadConfig])

  const toggleSection = (sectionKey: string, val: boolean) => {
    setConfig((prev) => ({
      ...prev,
      [sectionKey]: { ...prev[sectionKey], enabled: val },
    }))
  }

  const toggleModule = (
    sectionKey: string,
    moduleKey: string,
    val: boolean
  ) => {
    setConfig((prev) => ({
      ...prev,
      [sectionKey]: { ...prev[sectionKey], [moduleKey]: val },
    }))
  }

  const handleSave = async () => {
    setLoading(true)
    try {
      const sanitizedConfig = applySidebarModulePermissions(
        config,
        currentUser?.permissions?.sidebar_modules
      )
      const serialized = JSON.stringify(sanitizedConfig)
      const res = await api.put('/api/user/self', {
        sidebar_modules: serialized,
      })
      if (res.data.success) {
        // Sync to auth-store so useSidebarConfig re-runs and the sidebar
        // updates immediately without needing a page refresh.
        if (currentUser) {
          setUser({ ...currentUser, sidebar_modules: serialized })
        }
        toast.success(t('Saved successfully'))
      } else {
        toast.error(res.data.message || t('Save failed'))
      }
    } catch {
      toast.error(t('Save failed, please retry'))
    } finally {
      setLoading(false)
    }
  }

  const handleReset = () => {
    setConfig(
      buildDefaultSidebarModulesForSections(
        sectionDefs,
        currentUser?.permissions?.sidebar_modules
      )
    )
    toast.success(t('Reset to default configuration'))
  }

  return (
    <Card className='gap-0 overflow-hidden py-0'>
      <CardHeader className='border-b p-3 !pb-3 sm:p-5 sm:!pb-5'>
        <div className='flex items-center gap-3'>
          <div className='bg-muted flex h-8 w-8 shrink-0 items-center justify-center rounded-lg sm:h-9 sm:w-9'>
            <LayoutDashboard className='h-4 w-4' />
          </div>
          <div className='min-w-0'>
            <CardTitle className='text-lg tracking-tight sm:text-xl'>
              {t('Sidebar Personal Settings')}
            </CardTitle>
            <CardDescription className='text-xs sm:text-sm'>
              {t('Customize sidebar display content')}
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className='space-y-4 p-3 sm:space-y-5 sm:p-5'>
        {sectionDefs.map((section) => {
          const sectionEnabled = config[section.key]?.enabled !== false
          return (
            <div
              key={section.key}
              className='bg-background/60 rounded-xl border p-3'
            >
              <div className='flex items-start justify-between gap-3'>
                <div className='min-w-0'>
                  <p className='text-sm font-medium'>{section.title}</p>
                  <p className='text-muted-foreground text-xs'>
                    {section.description}
                  </p>
                </div>
                <Switch
                  checked={sectionEnabled}
                  onCheckedChange={(v) => toggleSection(section.key, v)}
                />
              </div>
              <div className='mt-3 grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-1'>
                {section.modules.map((mod) => (
                  <div
                    key={mod.key}
                    className={`flex min-h-16 items-center justify-between rounded-lg border p-3 transition-opacity ${
                      sectionEnabled ? '' : 'opacity-50'
                    }`}
                  >
                    <div className='mr-2 min-w-0'>
                      <p className='truncate text-sm font-medium'>
                        {mod.title}
                      </p>
                      <p className='text-muted-foreground truncate text-xs'>
                        {mod.description}
                      </p>
                    </div>
                    <Switch
                      checked={config[section.key]?.[mod.key] !== false}
                      onCheckedChange={(v) =>
                        toggleModule(section.key, mod.key, v)
                      }
                      disabled={!sectionEnabled}
                    />
                  </div>
                ))}
              </div>
            </div>
          )
        })}

        <div className='flex flex-col-reverse gap-2 border-t pt-4 sm:flex-row sm:justify-end'>
          <Button variant='outline' onClick={handleReset}>
            {t('Reset to Default')}
          </Button>
          <Button onClick={handleSave} disabled={loading}>
            {loading ? t('Saving...') : t('Save Changes')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
