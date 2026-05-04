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

import { useState, useEffect, useCallback, useRef } from 'react';
import { API, showError } from '../../helpers';

// Module-level shared cache for per-model groups data; survives across hook
// instances so PricingTable + ModelDetailSideSheet share TTL benefits.
const groupsCache = new Map();
const GROUPS_CACHE_TTL_MS = 30000;
const GROUPS_CACHE_MAX_ENTRIES = 256;
// Random jitter applied to the polling interval to avoid the synchronized-wakeup
// thundering herd when many tabs/users resume from sleep at the same instant.
const POLL_JITTER_MS = 5000;

// Shallow equality check for the overview map: skips state updates when the
// server returns identical data, avoiding cascade re-renders of the price table.
const overviewEqual = (a, b) => {
  if (a === b) return true;
  const aKeys = Object.keys(a);
  const bKeys = Object.keys(b);
  if (aKeys.length !== bKeys.length) return false;
  for (const k of aKeys) {
    const av = a[k];
    const bv = b[k];
    if (!bv) return false;
    if (av.availability !== bv.availability || av.status !== bv.status) return false;
  }
  return true;
};

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

  // Track the latest in-flight overview request so we can cancel it on
  // unmount or when a new fetch starts before the previous one finished.
  const abortRef = useRef(null);
  // Track mount state to avoid React state updates after unmount.
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (abortRef.current) abortRef.current.abort();
    };
  }, []);

  const loadOverview = useCallback(async (isAutoRefresh = false) => {
    // Cancel any prior in-flight overview request to avoid out-of-order
    // updates and to release sockets promptly.
    if (abortRef.current) abortRef.current.abort();
    const controller = new AbortController();
    abortRef.current = controller;

    if (!isAutoRefresh && mountedRef.current) setLoading(true);
    try {
      const res = await API.get('/api/availability/models', { signal: controller.signal });
      if (!mountedRef.current || controller.signal.aborted) return;
      const { success, message, data } = res.data;
      if (success && data) {
        const itemsMap = {};
        if (Array.isArray(data.items)) {
          data.items.forEach((item) => {
            itemsMap[item.model] = {
              availability: item.availability,
              status: item.status,
            };
          });
        }
        setOverview((prev) => (overviewEqual(prev, itemsMap) ? prev : itemsMap));
        if (data.thresholds) {
          setThresholds((prev) =>
            prev.green === data.thresholds.green && prev.red === data.thresholds.red
              ? prev
              : data.thresholds,
          );
        }
        if (data.window_seconds) setWindowSeconds(data.window_seconds);
        setError(null);
      } else if (message) {
        setError(message);
      }
    } catch (err) {
      // Aborted requests are expected on unmount / re-fetch — ignore them.
      if (err?.name === 'CanceledError' || err?.code === 'ERR_CANCELED' || controller.signal.aborted) {
        return;
      }
      console.error('Failed to load model availability:', err);
      if (mountedRef.current) setError(err.message);
    } finally {
      if (!isAutoRefresh && mountedRef.current) setLoading(false);
    }
  }, []);

  const getModelGroupsData = useCallback(
    async (modelName) => {
      if (!modelName || typeof modelName !== 'string') {
        return { items: [], thresholds };
      }
      const now = Date.now();
      const cached = groupsCache.get(modelName);

      if (cached && now - cached.timestamp < GROUPS_CACHE_TTL_MS) {
        return cached.data;
      }

      try {
        const res = await API.get(
          `/api/availability/groups?model=${encodeURIComponent(modelName)}`,
        );
        const { success, message, data } = res.data;
        if (success && data) {
          const result = {
            items: data.items || [],
            thresholds: data.thresholds || thresholds,
          };
          if (groupsCache.size >= GROUPS_CACHE_MAX_ENTRIES) {
            // Drop a single oldest-ish entry instead of clearing all so a
            // power user opening many models cannot evict every cached entry.
            const firstKey = groupsCache.keys().next().value;
            if (firstKey !== undefined) groupsCache.delete(firstKey);
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
    },
    [thresholds],
  );

  const refresh = useCallback(() => loadOverview(false), [loadOverview]);

  useEffect(() => {
    if (!enabled || !fetchOverview) return undefined;

    // Only poll when the document is visible. Wakes immediately when the user
    // switches back to the tab via the visibilitychange listener below.
    const isVisible = () =>
      typeof document === 'undefined' || document.visibilityState !== 'hidden';

    if (isVisible()) loadOverview();

    // Add up to ±POLL_JITTER_MS of randomness so 5000 tabs do not all hit the
    // server at the same wall-clock second.
    const jitter = Math.floor(Math.random() * POLL_JITTER_MS);
    const tick = () => {
      if (isVisible()) loadOverview(true);
    };
    const timer = setInterval(tick, refreshIntervalMs + jitter);

    const onVisibilityChange = () => {
      if (isVisible()) loadOverview(true);
    };
    if (typeof document !== 'undefined') {
      document.addEventListener('visibilitychange', onVisibilityChange);
    }

    return () => {
      clearInterval(timer);
      if (typeof document !== 'undefined') {
        document.removeEventListener('visibilitychange', onVisibilityChange);
      }
    };
  }, [enabled, fetchOverview, refreshIntervalMs, loadOverview]);

  return {
    overview,
    getModelGroupsData,
    refresh,
    loading,
    error,
    thresholds,
    windowSeconds,
  };
};
