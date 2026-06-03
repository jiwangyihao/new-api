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

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { describe, test } from 'node:test';

const localeFiles = {
  en: 'src/i18n/locales/en.json',
  zh: 'src/i18n/locales/zh.json',
  fr: 'src/i18n/locales/fr.json',
  ja: 'src/i18n/locales/ja.json',
  ru: 'src/i18n/locales/ru.json',
  vi: 'src/i18n/locales/vi.json',
  'zh-CN': 'src/i18n/locales/zh-CN.json',
  'zh-TW': 'src/i18n/locales/zh-TW.json',
};

const requiredAccountBalanceKeys = [
  '账户余额',
  '到账余额',
  '最低到账余额（CNY）',
  '每 1 CNY 到账余额实付单价',
  '渠道对每 1 CNY 到账余额收取的实付单价',
  '最低到账账户余额，单位为 CNY 元',
  '账户余额充值选项（CNY）',
  '到账余额折扣配置',
  '新用户初始账户余额',
  '邀请新用户奖励账户余额',
  '新用户使用邀请码奖励账户余额',
  '签到最小账户余额奖励（CNY）',
  '签到最大奖励账户余额（CNY）',
  '到账余额（CNY）',
  '输入 CNY 元，保存时会按分写入服务器',
  '当前余额',
];

describe('classic account balance i18n', () => {
  for (const [locale, filePath] of Object.entries(localeFiles)) {
    test(`${locale} has account balance translations`, () => {
      const json = JSON.parse(readFileSync(filePath, 'utf8'));
      assert.ok(Object.hasOwn(json, 'translation'));
      for (const key of requiredAccountBalanceKeys) {
        assert.ok(Object.hasOwn(json.translation, key), `${locale}: ${key}`);
        assert.notEqual(json.translation[key], '', `${locale}: ${key} is empty`);
      }
    });
  }
});
