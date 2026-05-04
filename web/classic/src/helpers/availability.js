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

/**
 * Get Semi UI Tag color name based on availability percentage and thresholds.
 * Used for <Tag color={...}> consumption.
 * @param {number|null} pct - Availability percentage (0-100)
 * @param {Object} thresholds - { green, red }
 * @returns {string} Semi UI Tag color: 'green' | 'yellow' | 'red' | 'grey'
 */
export const getAvailabilityColor = (pct, thresholds = { green: 99, red: 95 }) => {
  if (pct === null || pct === undefined) return 'grey';
  if (pct >= thresholds.green) return 'green';
  if (pct >= thresholds.red) return 'yellow';
  return 'red';
};

/**
 * Get the matching Semi UI semantic CSS variable suffix.
 * Returns one of 'success' / 'warning' / 'danger' / 'tertiary'
 * which are valid for `var(--semi-color-${suffix})` interpolation,
 * unlike the Tag color names which do not map to CSS vars.
 * @param {number|null} pct - Availability percentage (0-100)
 * @param {Object} thresholds - { green, red }
 * @returns {string} CSS variable suffix
 */
export const getAvailabilityCssColor = (pct, thresholds = { green: 99, red: 95 }) => {
  if (pct === null || pct === undefined) return 'tertiary';
  if (pct >= thresholds.green) return 'success';
  if (pct >= thresholds.red) return 'warning';
  return 'danger';
};

/**
 * Format availability percentage to string
 * @param {number|null} value - Availability percentage
 * @returns {string} Formatted string
 */
export const formatAvailability = (value) => {
  if (value === null || value === undefined) return '—';
  return `${value.toFixed(1)}%`;
};

/**
 * Get status preferring backend-provided status
 * @param {Object} item - { availability, status, ... }
 * @param {Object} thresholds - { green, red }
 * @returns {string} status 'green' | 'yellow' | 'red' | 'unknown'
 */
export const getStatusFromBackend = (item, thresholds = { green: 99, red: 95 }) => {
  if (item?.status && item.status !== 'unknown') {
    return item.status;
  }
  const pct = item?.availability;
  if (pct === null || pct === undefined) return 'unknown';
  if (pct >= thresholds.green) return 'green';
  if (pct >= thresholds.red) return 'yellow';
  return 'red';
};
