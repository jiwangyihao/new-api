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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useStatus } from '@/hooks/use-status'
import signUpAnimeGirlBackground01 from '../assets/sign-up-backgrounds/signup-moe-native-01.png'
import signUpAnimeGirlBackground02 from '../assets/sign-up-backgrounds/signup-moe-native-02.png'
import signUpAnimeGirlBackground03 from '../assets/sign-up-backgrounds/signup-moe-native-03.png'
import signUpAnimeGirlBackground04 from '../assets/sign-up-backgrounds/signup-moe-native-04.png'
import signUpAnimeGirlBackground05 from '../assets/sign-up-backgrounds/signup-moe-native-05.png'
import signUpAnimeGirlBackground06 from '../assets/sign-up-backgrounds/signup-moe-native-06.png'
import signUpAnimeGirlBackground07 from '../assets/sign-up-backgrounds/signup-moe-native-07.png'
import signUpAnimeGirlBackground08 from '../assets/sign-up-backgrounds/signup-moe-native-08.png'
import signUpAnimeGirlBackground09 from '../assets/sign-up-backgrounds/signup-moe-native-09.png'
import signUpAnimeGirlBackground10 from '../assets/sign-up-backgrounds/signup-moe-native-10.png'
import signUpAnimeGirlBackground11 from '../assets/sign-up-backgrounds/signup-moe-native-11.png'
import signUpAnimeGirlBackground12 from '../assets/sign-up-backgrounds/signup-moe-native-12.png'
import signUpAnimeGirlBackground13 from '../assets/sign-up-backgrounds/signup-moe-native-13.png'
import signUpAnimeGirlBackground14 from '../assets/sign-up-backgrounds/signup-moe-native-14.png'
import signUpAnimeGirlBackground15 from '../assets/sign-up-backgrounds/signup-moe-native-15.png'
import signUpAnimeGirlBackground16 from '../assets/sign-up-backgrounds/signup-moe-native-16.png'
import signUpAnimeGirlBackground17 from '../assets/sign-up-backgrounds/signup-moe-native-17.png'
import signUpAnimeGirlBackground18 from '../assets/sign-up-backgrounds/signup-moe-native-18.png'
import signUpAnimeGirlBackground19 from '../assets/sign-up-backgrounds/signup-moe-native-19.png'
import signUpAnimeGirlBackground20 from '../assets/sign-up-backgrounds/signup-moe-native-20.png'
import { AuthLayout } from '../auth-layout'
import { TermsFooter } from '../components/terms-footer'
import { SignUpForm } from './components/sign-up-form'

const signUpAnimeGirlBackgrounds = [
  signUpAnimeGirlBackground01,
  signUpAnimeGirlBackground02,
  signUpAnimeGirlBackground03,
  signUpAnimeGirlBackground04,
  signUpAnimeGirlBackground05,
  signUpAnimeGirlBackground06,
  signUpAnimeGirlBackground07,
  signUpAnimeGirlBackground08,
  signUpAnimeGirlBackground09,
  signUpAnimeGirlBackground10,
  signUpAnimeGirlBackground11,
  signUpAnimeGirlBackground12,
  signUpAnimeGirlBackground13,
  signUpAnimeGirlBackground14,
  signUpAnimeGirlBackground15,
  signUpAnimeGirlBackground16,
  signUpAnimeGirlBackground17,
  signUpAnimeGirlBackground18,
  signUpAnimeGirlBackground19,
  signUpAnimeGirlBackground20,
]

const signUpAnimeGirlBackground =
  signUpAnimeGirlBackgrounds[
    Math.floor(Math.random() * signUpAnimeGirlBackgrounds.length)
  ]

export function SignUp() {
  const { t } = useTranslation()
  const { status } = useStatus()

  return (
    <AuthLayout backgroundImageSrc={signUpAnimeGirlBackground}>
      <div className='w-full space-y-8'>
        <div className='space-y-2'>
          <h2 className='text-center text-2xl font-semibold tracking-tight sm:text-left'>
            {t('Create an account')}
          </h2>
          <p className='text-muted-foreground text-left text-sm sm:text-base'>
            {t('Already have an account?')}{' '}
            <Link
              to='/sign-in'
              className='hover:text-primary font-medium underline underline-offset-4'
            >
              {t('Sign in')}
            </Link>
            .
          </p>
        </div>

        <SignUpForm />

        <TermsFooter
          variant='sign-up'
          status={status}
          className='text-center'
        />
      </div>
    </AuthLayout>
  )
}
