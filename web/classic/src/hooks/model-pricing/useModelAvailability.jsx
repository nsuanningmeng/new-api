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

import { useState, useEffect, useCallback } from 'react';
import { API, showError } from '../../helpers';

// Module-level shared cache for per-model groups data; survives across hook
// instances so PricingTable + ModelDetailSideSheet share TTL benefits.
const groupsCache = new Map();
const GROUPS_CACHE_TTL_MS = 30000;
const GROUPS_CACHE_MAX_ENTRIES = 256;

export const useModelAvailability = ({
  enabled = true,
  refreshIntervalMs = 60000,
  fetchOverview = true,
} = {}) => {
  const [overview, setOverview] = useState({});
  const [thresholds, setThresholds] = useState({ green: 99, red: 95 });
  const [windowSeconds, setWindowSeconds] = useState(3600);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  const loadOverview = useCallback(async (isAutoRefresh = false) => {
    if (!isAutoRefresh) setLoading(true);
    try {
      const res = await API.get('/api/availability/models');
      const { success, message, data } = res.data;
      if (success && data) {
        const itemsMap = {};
        if (Array.isArray(data.items)) {
          data.items.forEach(item => {
            itemsMap[item.model] = {
              availability: item.availability,
              status: item.status,
              success_count: item.success_count,
              total_count: item.total_count
            };
          });
        }
        setOverview(itemsMap);
        if (data.thresholds) setThresholds(data.thresholds);
        if (data.window_seconds) setWindowSeconds(data.window_seconds);
        setError(null);
      } else if (message) {
        setError(message);
      }
    } catch (err) {
      console.error('Failed to load model availability:', err);
      setError(err.message);
    } finally {
      if (!isAutoRefresh) setLoading(false);
    }
  }, []);

  const getModelGroupsData = useCallback(async (modelName) => {
    if (!modelName || typeof modelName !== 'string') {
      return { items: [], thresholds };
    }
    const now = Date.now();
    const cached = groupsCache.get(modelName);

    if (cached && now - cached.timestamp < GROUPS_CACHE_TTL_MS) {
      return cached.data;
    }

    try {
      const res = await API.get(`/api/availability/groups?model=${encodeURIComponent(modelName)}`);
      const { success, message, data } = res.data;
      if (success && data) {
        const result = {
          items: data.items || [],
          thresholds: data.thresholds || thresholds,
        };
        if (groupsCache.size >= GROUPS_CACHE_MAX_ENTRIES) {
          groupsCache.clear();
        }
        groupsCache.set(modelName, { timestamp: now, data: result });
        return result;
      } else if (message) {
        showError(message);
      }
    } catch (err) {
      console.error(`Failed to load group availability for ${modelName}:`, err);
    }
    return { items: [], thresholds };
  }, [thresholds]);

  const refresh = useCallback(() => loadOverview(false), [loadOverview]);

  useEffect(() => {
    if (!enabled || !fetchOverview) return;

    loadOverview();
    const timer = setInterval(() => loadOverview(true), refreshIntervalMs);

    return () => clearInterval(timer);
  }, [enabled, fetchOverview, refreshIntervalMs, loadOverview]);

  return {
    overview,
    getModelGroupsData,
    refresh,
    loading,
    error,
    thresholds,
    windowSeconds
  };
};
