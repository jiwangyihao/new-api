/*
Copyright (C) 2025 QuantumNous

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

import React, { useContext, useEffect, useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import Turnstile from 'react-turnstile';
import {
  Button,
  Card,
  Checkbox,
  Form,
  Input,
  Typography,
} from '@douyinfe/semi-ui';
import {
  API,
  showError,
  showSuccess,
  setUserData,
  updateAPI,
} from '../../helpers';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';

const OAuthOnboarding = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [, userDispatch] = useContext(UserContext);
  const [statusState] = useContext(StatusContext);
  const [pendingInfo, setPendingInfo] = useState({});
  const [inputs, setInputs] = useState({
    email: '',
    verification_code: '',
    password: '',
    trial_code: '',
  });
  const [termsAccepted, setTermsAccepted] = useState(false);
  const [turnstileToken, setTurnstileToken] = useState('');
  const [turnstileRefreshKey, setTurnstileRefreshKey] = useState(0);
  const [loading, setLoading] = useState(false);
  const [verificationCodeLoading, setVerificationCodeLoading] = useState(false);
  const [disableButton, setDisableButton] = useState(false);
  const [countdown, setCountdown] = useState(30);

  const pendingToken = searchParams.get('pending_token') || '';
  const redirect = searchParams.get('redirect') || '/console/token';
  const status = useMemo(() => {
    if (statusState?.status) return statusState.status;
    const savedStatus = localStorage.getItem('status');
    if (!savedStatus) return {};
    try {
      return JSON.parse(savedStatus) || {};
    } catch (_error) {
      return {};
    }
  }, [statusState?.status]);
  const turnstileEnabled = Boolean(status?.turnstile_check);
  const turnstileSiteKey = status?.turnstile_site_key || '';
  const hasProviderEmail = Boolean(pendingInfo.email);

  useEffect(() => {
    if (!pendingToken) return;
    API.get('/api/oauth/onboarding', {
      params: { pending_token: pendingToken },
      disableDuplicate: true,
    })
      .then((res) => {
        if (res.data?.success) setPendingInfo(res.data.data || {});
      })
      .catch(() => showError(t('OAuth 建号会话无效或已过期')));
  }, [pendingToken, t]);

  useEffect(() => {
    let countdownInterval = null;
    if (disableButton && countdown > 0) {
      countdownInterval = setInterval(() => {
        setCountdown(countdown - 1);
      }, 1000);
    } else if (countdown === 0) {
      setDisableButton(false);
      setCountdown(30);
    }
    return () => clearInterval(countdownInterval);
  }, [disableButton, countdown]);

  const handleChange = (name, value) => {
    setInputs((current) => ({ ...current, [name]: value }));
  };


  const sendVerificationCode = async () => {
    if (!inputs.email.trim()) {
      showError(t('请输入邮箱！'));
      return;
    }
    if (turnstileEnabled && !turnstileToken) {
      showError(t('请稍后几秒重试，Turnstile 正在检查用户环境！'));
      return;
    }
    setVerificationCodeLoading(true);
    try {
      const res = await API.get(
        `/api/verification?email=${encodeURIComponent(inputs.email.trim())}&turnstile=${turnstileToken}`,
      );
      const { success, message } = res.data || {};
      if (success) {
        showSuccess(t('验证码发送成功，请检查你的邮箱！'));
        setTurnstileToken('');
        setTurnstileRefreshKey((key) => key + 1);
        setDisableButton(true);
      } else {
        showError(message || t('验证码发送失败'));
      }
    } catch (error) {
      showError(error);
    } finally {
      setVerificationCodeLoading(false);
    }
  };
  const submit = async () => {
    if (!pendingToken) {
      showError(t('OAuth 建号会话无效或已过期'));
      return;
    }
    if (!termsAccepted) {
      showError(t('请先阅读并同意用户协议和隐私政策'));
      return;
    }
    if (turnstileEnabled && !turnstileToken) {
      showError(t('请稍后几秒重试，Turnstile 正在检查用户环境！'));
      return;
    }
    if (!hasProviderEmail && !inputs.email.trim()) {
      showError(t('请输入邮箱地址'));
      return;
    }
    setLoading(true);
    try {
      const res = await API.post('/api/oauth/onboarding', {
        pending_token: pendingToken,
        email: inputs.email.trim() || undefined,
        verification_code: inputs.verification_code.trim() || undefined,
        password: inputs.password || undefined,
        trial_code: inputs.trial_code.trim() || undefined,
        terms_accepted: termsAccepted,
        turnstile_token: turnstileToken,
      });
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(message || t('授权失败'));
        return;
      }
      const user = data;
      if (user) {
        userDispatch({ type: 'login', payload: user });
        localStorage.setItem('user', JSON.stringify(user));
        setUserData(user);
        updateAPI();
      }
      showSuccess(t('登录成功！'));
      navigate(redirect);
    } catch (error) {
      showError(error);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className='relative overflow-hidden bg-gray-100 flex items-center justify-center py-12 px-4 sm:px-6 lg:px-8'>
      <div className='w-full max-w-md mt-[60px]'>
        <Card className='border-0 !rounded-2xl overflow-hidden'>
          <div className='mb-6 text-center'>
            <Typography.Title heading={3}>{t('完成账号创建')}</Typography.Title>
            <Typography.Text type='secondary'>
              {t('请确认 OAuth 账号信息后创建平台账号')}
            </Typography.Text>
          </div>
          {pendingInfo.login && (
            <Typography.Paragraph>
              {t('OAuth 账号')}：{pendingInfo.login}
            </Typography.Paragraph>
          )}
          {hasProviderEmail ? (
            <Typography.Paragraph>
              {t('邮箱')}：{pendingInfo.email}
            </Typography.Paragraph>
          ) : (
            <>
              <Form.Input
                field='email'
                label={t('邮箱')}
                placeholder='name@example.com'
                value={inputs.email}
                onChange={(value) => handleChange('email', value)}
              />
              <Form.Input
                field='verification_code'
                label={t('邮箱验证码')}
                placeholder={t('验证码')}
                value={inputs.verification_code}
                onChange={(value) => handleChange('verification_code', value)}
                suffix={
                  <Button
                    onClick={sendVerificationCode}
                    loading={verificationCodeLoading}
                    disabled={disableButton || verificationCodeLoading}
                  >
                    {disableButton
                      ? `${t('重新发送')} (${countdown})`
                      : t('获取验证码')}
                  </Button>
                }
              />
            </>
          )}
          <Form.Input
            field='password'
            label={t('密码')}
            type='password'
            placeholder={t('设置密码，之后可用账号密码登录')}
            value={inputs.password}
            onChange={(value) => handleChange('password', value)}
          />
          <Form.Input
            field='trial_code'
            label={t('试用码')}
            placeholder={t('如有试用码请填写')}
            value={inputs.trial_code}
            onChange={(value) => handleChange('trial_code', value)}
          />
          <Checkbox
            checked={termsAccepted}
            onChange={(event) => setTermsAccepted(event.target.checked)}
          >
            {t('我已阅读并同意用户协议和隐私政策')}
          </Checkbox>
          {turnstileEnabled && (
            <div className='flex justify-center mt-4'>
              <Turnstile
                key={turnstileRefreshKey}
                sitekey={turnstileSiteKey}
                onVerify={setTurnstileToken}
              />
            </div>
          )}
          <Button
            theme='solid'
            type='primary'
            block
            loading={loading}
            onClick={submit}
            className='mt-6'
          >
            {t('创建账号')}
          </Button>
        </Card>
      </div>
    </div>
  );
};

export default OAuthOnboarding;
