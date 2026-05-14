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
import { z } from 'zod'
import { createFileRoute, useNavigate, useSearch } from '@tanstack/react-router'
import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore, type AuthUser } from '@/stores/auth-store'
import { api, getSelf } from '@/lib/api'
import { useStatus } from '@/hooks/use-status'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { PasswordInput } from '@/components/password-input'
import { Turnstile } from '@/components/turnstile'
import { AuthLayout } from '@/features/auth/auth-layout'
import { sendEmailVerification } from '@/features/auth/api'
import { EMAIL_VERIFICATION_COUNTDOWN } from '@/features/auth/constants'
import { useCountdown } from '@/hooks/use-countdown'

const searchSchema = z.object({
  pending_token: z.string().optional(),
  redirect: z.string().optional(),
})

type OnboardingSearch = {
  pending_token?: string
  redirect?: string
}

type PendingInfo = {
  provider?: string
  login?: string
  email?: string
}

type OAuthOnboardingPendingResponse = {
  data?: {
    data?: PendingInfo
  }
}

function OAuthOnboardingPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const search = useSearch({
    from: '/(auth)/oauth-onboarding',
  }) as OnboardingSearch
  const { status } = useStatus()
  const [email, setEmail] = useState('')
  const [verificationCode, setVerificationCode] = useState('')
  const [trialCode, setTrialCode] = useState('')
  const [password, setPassword] = useState('')
  const [termsAccepted, setTermsAccepted] = useState(false)
  const [turnstileToken, setTurnstileToken] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [turnstileRefreshKey, setTurnstileRefreshKey] = useState(0)
  const [isSendingCode, setIsSendingCode] = useState(false)
  const [pendingInfo, setPendingInfo] = useState<PendingInfo>({})

  const pendingToken = search?.pending_token ?? ''
  const isTurnstileEnabled = Boolean(status?.turnstile_check)
  const turnstileSiteKey = String(status?.turnstile_site_key ?? '')
  const providerLabel = pendingInfo.provider || t('OAuth account')
  const hasProviderEmail = Boolean(pendingInfo.email)
  const requiresEmail = !hasProviderEmail

  const { secondsLeft, isActive, start: startEmailCountdown } = useCountdown({
    initialSeconds: EMAIL_VERIFICATION_COUNTDOWN,
  })
  useEffect(() => {
    if (!pendingToken) return
    let cancelled = false
    api
      .get('/api/oauth/onboarding', {
        params: { pending_token: pendingToken },
        skipBusinessError: true,
      } as never)
      .then((res: OAuthOnboardingPendingResponse) => {
        if (cancelled) return
        const data = res.data?.data ?? {}
        setPendingInfo({
          provider: data.provider,
          login: data.login,
          email: data.email,
        })
      })
      .catch(() => {
        if (!cancelled) {
          toast.error(t('OAuth onboarding session is invalid or expired'))
        }
      })
    return () => {
      cancelled = true
    }
  }, [pendingToken, t])

  async function sendVerificationCode() {
    const emailValue = email.trim()
    if (!emailValue) {
      toast.error(t('Please enter your email first'))
      return
    }
    if (isTurnstileEnabled && !turnstileToken) {
      toast.info(t('Please wait a moment, human check is initializing...'))
      return
    }
    setIsSendingCode(true)
    try {
      const res = await sendEmailVerification(emailValue, turnstileToken)
      if (res?.success) {
        startEmailCountdown()
        setTurnstileToken('')
        setTurnstileRefreshKey((key) => key + 1)
        toast.success(t('Verification email sent'))
      }
    } catch (_error) {
      // Global interceptors display the server error.
    } finally {
      setIsSendingCode(false)
    }
  }

  async function finishOnboarding() {
    if (!pendingToken) {
      toast.error(t('OAuth onboarding session is invalid or expired'))
      return
    }
    if (!termsAccepted) {
      toast.error(t('Please agree to the legal terms first'))
      return
    }
    if (isTurnstileEnabled && !turnstileToken) {
      toast.error(t('Please complete the Turnstile verification'))
      return
    }
    if (requiresEmail && !email.trim()) {
      toast.error(t('Please enter your email'))
      return
    }

    setIsLoading(true)
    try {
      const res = await api.post('/api/oauth/onboarding', {
        pending_token: pendingToken,
        email: email.trim() || undefined,
        verification_code: verificationCode.trim() || undefined,
        trial_code: trialCode.trim() || undefined,
        password: password || undefined,
        terms_accepted: termsAccepted,
        turnstile_token: turnstileToken,
      })
      if (res.data?.success) {
        const userData = res.data?.data as AuthUser | null | undefined
        if (userData) {
          useAuthStore.getState().auth.setUser(userData)
          if (userData.id != null) {
            window.localStorage.setItem('uid', String(userData.id))
          }
        } else {
          const self = await getSelf()
          if (self?.success && self.data) {
            const user = self.data as AuthUser
            useAuthStore.getState().auth.setUser(user)
            if (user.id != null) {
              window.localStorage.setItem('uid', String(user.id))
            }
          }
        }
        toast.success(t('Account created successfully'))
        navigate({ to: search?.redirect || '/dashboard', replace: true })
      }
    } catch (_error) {
      // Global interceptors display the server error.
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <AuthLayout>
      <Card className='w-full'>
        <CardHeader>
          <CardTitle>{t('Complete account setup')}</CardTitle>
          <CardDescription>
            {t(
              'Confirm your {{provider}} account before creating a platform account.',
              {
                provider: providerLabel,
              }
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-4'>
          {pendingInfo.login && (
            <div className='text-muted-foreground text-sm'>
              {t('OAuth login')}: {pendingInfo.login}
            </div>
          )}
          {hasProviderEmail ? (
            <div className='text-muted-foreground text-sm'>
              {t('Email')}: {pendingInfo.email}
            </div>
          ) : (
            <>
              <div className='grid gap-2'>
                <Label htmlFor='oauth-onboarding-email'>{t('Email')}</Label>
                <Input
                  id='oauth-onboarding-email'
                  type='email'
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  placeholder='name@example.com'
                />
              </div>
              <div className='grid gap-2'>
                <Label htmlFor='oauth-onboarding-verification-code'>
                  {t('Email verification code')}
                </Label>
                <div className='flex items-center gap-2'>
                  <Input
                    id='oauth-onboarding-verification-code'
                    value={verificationCode}
                    onChange={(event) => setVerificationCode(event.target.value)}
                    placeholder={t('Enter verification code')}
                  />
                  <Button
                    type='button'
                    variant='outline'
                    disabled={isSendingCode || isActive || !email.trim()}
                    onClick={sendVerificationCode}
                  >
                    {isActive ? (
                      t('Resend ({{seconds}}s)', { seconds: secondsLeft })
                    ) : isSendingCode ? (
                      <Loader2 className='h-4 w-4 animate-spin' />
                    ) : (
                      t('Send code')
                    )}
                  </Button>
                </div>
              </div>
            </>
          )}
          <div className='grid gap-2'>
            <Label htmlFor='oauth-onboarding-password'>{t('Password')}</Label>
            <PasswordInput
              id='oauth-onboarding-password'
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              placeholder={t('Set a password for later sign-in')}
            />
          </div>
          <div className='grid gap-2'>
            <Label htmlFor='oauth-onboarding-trial-code'>
              {t('Trial code')}
            </Label>
            <Input
              id='oauth-onboarding-trial-code'
              value={trialCode}
              onChange={(event) => setTrialCode(event.target.value)}
              placeholder={t('Enter trial code if you have one')}
            />
          </div>
          <div className='flex items-start gap-2'>
            <Checkbox
              id='oauth-onboarding-terms'
              checked={termsAccepted}
              onCheckedChange={(value) => setTermsAccepted(value === true)}
            />
            <Label htmlFor='oauth-onboarding-terms' className='font-normal'>
              {t('I have read and agree to the terms and privacy policy')}
            </Label>
          </div>
          {isTurnstileEnabled && (
            <Turnstile
              key={turnstileRefreshKey}
              siteKey={turnstileSiteKey}
              onVerify={setTurnstileToken}
              onExpire={() => setTurnstileToken('')}
            />
          )}
          <Button
            className='w-full'
            onClick={finishOnboarding}
            disabled={isLoading}
          >
            {isLoading && <Loader2 className='animate-spin' />}
            {t('Create account')}
          </Button>
        </CardContent>
      </Card>
    </AuthLayout>
  )
}

export const Route = createFileRoute('/(auth)/oauth-onboarding')({
  component: OAuthOnboardingPage,
  validateSearch: searchSchema,
})
