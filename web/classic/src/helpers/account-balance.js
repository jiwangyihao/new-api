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

export const accountBalanceCentsToCnyAmount = (cents) => {
  const value = Number(cents || 0);
  if (!Number.isFinite(value)) return 0;
  return value / 100;
};

export const accountBalanceCnyToCents = (amount) => {
  const value = Number(amount || 0);
  if (!Number.isFinite(value)) return 0;
  return Math.round(value * 100);
};

export const formatAccountBalance = (cents) => {
  return `¥${accountBalanceCentsToCnyAmount(cents).toFixed(2)}`;
};
