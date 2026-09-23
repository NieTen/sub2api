import { buildCcSwitchImportDeeplink, OPENAI_CC_SWITCH_CODEX_MODEL, resolveCcSwitchImportConfig } from '@/utils/ccswitchImport'
import { formatDateLocalInput } from '@/utils/format'

let pageInstanceSequence = 0

// 保留原页面模板，由 Vue 提供独立容器、路由和已认证的请求客户端。
export function createSidebarPageOverlay({ target, navigate, request }) {
  const PAGE_CONFIG = {
    requestTimeoutMs: 15000,
    ccSwitchCodexModel: OPENAI_CC_SWITCH_CODEX_MODEL,
  };
  const PAGE_ROOT_ID = `sub2api-page-overlay-root-${++pageInstanceSequence}`;
  const PAGE_STYLE_ID = `${PAGE_ROOT_ID}-style`;
  const DASHBOARD_DAYS_STATE_KEY = 'dashboardDays';
  const pendingControllers = new Set();
  const pendingTimers = new Set();
  const pageState = {
    [DASHBOARD_DAYS_STATE_KEY]: 7,
    guideStep: 1,
    integrationMode: 'apps',
    selectedApp: 'ccswitch',
    selectedProtocol: 'openai',
    selectedKeyId: '',
    selectedModel: '',
    keyMenuOpen: false,
    settings: null,
    keys: [],
    models: [],
    modelCatalog: [],
  };
  const root = document.createElement('div');
  root.id = PAGE_ROOT_ID;
  let pageStyle = null;
  let pageRequestVersion = 0;
  let pageViewVersion = 0;
  let connectionRequestVersion = 0;
  let destroyed = false;

  function debugLog(...args) {
    if (import.meta.env.DEV) console.debug('[Sub2API 页面]', ...args);
  }

  function isCurrentPage(version, mode) {
    return !destroyed && version === pageRequestVersion &&
      root.parentElement === target && root.isConnected && root.dataset.pageMode === mode;
  }

  function isCurrentView(version, viewVersion) {
    return isCurrentPage(version, root.dataset.pageMode) && viewVersion === pageViewVersion;
  }

  function schedule(callback, delay) {
    const timer = window.setTimeout(() => {
      pendingTimers.delete(timer);
      if (!destroyed) callback();
    }, delay);
    pendingTimers.add(timer);
    return timer;
  }

  function clearPendingWork() {
    for (const controller of pendingControllers) controller.abort();
    pendingControllers.clear();
    for (const timer of pendingTimers) window.clearTimeout(timer);
    pendingTimers.clear();
  }

  // API Key 连通性探测单独使用 fetch，并在换页或卸载时取消。
  async function fetchWithTimeout(path, options = {}, readResponse = (response) => response) {
    const controller = new AbortController();
    pendingControllers.add(controller);
    const timer = schedule(() => controller.abort(), PAGE_CONFIG.requestTimeoutMs);
    try {
      const response = await window.fetch(path, { ...options, signal: controller.signal });
      return await readResponse(response);
    } finally {
      pendingControllers.delete(controller);
      pendingTimers.delete(timer);
      window.clearTimeout(timer);
    }
  }

  function escapeHtml(value) {
    return String(value ?? '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  function formatNumber(value) {
    const number = Number(value || 0);
    return Number.isFinite(number) ? number.toLocaleString('zh-CN') : '0';
  }

  function formatCompactNumber(value) {
    const number = Number(value || 0);
    if (!Number.isFinite(number)) return '0';
    if (Math.abs(number) >= 1000000000) {
      return `${(number / 1000000000).toFixed(2).replace(/\.00$/, '')}B`;
    }
    if (Math.abs(number) >= 1000000) {
      return `${(number / 1000000).toFixed(2).replace(/\.00$/, '')}M`;
    }
    if (Math.abs(number) >= 1000) {
      return `${(number / 1000).toFixed(1).replace(/\.0$/, '')}K`;
    }
    return formatNumber(number);
  }

  function formatMoney(value) {
    const number = Number(value || 0);
    return `$${(Number.isFinite(number) ? number : 0).toLocaleString('en-US', {
      minimumFractionDigits: 2,
      maximumFractionDigits: 4,
    })}`;
  }

  function formatDuration(value) {
    const number = Number(value || 0);
    if (!Number.isFinite(number) || number <= 0) return '0ms';
    if (number >= 1000) return `${(number / 1000).toFixed(2)}s`;
    return `${Math.round(number)}ms`;
  }

  function icon(name, size) {
    const paths = {
      activity: '<path d="M3 12h4l2-7 4 14 2-7h6"/>',
      arrow: '<path d="M5 12h14M13 6l6 6-6 6"/>',
      back: '<path d="M19 12H5m6 6-6-6 6-6"/>',
      book: '<path d="M4 5.5A2.5 2.5 0 0 1 6.5 3H11v16H6.5A2.5 2.5 0 0 0 4 21.5z"/><path d="M20 5.5A2.5 2.5 0 0 0 17.5 3H13v16h4.5a2.5 2.5 0 0 1 2.5 2.5z"/>',
      check: '<path d="m5 12 4 4L19 6"/>',
      chevron: '<path d="m6 9 6 6 6-6"/>',
      code: '<path d="m8 9-3 3 3 3m8-6 3 3-3 3m-2-9-4 12"/>',
      copy: '<rect x="8" y="8" width="11" height="11" rx="2"/><path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3"/>',
      database: '<ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v7c0 1.7 3.6 3 8 3s8-1.3 8-3V5M4 12v7c0 1.7 3.6 3 8 3s8-1.3 8-3v-7"/>',
      external: '<path d="M15 3h6v6m0-6-9 9"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>',
      key: '<circle cx="8" cy="15" r="4"/><path d="m11 12 9-9m-3 3 3 3m-6 0 3 3"/>',
      model: '<rect x="3" y="3" width="7" height="7" rx="2"/><rect x="14" y="3" width="7" height="7" rx="2"/><rect x="3" y="14" width="7" height="7" rx="2"/><rect x="14" y="14" width="7" height="7" rx="2"/>',
      play: '<path d="m8 5 11 7-11 7z"/>',
      refresh: '<path d="M20 11a8 8 0 1 0 2 5.5M20 4v7h-7"/>',
      rocket: '<path d="M4.5 16.5c-1.5 1.2-2 4-2 4s2.8-.5 4-2M9 15l-3-3c2.3-5.4 6-8.4 12-9 1.2 6-1.8 9.7-7.2 12z"/><path d="M14 10h.01M9 15l-1 5 3-2 2-3"/>',
      send: '<path d="m22 2-7 20-4-9-9-4Z"/><path d="M22 2 11 13"/>',
      shield: '<path d="M12 22s8-3.8 8-10V5l-8-3-8 3v7c0 6.2 8 10 8 10z"/><path d="m9 12 2 2 4-4"/>',
      sparkles: '<path d="m12 3-1 3.5L7.5 8 11 9.5 12 13l1-3.5L16.5 8 13 6.5zM5 14l-.7 2.3L2 17l2.3.7L5 20l.7-2.3L8 17l-2.3-.7zM19 13l-.7 2.3-2.3.7 2.3.7L19 19l.7-2.3L22 16l-2.3-.7z"/>',
      terminal: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="m7 9 3 3-3 3m5 0h5"/>',
      timer: '<circle cx="12" cy="13" r="8"/><path d="M12 9v4l3 2M9 2h6"/>',
      wallet: '<path d="M4 5h14a2 2 0 0 1 2 2v12H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h13"/><path d="M16 11h6v4h-6a2 2 0 0 1 0-4z"/>',
    };
    return `<svg class="s2-icon" width="${size || 18}" height="${size || 18}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${paths[name] || paths.model}</svg>`;
  }

  function readObjectField(source, fieldName, fallbackValue) {
    if (!source || typeof source !== 'object') return fallbackValue;
    return Object.prototype.hasOwnProperty.call(source, fieldName)
      ? source[fieldName]
      : fallbackValue;
  }

  // 认证、刷新令牌和接口错误处理均交给现有 apiClient。
  async function pageApiFetch(path) {
    return request(path);
  }

  function dateRange(days) {
    const end = new Date();
    const start = new Date(end);
    start.setDate(start.getDate() - (days - 1));
    // 与 apiClient 传递的浏览器时区一致，避免本地凌晨查询到前一天。
    return { start: formatDateLocalInput(start), end: formatDateLocalInput(end) };
  }

  async function loadDashboardData(days) {
    const safeDays = days === 30 ? 30 : 7;
    const range = dateRange(safeDays);
    const query = `?start_date=${range.start}&end_date=${range.end}&granularity=day`;
    const results = await Promise.allSettled([
      pageApiFetch('/auth/me'),
      pageApiFetch('/usage/dashboard/stats'),
      pageApiFetch(`/usage/dashboard/trend${query}`),
      pageApiFetch(`/usage/dashboard/models${query}`),
    ]);
    return {
      user: results[0].status === 'fulfilled' ? results[0].value || {} : {},
      stats: results[1].status === 'fulfilled' ? results[1].value || {} : {},
      trend:
        results[2].status === 'fulfilled'
          ? results[2].value?.trend || []
          : [],
      models:
        results[3].status === 'fulfilled'
          ? results[3].value?.models || []
          : [],
      days: safeDays,
      failed: results.every((result) => result.status === 'rejected'),
    };
  }

  async function loadGuideData() {
    const results = await Promise.allSettled([
      pageApiFetch('/settings/public'),
      pageApiFetch('/keys?page=1&page_size=100'),
      pageApiFetch('/usage/dashboard/models'),
      pageApiFetch('/settings/home-models'),
    ]);
    const settings =
      results[0].status === 'fulfilled' ? results[0].value || {} : {};
    const keyPayload =
      results[1].status === 'fulfilled' ? results[1].value || {} : {};
    const modelPayload =
      results[2].status === 'fulfilled' ? results[2].value || {} : {};
    return {
      settings,
      keys: Array.isArray(keyPayload) ? keyPayload : keyPayload.items || [],
      models: modelPayload.models || [],
      modelCatalog:
        results[3].status === 'fulfilled' && Array.isArray(results[3].value)
          ? results[3].value
          : [],
    };
  }

  function normalizeDashboardTrend(trend, days) {
    const safeDays = days === 30 ? 30 : 7;
    const rowsByDate = new Map(
      (Array.isArray(trend) ? trend : []).map((item) => [
        String(item.date || '').slice(0, 10),
        item,
      ]),
    );
    const range = dateRange(safeDays);
    const start = new Date(`${range.start}T00:00:00Z`);
    return Array.from({ length: safeDays }, (_, index) => {
      const date = new Date(start.getTime() + index * 86400000)
        .toISOString()
        .slice(0, 10);
      return (
        rowsByDate.get(date) || {
          date,
          requests: 0,
          total_tokens: 0,
          actual_cost: 0,
        }
      );
    });
  }

  function buildSmoothPath(points) {
    if (!points.length) return '';
    if (points.length === 1) return `M ${points[0].x} ${points[0].y}`;
    let path = `M ${points[0].x} ${points[0].y}`;
    for (let index = 0; index < points.length - 1; index += 1) {
      const previous = points[index - 1] || points[index];
      const current = points[index];
      const next = points[index + 1];
      const afterNext = points[index + 2] || next;
      const controlOneX = current.x + (next.x - previous.x) / 6;
      const controlOneY = current.y + (next.y - previous.y) / 6;
      const controlTwoX = next.x - (afterNext.x - current.x) / 6;
      const controlTwoY = next.y - (afterNext.y - current.y) / 6;
      path += ` C ${controlOneX} ${controlOneY}, ${controlTwoX} ${controlTwoY}, ${next.x} ${next.y}`;
    }
    return path;
  }

  function dashboardWeekday(dateValue) {
    const date = new Date(`${String(dateValue || '').slice(0, 10)}T00:00:00`);
    if (Number.isNaN(date.getTime())) return '';
    return new Intl.DateTimeFormat('zh-CN', { weekday: 'short' }).format(date);
  }

  function buildTrendSvg(trend, days) {
    const safeDays = days === 30 ? 30 : 7;
    const normalizedTrend = normalizeDashboardTrend(trend, safeDays);
    const values = normalizedTrend.map((item) =>
      Number(item.requests ?? item.total_tokens ?? item.tokens ?? 0),
    );
    if (!values.some((value) => value > 0)) return '';

    const width = 680;
    const height = 220;
    const left = 26;
    const right = 18;
    const top = 30;
    const bottom = 44;
    const plotWidth = width - left - right;
    const plotHeight = height - top - bottom;
    const max = Math.max(...values, 1);
    const points = values.map((value, index) => {
      const x =
        left + (values.length === 1 ? plotWidth / 2 : (plotWidth * index) / (values.length - 1));
      const y = top + plotHeight - (value / max) * plotHeight;
      return { x, y, value, item: normalizedTrend[index] };
    });
    const linePath = buildSmoothPath(points);
    const baseline = top + plotHeight;
    const areaPath = `${linePath} L ${left + plotWidth} ${baseline} L ${left} ${baseline} Z`;
    const labels = points
      .map((point, index) => {
        const visible =
          safeDays === 7 ||
          index === 0 ||
          index === points.length - 1 ||
          index % 5 === 0;
        if (!visible) return '';
        const label =
          safeDays === 7
            ? dashboardWeekday(point.item.date)
            : String(point.item.date || '').slice(5);
        return `<text x="${point.x}" y="208" text-anchor="middle">${escapeHtml(label)}</text>`;
      })
      .join('');
    const peak = points.reduce((current, point) =>
      point.value > current.value ? point : current,
    );
    const tooltipLeft = Math.min(90, Math.max(10, (peak.x / width) * 100));
    const tooltipTop = Math.min(86, Math.max(20, (peak.y / height) * 100));
    const peakLabel = `${dashboardWeekday(peak.item.date)} · ${formatNumber(
      peak.item.requests ?? peak.value,
    )} 次请求`;

    return `
      <div class="s2-chart-wrap">
        <svg class="s2-trend-chart" data-dashboard-range="${safeDays}" data-dashboard-start-date="${normalizedTrend[0].date}" data-dashboard-end-date="${normalizedTrend[normalizedTrend.length - 1].date}" viewBox="0 0 ${width} ${height}" role="img" aria-label="近 ${safeDays} 天调用活跃度趋势">
          <defs>
            <linearGradient id="${PAGE_ROOT_ID}-area" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0" stop-color="currentColor" stop-opacity=".34"></stop>
              <stop offset="1" stop-color="currentColor" stop-opacity="0"></stop>
            </linearGradient>
          </defs>
          <g class="s2-chart-grid">
            <line x1="${left}" y1="${top}" x2="${left + plotWidth}" y2="${top}"></line>
            <line x1="${left}" y1="${top + plotHeight / 3}" x2="${left + plotWidth}" y2="${top + plotHeight / 3}"></line>
            <line x1="${left}" y1="${top + (plotHeight * 2) / 3}" x2="${left + plotWidth}" y2="${top + (plotHeight * 2) / 3}"></line>
            <line x1="${left}" y1="${baseline}" x2="${left + plotWidth}" y2="${baseline}"></line>
          </g>
          <path class="s2-chart-area" d="${areaPath}"></path>
          <path class="s2-chart-line" d="${linePath}"></path>
          <circle class="s2-chart-peak" cx="${peak.x}" cy="${peak.y}" r="5"></circle>
          <g class="s2-chart-labels">${labels}</g>
        </svg>
        <div class="s2-chart-tooltip-bubble" style="--s2-tooltip-x:${tooltipLeft}%;--s2-tooltip-y:${tooltipTop}%">${escapeHtml(peakLabel)}</div>
      </div>`;
  }

  function normalizeModelRows(models) {
    const rows = models
      .map((item) => ({
        name: item.model || item.name || item.requested_model || '未知模型',
        value: Number(
          item.requests ?? item.request_count ?? item.total_requests ?? item.total_tokens ?? 0,
        ),
      }))
      .filter((item) => item.value > 0)
      .sort((a, b) => b.value - a.value)
      .slice(0, 4);
    const total = rows.reduce((sum, item) => sum + item.value, 0) || 1;
    return rows.map((item) => ({
      ...item,
      percent: Math.max(1, Math.round((item.value / total) * 100)),
    }));
  }

  function buildModelDonutHtml(models) {
    const rows = normalizeModelRows(Array.isArray(models) ? models : []);
    const primaryPercent = rows[0]?.percent || 0;
    const circumference = 2 * Math.PI * 44;
    const primaryLength = (circumference * primaryPercent) / 100;
    const legends = rows
      .slice(0, 3)
      .map(
        (item) => `
          <div class="s2-donut-legend" data-dashboard-model-legend title="${escapeHtml(item.name)}">
            <span class="s2-donut-swatch"></span>
            <span class="s2-donut-label">${escapeHtml(item.name)} · ${item.percent}%</span>
          </div>`,
      )
      .join('');
    const otherPercent = Math.max(
      0,
      100 - rows.slice(0, 3).reduce((sum, item) => sum + item.percent, 0),
    );
    return `
      <div class="s2-donut-wrap ${rows.length ? '' : 'is-empty'}">
        <svg class="s2-donut" data-dashboard-donut viewBox="0 0 120 120" role="img" aria-label="模型请求占比圆环图">
          <title>模型请求占比</title>
          <circle class="s2-donut-base" cx="60" cy="60" r="44"></circle>
          <circle class="s2-donut-segment" cx="60" cy="60" r="44" stroke-dasharray="${primaryLength} ${circumference - primaryLength}"></circle>
          <circle class="s2-donut-center" cx="60" cy="60" r="29"></circle>
        </svg>
        <div class="s2-donut-copy">
          ${legends || '<div class="s2-donut-empty">暂无模型调用</div>'}
          <span class="s2-donut-other">其余模型占 ${otherPercent}%</span>
        </div>
      </div>`;
  }

  function maskApiKey(key) {
    const value = String(key || '');
    if (!value) return '尚未创建 API Key';
    if (value.length <= 10) return `${value.slice(0, 3)}••••`;
    return `${value.slice(0, 7)}••••••••${value.slice(-4)}`;
  }

  function buildCcSwitchImportUrl(apiKey, settings, clientType) {
    const baseUrl = String(
      settings?.api_base_url || window.location.origin,
    ).replace(/\/+$/, '');
    const usageScript = `({
      request: {
        url: "{{baseUrl}}/v1/usage",
        method: "GET",
        headers: { "Authorization": "Bearer {{apiKey}}" }
      },
      extractor: function(response) {
        const remaining = response?.remaining ?? response?.quota?.remaining ?? response?.balance;
        const unit = response?.unit ?? response?.quota?.unit ?? "USD";
        return {
          isValid: response?.is_active ?? response?.isValid ?? true,
          remaining: remaining,
          unit: unit
        };
      }
    })`;
    return buildCcSwitchImportDeeplink({
      baseUrl,
      platform: apiKey?.group?.platform || 'anthropic',
      clientType,
      providerName: String(settings?.site_name || 'sub2api').trim() || 'sub2api',
      apiKey: String(apiKey?.key || ''),
      usageScript,
    });
  }
  function injectPageStyles() {
    if (pageStyle) return;
    const style = document.createElement('style');
    style.id = PAGE_STYLE_ID;
    style.textContent = `
      #${PAGE_ROOT_ID} {
        --s2-bg: #f7fafb;
        --s2-card: rgba(255,255,255,.94);
        --s2-card-solid: #ffffff;
        --s2-text: #172033;
        --s2-muted: #6b7280;
        --s2-border: rgba(148,163,184,.22);
        --s2-primary: #0f9f91;
        --s2-primary-strong: #087f75;
        --s2-primary-soft: rgba(15,159,145,.10);
        --s2-violet: #7c5cff;
        --s2-amber: #f59e0b;
        --s2-danger: #ef476f;
        position: relative;
        color: var(--s2-text);
        font-family: inherit;
      }
      .dark #${PAGE_ROOT_ID} {
        --s2-bg: #090e17;
        --s2-card: rgba(19,27,40,.94);
        --s2-card-solid: #131b28;
        --s2-text: #eef2f7;
        --s2-muted: #9aa6b6;
        --s2-border: rgba(148,163,184,.17);
        --s2-primary: #34d6c5;
        --s2-primary-strong: #65e4d7;
        --s2-primary-soft: rgba(52,214,197,.11);
        --s2-violet: #a78bfa;
        --s2-amber: #fbbf24;
      }
      #${PAGE_ROOT_ID}, #${PAGE_ROOT_ID} * { box-sizing: border-box; }
      #${PAGE_ROOT_ID} button, #${PAGE_ROOT_ID} select { font: inherit; }
      #${PAGE_ROOT_ID} button { cursor: pointer; }
      #${PAGE_ROOT_ID} .s2-icon { display: block; flex: 0 0 auto; }
      #${PAGE_ROOT_ID} .s2-page {
        min-height: calc(100vh - 9rem);
        border-radius: 1.1rem;
        background:
          radial-gradient(circle at 55% 0%, rgba(38,198,184,.13), transparent 34%),
          transparent;
      }
      #${PAGE_ROOT_ID} .s2-card {
        border: 1px solid var(--s2-border);
        border-radius: 1rem;
        background: var(--s2-card);
        box-shadow: 0 14px 40px rgba(15,23,42,.055);
        backdrop-filter: blur(14px);
      }
      #${PAGE_ROOT_ID} .s2-hero {
        position: relative;
        display: grid;
        grid-template-columns: minmax(0,1.3fr) minmax(15rem,.7fr);
        gap: 1.5rem;
        overflow: hidden;
        padding: 1.5rem;
        margin-bottom: .9rem;
        background:
          radial-gradient(circle at 88% 4%, rgba(124,92,255,.22), transparent 42%),
          linear-gradient(135deg, var(--s2-primary-soft), var(--s2-card));
      }
      #${PAGE_ROOT_ID} .s2-eyebrow { display:flex; align-items:center; gap:.45rem; color:var(--s2-primary-strong); font-size:.78rem; font-weight:600; }
      #${PAGE_ROOT_ID} .s2-hero h1 { margin:.55rem 0 .35rem; color:var(--s2-text); font-size:clamp(1.45rem,2.7vw,2rem); line-height:1.15; letter-spacing:-.035em; }
      #${PAGE_ROOT_ID} .s2-hero p { max-width:38rem; margin:0; color:var(--s2-muted); line-height:1.65; }
      #${PAGE_ROOT_ID} .s2-hero-actions { display:flex; gap:.55rem; margin-top:1rem; flex-wrap:wrap; }
      #${PAGE_ROOT_ID} .s2-btn {
        display:inline-flex; align-items:center; justify-content:center; gap:.42rem;
        min-height:2.35rem; padding:.52rem .8rem; border:1px solid var(--s2-border);
        border-radius:.7rem; color:var(--s2-text); background:var(--s2-card-solid);
        font-size:.82rem; font-weight:600; transition:transform .16s, border-color .16s, background .16s;
      }
      #${PAGE_ROOT_ID} .s2-btn:hover { transform:translateY(-1px); border-color:rgba(15,159,145,.45); }
      #${PAGE_ROOT_ID} .s2-btn-primary { border-color:transparent; color:#fff; background:linear-gradient(135deg,var(--s2-primary),var(--s2-primary-strong)); box-shadow:0 10px 24px rgba(15,159,145,.19); }
      #${PAGE_ROOT_ID} .s2-btn-ghost { border-color:transparent; background:transparent; color:var(--s2-muted); }
      #${PAGE_ROOT_ID} .s2-asset-card { align-self:center; padding:1rem; border:1px solid rgba(15,159,145,.22); border-radius:.9rem; background:rgba(255,255,255,.58); box-shadow:0 16px 38px rgba(15,159,145,.10); }
      .dark #${PAGE_ROOT_ID} .s2-asset-card { background:rgba(19,27,40,.6); }
      #${PAGE_ROOT_ID} .s2-label { color:var(--s2-muted); font-size:.75rem; }
      #${PAGE_ROOT_ID} .s2-asset-value { margin:.38rem 0 .2rem; font-size:1.7rem; font-weight:700; letter-spacing:-.035em; }
      #${PAGE_ROOT_ID} .s2-badge { display:inline-flex; align-items:center; gap:.3rem; padding:.25rem .5rem; border-radius:999px; color:var(--s2-primary-strong); background:var(--s2-primary-soft); font-size:.7rem; font-weight:600; }
      #${PAGE_ROOT_ID} .s2-metrics { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:.7rem; margin-bottom:.9rem; }
      #${PAGE_ROOT_ID} .s2-metric { padding:1rem; }
      #${PAGE_ROOT_ID} .s2-metric-top { display:flex; align-items:center; justify-content:space-between; gap:.5rem; }
      #${PAGE_ROOT_ID} .s2-icon-box { display:grid; place-items:center; width:2rem; height:2rem; border-radius:.65rem; color:var(--s2-primary-strong); background:var(--s2-primary-soft); }
      #${PAGE_ROOT_ID} .s2-metric-value { margin:.55rem 0 .15rem; color:var(--s2-text); font-size:1.45rem; font-weight:700; letter-spacing:-.03em; }
      #${PAGE_ROOT_ID} .s2-metric-note { color:var(--s2-muted); font-size:.72rem; }
      #${PAGE_ROOT_ID} .s2-dashboard-grid { display:grid; grid-template-columns:minmax(0,1.4fr) minmax(16rem,.6fr); gap:.8rem; }
      #${PAGE_ROOT_ID} .s2-panel { min-width:0; padding:1rem; }
      #${PAGE_ROOT_ID} .s2-panel-head { display:flex; align-items:flex-start; justify-content:space-between; gap:.8rem; margin-bottom:.8rem; }
      #${PAGE_ROOT_ID} .s2-panel-title { color:var(--s2-text); font-size:.92rem; font-weight:700; }
      #${PAGE_ROOT_ID} .s2-panel-note { margin-top:.16rem; color:var(--s2-muted); font-size:.72rem; }
      #${PAGE_ROOT_ID} .s2-trend-chart { display:block; width:100%; height:auto; min-height:13rem; color:var(--s2-primary); }
      #${PAGE_ROOT_ID} .s2-chart-grid line { stroke:var(--s2-border); stroke-width:1; }
      #${PAGE_ROOT_ID} .s2-chart-area { fill:url(#${PAGE_ROOT_ID}-area); }
      #${PAGE_ROOT_ID} .s2-chart-line { fill:none; stroke:currentColor; stroke-width:3; stroke-linecap:round; stroke-linejoin:round; }
      #${PAGE_ROOT_ID} .s2-chart-peak { fill:var(--s2-card-solid); stroke:currentColor; stroke-width:3; }
      #${PAGE_ROOT_ID} .s2-chart-labels { fill:var(--s2-muted); font-size:10px; }
      #${PAGE_ROOT_ID} .s2-model-list { display:grid; gap:.8rem; }
      #${PAGE_ROOT_ID} .s2-model-row { display:grid; grid-template-columns:minmax(0,1fr) auto; gap:.35rem .7rem; align-items:center; }
      #${PAGE_ROOT_ID} .s2-model-name { display:flex; align-items:center; gap:.45rem; min-width:0; font-size:.8rem; }
      #${PAGE_ROOT_ID} .s2-model-dot { width:.48rem; height:.48rem; border-radius:50%; background:var(--s2-primary); }
      #${PAGE_ROOT_ID} .s2-model-percent { font-size:.75rem; font-weight:700; }
      #${PAGE_ROOT_ID} .s2-progress { grid-column:1/-1; height:.34rem; overflow:hidden; border-radius:999px; background:rgba(148,163,184,.15); }
      #${PAGE_ROOT_ID} .s2-progress span { display:block; height:100%; border-radius:inherit; background:linear-gradient(90deg,var(--s2-primary),var(--s2-violet)); }
      #${PAGE_ROOT_ID} .s2-dashboard-page {
        --s2-dashboard-blue:#339cff;
        --s2-dashboard-orange:#ff7a2f;
        --s2-dashboard-green:#5dc977;
        --s2-dashboard-surface:#f4f4f5;
        display:grid;
        width:100%;
        max-width:1440px;
        min-height:0;
        margin-inline:auto;
        align-content:start;
        gap:12px;
        border-radius:0;
        color:#1a1c1f;
        background:transparent;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-hero {
        position:relative;
        display:grid;
        min-height:184px;
        grid-template-columns:minmax(0,1.3fr) minmax(190px,.7fr);
        align-items:center;
        gap:20px;
        overflow:hidden;
        padding:22px;
        border:0;
        border-radius:16px;
        background:linear-gradient(112deg,#dcebfa 0%,#eef2f6 52%,#d7efdf 100%);
      }
      #${PAGE_ROOT_ID} .s2-dashboard-eyebrow {
        display:flex;
        align-items:center;
        gap:6px;
        color:var(--s2-dashboard-blue);
        font-size:14px;
        font-weight:500;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-hero h1 {
        margin:9px 0 4px;
        color:#1a1c1f;
        font-size:25px;
        font-weight:500;
        line-height:1.25;
        letter-spacing:0;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-hero p {
        max-width:480px;
        margin:0;
        color:#7c8189;
        font-size:14px;
        line-height:1.5;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-actions {
        display:flex;
        flex-wrap:wrap;
        gap:8px;
        margin-top:15px;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-page .s2-btn {
        min-height:28px;
        padding:4px 9px;
        border-color:rgba(26,28,31,.12);
        border-radius:8px;
        color:#1a1c1f;
        background:rgba(255,255,255,.96);
        box-shadow:none;
        font-size:13px;
        font-weight:500;
        transition:border-color .18s,background-color .18s,color .18s;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-page .s2-btn:hover {
        transform:none;
        border-color:rgba(51,156,255,.42);
        background:#fff;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-page .s2-btn:focus-visible {
        outline:2px solid rgba(51,156,255,.7);
        outline-offset:2px;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-page .s2-btn-primary {
        border-color:#1a1c1f;
        color:#fff;
        background:#1a1c1f;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-page .s2-btn-primary:hover {
        border-color:#30343a;
        background:#30343a;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-balance {
        align-self:center;
        justify-self:end;
        width:100%;
        min-width:190px;
        max-width:220px;
        padding:16px;
        border:1px solid rgba(51,156,255,.34);
        border-radius:14px;
        background:rgba(235,245,240,.56);
      }
      #${PAGE_ROOT_ID} .s2-dashboard-balance-label,
      #${PAGE_ROOT_ID} .s2-dashboard-metric-label {
        color:#81858b;
        font-size:14px;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-balance-value {
        margin:7px 0 3px;
        color:#1a1c1f;
        font-size:27px;
        font-weight:500;
        line-height:1.15;
        letter-spacing:0;
        white-space:nowrap;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-status {
        display:inline-flex;
        align-items:center;
        width:max-content;
        min-height:22px;
        padding:3px 8px;
        border-radius:999px;
        color:var(--s2-dashboard-blue);
        background:rgba(51,156,255,.12);
        font-size:12px;
        font-weight:600;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-metrics {
        display:grid;
        grid-template-columns:repeat(3,minmax(0,1fr));
        gap:10px;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-metric,
      #${PAGE_ROOT_ID} .s2-dashboard-panel {
        min-width:0;
        border:1px solid rgba(26,28,31,.055);
        background:var(--s2-dashboard-surface);
        box-shadow:none;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-metric {
        min-height:112px;
        padding:14px;
        border-radius:15px;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-metric-head {
        display:flex;
        align-items:center;
        justify-content:space-between;
        gap:8px;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-icon {
        display:grid;
        place-items:center;
        width:32px;
        height:32px;
        flex:0 0 32px;
        border-radius:10px;
        color:var(--s2-dashboard-blue);
        background:#e2f0ff;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-metric-value {
        margin:7px 0 1px;
        color:#1a1c1f;
        font-size:24px;
        font-weight:500;
        line-height:1.22;
        letter-spacing:0;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-metric-note {
        min-height:18px;
        overflow:hidden;
        color:#898d93;
        font-size:12px;
        line-height:1.5;
        text-overflow:ellipsis;
        white-space:nowrap;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-metric-accent { color:var(--s2-dashboard-blue); }
      #${PAGE_ROOT_ID} .s2-dashboard-analysis {
        display:grid;
        grid-template-columns:minmax(0,1.35fr) minmax(250px,.65fr);
        gap:12px;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-panel {
        display:flex;
        height:clamp(292px,26vw,360px);
        min-height:292px;
        flex-direction:column;
        overflow:hidden;
        padding:14px;
        border-radius:16px;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-panel-head {
        display:flex;
        align-items:flex-start;
        justify-content:space-between;
        gap:10px;
        margin-bottom:8px;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-panel-title {
        color:#1a1c1f;
        font-size:14px;
        font-weight:600;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-panel-note {
        margin-top:1px;
        color:#898d93;
        font-size:12px;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-periods {
        display:flex;
        align-items:center;
        gap:6px;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-periods .s2-btn {
        min-width:48px;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-periods .s2-btn[aria-pressed="true"] {
        border-color:var(--s2-dashboard-blue);
        color:#fff;
        background:var(--s2-dashboard-blue);
      }
      #${PAGE_ROOT_ID} .s2-chart-wrap {
        position:relative;
        flex:1 1 auto;
        min-height:208px;
      }
      #${PAGE_ROOT_ID} .s2-trend-chart {
        display:block;
        width:100%;
        height:100%;
        min-height:0;
        color:var(--s2-dashboard-orange);
      }
      #${PAGE_ROOT_ID} .s2-chart-grid line {
        stroke:rgba(26,28,31,.065);
        stroke-width:1;
      }
      #${PAGE_ROOT_ID} .s2-chart-area { fill:url(#${PAGE_ROOT_ID}-area); }
      #${PAGE_ROOT_ID} .s2-chart-line {
        fill:none;
        stroke:currentColor;
        stroke-width:3;
        stroke-linecap:round;
        stroke-linejoin:round;
      }
      #${PAGE_ROOT_ID} .s2-chart-peak {
        fill:var(--s2-dashboard-surface);
        stroke:currentColor;
        stroke-width:2;
      }
      #${PAGE_ROOT_ID} .s2-chart-labels {
        fill:#989ca2;
        font-size:10px;
      }
      #${PAGE_ROOT_ID} .s2-chart-tooltip-bubble {
        position:absolute;
        left:var(--s2-tooltip-x);
        top:var(--s2-tooltip-y);
        z-index:1;
        max-width:calc(100% - 16px);
        padding:7px 10px;
        transform:translate(-50%,-125%);
        border:1px solid rgba(26,28,31,.1);
        border-radius:8px;
        color:#3e4248;
        background:rgba(246,247,248,.96);
        box-shadow:0 6px 16px rgba(26,28,31,.08);
        font-size:12px;
        font-weight:500;
        line-height:1.25;
        white-space:nowrap;
        pointer-events:none;
      }
      #${PAGE_ROOT_ID} .s2-donut-wrap {
        display:grid;
        min-height:0;
        flex:1 1 auto;
        grid-template-columns:145px minmax(0,1fr);
        align-items:center;
        gap:8px;
      }
      #${PAGE_ROOT_ID} .s2-donut {
        width:145px;
        height:145px;
        transform:rotate(-90deg);
      }
      #${PAGE_ROOT_ID} .s2-donut-base {
        fill:none;
        stroke:#dedfe2;
        stroke-width:14;
      }
      #${PAGE_ROOT_ID} .s2-donut-segment {
        fill:none;
        stroke:var(--s2-dashboard-blue);
        stroke-width:14;
        stroke-linecap:round;
      }
      #${PAGE_ROOT_ID} .s2-donut-center { fill:var(--s2-dashboard-surface); }
      #${PAGE_ROOT_ID} .s2-donut-copy { display:grid; gap:9px; min-width:0; }
      #${PAGE_ROOT_ID} .s2-donut-legend {
        display:flex;
        align-items:center;
        gap:7px;
        min-width:0;
        color:#30343a;
        font-size:13px;
      }
      #${PAGE_ROOT_ID} .s2-donut-swatch {
        width:7px;
        height:7px;
        flex:0 0 7px;
        border-radius:50%;
        background:var(--s2-dashboard-blue);
      }
      #${PAGE_ROOT_ID} .s2-donut-legend:nth-child(2) .s2-donut-swatch { background:var(--s2-dashboard-orange); }
      #${PAGE_ROOT_ID} .s2-donut-legend:nth-child(3) .s2-donut-swatch { background:var(--s2-dashboard-green); }
      #${PAGE_ROOT_ID} .s2-donut-label {
        display:-webkit-box;
        min-width:0;
        overflow:hidden;
        line-height:1.35;
        overflow-wrap:anywhere;
        white-space:normal;
        -webkit-box-orient:vertical;
        -webkit-line-clamp:2;
      }
      #${PAGE_ROOT_ID} .s2-donut-other,
      #${PAGE_ROOT_ID} .s2-donut-empty {
        color:#8b8f96;
        font-size:12px;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-empty {
        display:grid;
        min-height:224px;
        place-items:center;
        color:#898d93;
        text-align:center;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-empty h3 {
        margin:10px 0 4px;
        color:#3e4248;
        font-size:14px;
        font-weight:600;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-empty p { margin:0 0 10px; font-size:13px; }
      #${PAGE_ROOT_ID} .s2-dashboard-skeleton {
        display:block;
        height:12px;
        border-radius:999px;
        background:linear-gradient(90deg,rgba(148,163,184,.13),rgba(148,163,184,.26),rgba(148,163,184,.13));
        background-size:200% 100%;
        animation:s2-dashboard-loading 1.25s ease-in-out infinite;
      }
      #${PAGE_ROOT_ID} .s2-dashboard-skeleton.is-title { width:min(72%,28rem); height:30px; margin:12px 0 8px; }
      #${PAGE_ROOT_ID} .s2-dashboard-skeleton.is-copy { width:min(86%,32rem); }
      #${PAGE_ROOT_ID} .s2-dashboard-skeleton.is-value { width:48%; height:28px; margin:12px 0 7px; }
      #${PAGE_ROOT_ID} .s2-dashboard-skeleton.is-chart { width:100%; height:208px; border-radius:12px; }
      @keyframes s2-dashboard-loading { to { background-position:-200% 0; } }
      .dark #${PAGE_ROOT_ID} .s2-dashboard-page { --s2-dashboard-surface:#131b28; color:#eef2f7; }
      .dark #${PAGE_ROOT_ID} .s2-dashboard-hero {
        background:linear-gradient(112deg,#17283a 0%,#172231 52%,#173329 100%);
      }
      .dark #${PAGE_ROOT_ID} .s2-dashboard-hero h1,
      .dark #${PAGE_ROOT_ID} .s2-dashboard-balance-value,
      .dark #${PAGE_ROOT_ID} .s2-dashboard-metric-value,
      .dark #${PAGE_ROOT_ID} .s2-dashboard-panel-title { color:#eef2f7; }
      .dark #${PAGE_ROOT_ID} .s2-dashboard-hero p,
      .dark #${PAGE_ROOT_ID} .s2-dashboard-balance-label,
      .dark #${PAGE_ROOT_ID} .s2-dashboard-metric-label,
      .dark #${PAGE_ROOT_ID} .s2-dashboard-metric-note,
      .dark #${PAGE_ROOT_ID} .s2-dashboard-panel-note,
      .dark #${PAGE_ROOT_ID} .s2-donut-other,
      .dark #${PAGE_ROOT_ID} .s2-donut-empty { color:#9aa6b6; }
      .dark #${PAGE_ROOT_ID} .s2-dashboard-balance {
        border-color:rgba(83,177,255,.4);
        background:rgba(18,39,48,.56);
      }
      .dark #${PAGE_ROOT_ID} .s2-dashboard-status {
        color:#8dceff;
        background:rgba(51,156,255,.16);
      }
      .dark #${PAGE_ROOT_ID} .s2-dashboard-metric,
      .dark #${PAGE_ROOT_ID} .s2-dashboard-panel {
        border-color:rgba(148,163,184,.1);
        background:var(--s2-dashboard-surface);
      }
      .dark #${PAGE_ROOT_ID} .s2-dashboard-icon { background:#142d45; }
      .dark #${PAGE_ROOT_ID} .s2-dashboard-page .s2-btn {
        border-color:rgba(148,163,184,.22);
        color:#eef2f7;
        background:#1b2736;
      }
      .dark #${PAGE_ROOT_ID} .s2-dashboard-page .s2-btn-primary {
        border-color:#eef2f7;
        color:#111827;
        background:#eef2f7;
      }
      .dark #${PAGE_ROOT_ID} .s2-chart-grid line { stroke:rgba(148,163,184,.12); }
      .dark #${PAGE_ROOT_ID} .s2-chart-labels { fill:#8290a2; }
      .dark #${PAGE_ROOT_ID} .s2-chart-tooltip-bubble {
        border-color:rgba(148,163,184,.2);
        color:#dbe5ef;
        background:rgba(29,41,56,.96);
        box-shadow:0 8px 20px rgba(0,0,0,.22);
      }
      .dark #${PAGE_ROOT_ID} .s2-donut-legend { fill:#dbe5ef; color:#dbe5ef; }
      .dark #${PAGE_ROOT_ID} .s2-dashboard-empty h3 { color:#dbe5ef; }
      .dark #${PAGE_ROOT_ID} .s2-donut-base { stroke:#293342; }
      @media (max-width:620px) {
        #${PAGE_ROOT_ID} .s2-dashboard-hero,
        #${PAGE_ROOT_ID} .s2-dashboard-metrics,
        #${PAGE_ROOT_ID} .s2-dashboard-analysis { grid-template-columns:1fr; }
        #${PAGE_ROOT_ID} .s2-dashboard-hero { padding:18px; }
        #${PAGE_ROOT_ID} .s2-dashboard-balance { justify-self:stretch; max-width:none; }
        #${PAGE_ROOT_ID} .s2-dashboard-actions .s2-btn { flex:1 1 10rem; }
        #${PAGE_ROOT_ID} .s2-dashboard-panel { height:auto; min-height:280px; }
        #${PAGE_ROOT_ID} .s2-chart-wrap { min-height:220px; }
        #${PAGE_ROOT_ID} .s2-dashboard-model-panel .s2-donut-wrap { min-height:224px; }
      }
      @media (max-width:420px) {
        #${PAGE_ROOT_ID} .s2-donut-wrap { grid-template-columns:1fr; align-content:center; justify-items:center; }
        #${PAGE_ROOT_ID} .s2-donut-copy { width:100%; }
      }
      @media (prefers-reduced-motion:reduce) {
        #${PAGE_ROOT_ID} .s2-dashboard-page *,
        #${PAGE_ROOT_ID} .s2-dashboard-page *::before,
        #${PAGE_ROOT_ID} .s2-dashboard-page *::after { animation:none !important; scroll-behavior:auto !important; transition:none !important; }
      }
      #${PAGE_ROOT_ID} .s2-empty { display:grid; place-items:center; min-height:15rem; padding:1.5rem; text-align:center; }
      #${PAGE_ROOT_ID} .s2-empty h3 { margin:.65rem 0 .3rem; font-size:1rem; }
      #${PAGE_ROOT_ID} .s2-empty p { max-width:28rem; margin:0 auto .9rem; color:var(--s2-muted); font-size:.8rem; line-height:1.55; }
      #${PAGE_ROOT_ID} .s2-guide-breadcrumb { display:flex; align-items:center; gap:.45rem; margin:0 0 .65rem; color:var(--s2-muted); font-size:.74rem; }
      #${PAGE_ROOT_ID} .s2-guide-layout { display:grid; grid-template-columns:13rem minmax(0,1fr); gap:.85rem; align-items:start; }
      #${PAGE_ROOT_ID} .s2-steps { padding:.85rem; }
      #${PAGE_ROOT_ID} .s2-steps-title { margin:.15rem .4rem .65rem; font-size:.82rem; font-weight:700; }
      #${PAGE_ROOT_ID} .s2-step { display:grid; grid-template-columns:1.65rem minmax(0,1fr); gap:.55rem; align-items:start; width:100%; padding:.65rem; border:0; border-radius:.7rem; color:var(--s2-text); background:transparent; text-align:left; }
      #${PAGE_ROOT_ID} .s2-step:hover { background:rgba(148,163,184,.09); }
      #${PAGE_ROOT_ID} .s2-step.is-active { color:var(--s2-primary-strong); background:var(--s2-primary-soft); }
      #${PAGE_ROOT_ID} .s2-step-number { display:grid; place-items:center; width:1.6rem; height:1.6rem; border-radius:50%; color:var(--s2-muted); background:rgba(148,163,184,.15); font-size:.72rem; font-weight:700; }
      #${PAGE_ROOT_ID} .s2-step.is-active .s2-step-number { color:#fff; background:var(--s2-primary); }
      #${PAGE_ROOT_ID} .s2-step strong { display:block; font-size:.78rem; }
      #${PAGE_ROOT_ID} .s2-step small { display:block; margin-top:.12rem; color:var(--s2-muted); font-size:.68rem; }
      #${PAGE_ROOT_ID} .s2-guide-panel { min-width:0; padding:1rem; }
      #${PAGE_ROOT_ID} .s2-section-head { display:flex; align-items:flex-start; justify-content:space-between; gap:.8rem; margin-bottom:.9rem; }
      #${PAGE_ROOT_ID} .s2-section-head h2 { margin:0; font-size:1.05rem; }
      #${PAGE_ROOT_ID} .s2-section-head p { margin:.22rem 0 0; color:var(--s2-muted); font-size:.76rem; }
      #${PAGE_ROOT_ID} .s2-fields { display:grid; gap:.65rem; }
      #${PAGE_ROOT_ID} .s2-field-label { display:block; margin-bottom:.32rem; color:var(--s2-muted); font-size:.72rem; }
      #${PAGE_ROOT_ID} .s2-copy-field { display:flex; align-items:center; gap:.55rem; min-width:0; padding:.65rem; border:1px solid var(--s2-border); border-radius:.7rem; background:rgba(148,163,184,.07); }
      #${PAGE_ROOT_ID} .s2-copy-field code { min-width:0; flex:1; overflow-wrap:anywhere; color:var(--s2-text); font-size:.76rem; }
      #${PAGE_ROOT_ID} .s2-select { width:100%; min-height:2.5rem; padding:.55rem .7rem; border:1px solid var(--s2-border); border-radius:.7rem; color:var(--s2-text); background:var(--s2-card-solid); }
      #${PAGE_ROOT_ID} .s2-note { display:flex; gap:.5rem; align-items:flex-start; padding:.7rem; margin-top:.7rem; border-radius:.7rem; color:var(--s2-muted); background:rgba(245,158,11,.08); font-size:.72rem; line-height:1.5; }
      #${PAGE_ROOT_ID} .s2-mode-tabs, #${PAGE_ROOT_ID} .s2-protocol-tabs { display:flex; gap:.3rem; padding:.28rem; margin-bottom:.75rem; border:1px solid var(--s2-border); border-radius:.75rem; background:rgba(148,163,184,.09); }
      #${PAGE_ROOT_ID} .s2-mode-tabs button, #${PAGE_ROOT_ID} .s2-protocol-tabs button { flex:1; min-height:2.25rem; border:0; border-radius:.55rem; color:var(--s2-muted); background:transparent; font-size:.76rem; font-weight:600; }
      #${PAGE_ROOT_ID} .s2-mode-tabs button.is-active, #${PAGE_ROOT_ID} .s2-protocol-tabs button.is-active { color:#fff; background:var(--s2-primary); }
      #${PAGE_ROOT_ID} .s2-app-grid { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:.5rem; margin-bottom:.7rem; }
      #${PAGE_ROOT_ID} .s2-app { display:flex; align-items:center; gap:.5rem; min-width:0; padding:.65rem; border:1px solid var(--s2-border); border-radius:.7rem; color:var(--s2-text); background:var(--s2-card-solid); text-align:left; }
      #${PAGE_ROOT_ID} .s2-app.is-active { border-color:var(--s2-primary); background:var(--s2-primary-soft); box-shadow:0 0 0 2px rgba(15,159,145,.1); }
      #${PAGE_ROOT_ID} .s2-app-copy { min-width:0; }
      #${PAGE_ROOT_ID} .s2-app-copy strong, #${PAGE_ROOT_ID} .s2-app-copy small { display:block; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
      #${PAGE_ROOT_ID} .s2-app-copy strong { font-size:.75rem; }
      #${PAGE_ROOT_ID} .s2-app-copy small { margin-top:.1rem; color:var(--s2-muted); font-size:.65rem; }
      #${PAGE_ROOT_ID} .s2-app-detail { padding:.8rem; border:1px solid var(--s2-border); border-radius:.8rem; background:linear-gradient(135deg,var(--s2-primary-soft),var(--s2-card)); }
      #${PAGE_ROOT_ID} .s2-app-detail-head { display:flex; align-items:flex-start; justify-content:space-between; gap:.6rem; margin-bottom:.65rem; }
      #${PAGE_ROOT_ID} .s2-app-detail h3 { margin:0; font-size:.88rem; }
      #${PAGE_ROOT_ID} .s2-app-detail p { margin:.18rem 0 0; color:var(--s2-muted); font-size:.7rem; }
      #${PAGE_ROOT_ID} .s2-mapping { display:flex; align-items:center; justify-content:space-between; gap:.6rem; padding:.52rem .6rem; margin-top:.38rem; border-radius:.55rem; background:rgba(148,163,184,.09); font-size:.7rem; }
      #${PAGE_ROOT_ID} .s2-mapping span { color:var(--s2-muted); }
      #${PAGE_ROOT_ID} .s2-mapping strong { overflow-wrap:anywhere; text-align:right; }
      #${PAGE_ROOT_ID} .s2-panel-actions { display:flex; align-items:center; justify-content:flex-end; gap:.5rem; margin-top:.8rem; flex-wrap:wrap; }
      #${PAGE_ROOT_ID} .s2-code { margin:0; padding:.8rem; overflow:auto; overflow-wrap:anywhere; word-break:break-word; border:1px solid var(--s2-border); border-radius:.7rem; color:var(--s2-text); background:rgba(15,23,42,.05); font-size:.72rem; line-height:1.55; white-space:pre-wrap; }
      .dark #${PAGE_ROOT_ID} .s2-code { background:rgba(2,6,23,.35); }
      #${PAGE_ROOT_ID} .s2-model-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:.55rem; }
      #${PAGE_ROOT_ID} .s2-model-option { display:flex; align-items:center; justify-content:space-between; gap:.6rem; padding:.75rem; border:1px solid var(--s2-border); border-radius:.7rem; color:var(--s2-text); background:var(--s2-card-solid); text-align:left; }
      #${PAGE_ROOT_ID} .s2-model-option strong, #${PAGE_ROOT_ID} .s2-model-option small { display:block; }
      #${PAGE_ROOT_ID} .s2-model-option small { margin-top:.15rem; color:var(--s2-muted); font-size:.68rem; }
      #${PAGE_ROOT_ID} .s2-success { display:grid; place-items:center; min-height:20rem; padding:1.5rem; text-align:center; }
      #${PAGE_ROOT_ID} .s2-success-mark { display:grid; place-items:center; width:3.2rem; height:3.2rem; margin:0 auto .75rem; border-radius:50%; color:#fff; background:var(--s2-primary); box-shadow:0 12px 28px rgba(15,159,145,.22); }
      #${PAGE_ROOT_ID} .s2-success h2 { margin:0 0 .35rem; font-size:1.15rem; }
      #${PAGE_ROOT_ID} .s2-success p { max-width:28rem; margin:0 auto; color:var(--s2-muted); font-size:.78rem; line-height:1.6; }
      #${PAGE_ROOT_ID} .s2-toast { position:fixed; right:1.2rem; bottom:1.2rem; z-index:80; padding:.65rem .8rem; border:1px solid var(--s2-border); border-radius:.7rem; color:var(--s2-text); background:var(--s2-card-solid); box-shadow:0 14px 38px rgba(15,23,42,.18); font-size:.75rem; }
      #${PAGE_ROOT_ID} .s2-guide-page {
        max-width: 74rem;
        margin: 0 auto;
        padding-bottom: 1.5rem;
        background:
          radial-gradient(circle at 42% 3%, rgba(56,189,248,.13), transparent 28rem),
          radial-gradient(circle at 96% 22%, rgba(99,102,241,.08), transparent 24rem);
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-guide-breadcrumb {
        margin-bottom: 1.1rem;
        font-size: .82rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-hero {
        min-height: 20rem;
        grid-template-columns: minmax(0,1.35fr) minmax(20rem,.65fr);
        align-items: center;
        gap: 2.6rem;
        padding: 2.2rem 2.35rem;
        margin-bottom: 1.25rem;
        border-color: rgba(96,165,250,.22);
        border-radius: 1.5rem;
        background:
          radial-gradient(circle at 92% 2%, rgba(167,139,250,.25), transparent 42%),
          radial-gradient(circle at 5% 105%, rgba(96,165,250,.20), transparent 46%),
          linear-gradient(118deg, rgba(230,244,255,.98), rgba(250,248,246,.98) 55%, rgba(225,248,233,.96));
        box-shadow: 0 22px 55px rgba(30,64,175,.08);
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-eyebrow {
        color: #2997ff;
        font-size: .95rem;
        font-weight: 700;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-hero h1 {
        max-width: 35rem;
        margin: .9rem 0 .65rem;
        font-size: clamp(2.15rem,4vw,3rem);
        line-height: 1.12;
        letter-spacing: -.045em;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-hero p {
        max-width: 38rem;
        color: #64748b;
        font-size: 1.05rem;
        line-height: 1.72;
      }
      #${PAGE_ROOT_ID} .s2-guide-trust {
        display: flex;
        align-items: center;
        gap: 1.25rem;
        margin-top: 1.35rem;
        flex-wrap: wrap;
      }
      #${PAGE_ROOT_ID} .s2-guide-trust-item {
        display: inline-flex;
        align-items: center;
        gap: .45rem;
        color: #334155;
        font-size: .85rem;
        white-space: nowrap;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-asset-card {
        min-height: 10.5rem;
        display: flex;
        flex-direction: column;
        justify-content: center;
        padding: 1.55rem 1.7rem;
        border-color: rgba(96,165,250,.34);
        border-radius: 1.15rem;
        background: rgba(255,255,255,.48);
        box-shadow: 0 20px 46px rgba(59,130,246,.10);
      }
      #${PAGE_ROOT_ID} .s2-guide-status-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: .8rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-asset-value {
        margin: .7rem 0 .2rem;
        font-size: 1.85rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-badge {
        color: #1685ee;
        background: rgba(232,245,255,.92);
        font-size: .78rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-status-note {
        color: #7c8797;
        font-size: .8rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-guide-layout {
        grid-template-columns: 16.5rem minmax(0,1fr);
        gap: 1.25rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-steps,
      #${PAGE_ROOT_ID} .s2-guide-page .s2-guide-panel {
        border: 0;
        border-radius: 1.4rem;
        background: rgba(247,247,249,.96);
        box-shadow: none;
        backdrop-filter: none;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-steps {
        padding: 1.25rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-steps-title {
        margin: 0 0 1rem;
        font-size: 1rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-step {
        min-height: 5.35rem;
        grid-template-columns: 2.35rem minmax(0,1fr);
        gap: .75rem;
        align-items: center;
        padding: .85rem .9rem;
        border-radius: .9rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-step.is-active {
        color: #1685ee;
        background: rgba(224,239,255,.94);
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-step-number {
        width: 2.25rem;
        height: 2.25rem;
        font-size: .9rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-step.is-active .s2-step-number {
        background: #2997ff;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-step strong {
        font-size: .95rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-step small {
        margin-top: .22rem;
        font-size: .78rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-support {
        margin-top: 1.15rem;
        padding: 1.05rem;
        border-radius: .95rem;
        color: #64748b;
        background: #e9e9ec;
      }
      #${PAGE_ROOT_ID} .s2-guide-support strong {
        display: block;
        margin-bottom: .35rem;
        color: #475569;
        font-size: .82rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-support p {
        margin: 0;
        font-size: .78rem;
        line-height: 1.65;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-guide-panel {
        min-height: 35rem;
        padding: 1.6rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-section-head {
        margin-bottom: 1.3rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-section-head h2 {
        font-size: 1.55rem;
        letter-spacing: -.025em;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-section-head p {
        margin-top: .35rem;
        font-size: .9rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-fields {
        gap: 1rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-field-label {
        margin-bottom: .48rem;
        font-size: .82rem;
      }
      #${PAGE_ROOT_ID} .s2-key-select {
        position: relative;
        z-index: 12;
      }
      #${PAGE_ROOT_ID} .s2-key-select-trigger {
        display: grid;
        grid-template-columns: 2.55rem minmax(0,1fr) auto;
        align-items: center;
        gap: .85rem;
        width: 100%;
        min-height: 4.75rem;
        padding: .7rem .85rem;
        border: 1px solid rgba(148,163,184,.30);
        border-radius: 1rem;
        color: var(--s2-text);
        background: linear-gradient(180deg,rgba(255,255,255,.94),rgba(244,247,250,.94));
        box-shadow: 0 8px 20px rgba(15,23,42,.06), inset 0 1px 0 rgba(255,255,255,.9);
        text-align: left;
        transition: border-color .18s, box-shadow .18s, background .18s;
      }
      #${PAGE_ROOT_ID} .s2-key-select-trigger:hover,
      #${PAGE_ROOT_ID} .s2-key-select.is-open .s2-key-select-trigger {
        border-color: rgba(41,151,255,.58);
        background: #fff;
        box-shadow: 0 10px 26px rgba(41,151,255,.10), 0 0 0 3px rgba(41,151,255,.08);
      }
      #${PAGE_ROOT_ID} .s2-key-select-trigger:focus-visible {
        outline: 3px solid rgba(41,151,255,.25);
        outline-offset: 2px;
      }
      #${PAGE_ROOT_ID} .s2-key-select-icon {
        display: grid;
        place-items: center;
        width: 2.55rem;
        height: 2.55rem;
        border-radius: .8rem;
        color: #1685ee;
        background: linear-gradient(145deg,#e2f1ff,#eef8ff);
      }
      #${PAGE_ROOT_ID} .s2-key-select-copy {
        min-width: 0;
      }
      #${PAGE_ROOT_ID} .s2-key-select-copy strong,
      #${PAGE_ROOT_ID} .s2-key-select-copy small {
        display: block;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }
      #${PAGE_ROOT_ID} .s2-key-select-copy strong {
        font-size: .93rem;
      }
      #${PAGE_ROOT_ID} .s2-key-select-copy small {
        margin-top: .22rem;
        color: var(--s2-muted);
        font-size: .76rem;
      }
      #${PAGE_ROOT_ID} .s2-key-select-chevron {
        color: #7c8797;
        transition: transform .18s;
      }
      #${PAGE_ROOT_ID} .s2-key-select.is-open .s2-key-select-chevron {
        transform: rotate(180deg);
      }
      #${PAGE_ROOT_ID} .s2-key-select-menu {
        position: absolute;
        top: calc(100% + .55rem);
        left: 0;
        right: 0;
        z-index: 30;
        display: grid;
        gap: .35rem;
        max-height: 18rem;
        padding: .55rem;
        overflow: auto;
        border: 1px solid rgba(148,163,184,.25);
        border-radius: 1rem;
        background: rgba(255,255,255,.98);
        box-shadow: 0 22px 54px rgba(15,23,42,.18), 0 4px 14px rgba(15,23,42,.08);
        backdrop-filter: blur(18px);
      }
      #${PAGE_ROOT_ID} .s2-key-select-option {
        display: grid;
        grid-template-columns: 2.25rem minmax(0,1fr) auto;
        align-items: center;
        gap: .75rem;
        width: 100%;
        min-height: 3.8rem;
        padding: .55rem .65rem;
        border: 0;
        border-radius: .78rem;
        color: var(--s2-text);
        background: transparent;
        text-align: left;
      }
      #${PAGE_ROOT_ID} .s2-key-select-option:hover {
        background: rgba(241,245,249,.94);
      }
      #${PAGE_ROOT_ID} .s2-key-select-option.is-active {
        color: #126fca;
        background: rgba(224,239,255,.94);
      }
      #${PAGE_ROOT_ID} .s2-key-select-option .s2-key-select-icon {
        width: 2.25rem;
        height: 2.25rem;
        border-radius: .7rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-select,
      #${PAGE_ROOT_ID} .s2-guide-page .s2-copy-field {
        min-height: 4.5rem;
        padding: .85rem 1rem;
        border-color: rgba(148,163,184,.35);
        border-radius: 1rem;
        background: rgba(226,226,229,.82);
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-copy-field code {
        padding: .35rem .6rem;
        border-radius: .5rem;
        background: rgba(148,148,153,.25);
        font-size: .9rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-note {
        min-height: 5.4rem;
        align-items: center;
        padding: 1rem 1.1rem;
        margin-top: 1rem;
        border-radius: 1rem;
        color: #6b7b79;
        background: rgba(211,225,216,.92);
        font-size: .84rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-btn {
        min-height: 2.75rem;
        padding: .62rem 1rem;
        border-radius: .8rem;
        font-size: .88rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-btn-primary {
        color: #fff;
        background: #191c20;
        box-shadow: 0 10px 24px rgba(15,23,42,.14);
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-mode-tabs,
      #${PAGE_ROOT_ID} .s2-guide-page .s2-protocol-tabs {
        padding: .35rem;
        margin-bottom: 1rem;
        border-color: rgba(148,163,184,.28);
        border-radius: .9rem;
        background: rgba(234,239,245,.78);
        box-shadow: inset 0 1px 2px rgba(15,23,42,.04);
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-mode-tabs button,
      #${PAGE_ROOT_ID} .s2-guide-page .s2-protocol-tabs button {
        display: flex;
        align-items: center;
        justify-content: center;
        gap: .5rem;
        min-height: 2.75rem;
        border: 1px solid transparent;
        border-radius: .7rem;
        font-size: .85rem;
        transition: color .18s, background .18s, box-shadow .18s;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-mode-tabs button.is-active,
      #${PAGE_ROOT_ID} .s2-guide-page .s2-protocol-tabs button.is-active {
        background: #2997ff;
        box-shadow: 0 7px 16px rgba(41,151,255,.20);
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-app-grid {
        gap: .7rem;
        margin-bottom: 1rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-app {
        min-height: 4.5rem;
        padding: .8rem;
        border-radius: .9rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-app.is-active {
        border-color: #2997ff;
        background: rgba(224,239,255,.72);
        box-shadow: 0 0 0 2px rgba(41,151,255,.08);
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-app-copy strong { font-size: .82rem; }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-app-copy small { font-size: .7rem; }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-app-detail {
        min-height: 13.5rem;
        padding: 1rem;
        border-radius: 1rem;
        background: linear-gradient(135deg,rgba(224,242,254,.7),rgba(255,255,255,.72));
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-mapping {
        min-height: 2.65rem;
        padding: .65rem .75rem;
        border-radius: .7rem;
        font-size: .78rem;
      }
      #${PAGE_ROOT_ID} .s2-guide-page .s2-model-option.is-active {
        border-color: #2997ff;
        background: rgba(224,239,255,.72);
        box-shadow: 0 0 0 3px rgba(41,151,255,.08);
      }
      #${PAGE_ROOT_ID} .s2-model-recommendation {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: .75rem;
        padding: .75rem .9rem;
        margin-bottom: .85rem;
        border: 1px solid rgba(41,151,255,.16);
        border-radius: .85rem;
        color: #526174;
        background: rgba(232,245,255,.7);
        font-size: .78rem;
      }
      #${PAGE_ROOT_ID} .s2-model-recommendation strong {
        color: #126fca;
      }
      #${PAGE_ROOT_ID} .s2-model-price {
        margin-top: .28rem;
        color: #7c8797;
        font-size: .67rem;
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page {
        --s2-primary: #3aa6ff;
        --s2-primary-strong: #70c1ff;
        --s2-primary-soft: rgba(58,166,255,.14);
        color-scheme: dark;
        background:
          radial-gradient(circle at 42% 3%, rgba(14,116,177,.13), transparent 28rem),
          radial-gradient(circle at 96% 22%, rgba(99,102,241,.11), transparent 24rem);
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-hero {
        border-color: rgba(77,163,225,.32);
        background:
          radial-gradient(circle at 92% 2%, rgba(106,92,196,.25), transparent 42%),
          radial-gradient(circle at 5% 105%, rgba(21,122,182,.17), transparent 46%),
          linear-gradient(118deg, rgba(13,33,49,.99), rgba(18,27,41,.99) 55%, rgba(16,46,42,.98));
        box-shadow: 0 24px 58px rgba(0,0,0,.24);
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-hero p,
      .dark #${PAGE_ROOT_ID} .s2-guide-trust-item { color: #a9b8ca; }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-asset-card {
        border-color: rgba(86,172,235,.38);
        color: #eef6ff;
        background: linear-gradient(145deg,rgba(31,49,71,.96),rgba(19,34,51,.96));
        box-shadow: 0 22px 48px rgba(0,0,0,.25), inset 0 1px 0 rgba(255,255,255,.05);
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-label,
      .dark #${PAGE_ROOT_ID} .s2-guide-status-note { color:#9eb0c5; }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-badge {
        color:#70c1ff;
        background:rgba(26,104,161,.24);
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-steps,
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-guide-panel {
        border:1px solid rgba(101,126,157,.12);
        background:rgba(15,26,40,.97);
        box-shadow:0 18px 42px rgba(0,0,0,.12);
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-step:hover { background:rgba(54,76,103,.28); }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-step.is-active {
        color:#8dceff;
        background:linear-gradient(135deg,rgba(25,102,163,.42),rgba(37,78,125,.34));
        box-shadow:inset 0 0 0 1px rgba(74,166,235,.16);
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-step.is-active small { color:#a9c3d8; }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-step-number { color:#9eb0c5; background:#223145; }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-step.is-active .s2-step-number { color:#fff; background:#2997ff; }
      .dark #${PAGE_ROOT_ID} .s2-guide-support { color:#a9b8ca; background:#202f43; }
      .dark #${PAGE_ROOT_ID} .s2-guide-support strong { color:#e2eaf4; }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-select,
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-copy-field {
        border-color:rgba(111,139,173,.30);
        background:rgba(27,42,62,.96);
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-copy-field code {
        color:#dce8f5;
        background:rgba(6,14,25,.46);
      }
      .dark #${PAGE_ROOT_ID} .s2-key-select-trigger,
      .dark #${PAGE_ROOT_ID} .s2-key-select-menu {
        border-color:rgba(103,134,169,.32);
        color:var(--s2-text);
        background:linear-gradient(180deg,rgba(24,39,58,.99),rgba(17,29,44,.99));
        box-shadow:0 18px 44px rgba(0,0,0,.34), inset 0 1px 0 rgba(255,255,255,.04);
      }
      .dark #${PAGE_ROOT_ID} .s2-key-select-trigger:hover,
      .dark #${PAGE_ROOT_ID} .s2-key-select.is-open .s2-key-select-trigger {
        border-color:rgba(58,166,255,.76);
        color:#f2f8ff;
        background:linear-gradient(180deg,rgba(28,49,72,.99),rgba(19,34,52,.99));
        box-shadow:0 14px 32px rgba(0,0,0,.28),0 0 0 3px rgba(58,166,255,.12);
      }
      .dark #${PAGE_ROOT_ID} .s2-key-select-icon {
        color:#70c1ff;
        background:linear-gradient(145deg,rgba(25,92,145,.48),rgba(22,65,103,.42));
      }
      .dark #${PAGE_ROOT_ID} .s2-key-select-chevron { color:#a9b8ca; }
      .dark #${PAGE_ROOT_ID} .s2-key-select-option:hover { background:rgba(55,78,106,.56); }
      .dark #${PAGE_ROOT_ID} .s2-key-select-option.is-active {
        color:#8dceff;
        background:linear-gradient(135deg,rgba(25,102,163,.48),rgba(35,72,113,.46));
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-note {
        color:#a8c7c3;
        background:rgba(25,67,62,.76);
        box-shadow:inset 0 0 0 1px rgba(82,154,143,.12);
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-btn {
        border-color:rgba(101,126,157,.30);
        color:#e4edf7;
        background:#152235;
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-btn:hover {
        border-color:rgba(58,166,255,.58);
        background:#1a2a40;
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-btn-primary {
        border-color:transparent;
        color:#fff;
        background:linear-gradient(135deg,#2997ff,#176fca);
        box-shadow:0 10px 26px rgba(15,110,197,.28);
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-mode-tabs {
        border-color:rgba(94,122,155,.26);
        background:rgba(7,17,29,.68);
        box-shadow:inset 0 1px 2px rgba(0,0,0,.25);
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-protocol-tabs {
        border-color:rgba(94,122,155,.26);
        background:rgba(7,17,29,.68);
        box-shadow:inset 0 1px 2px rgba(0,0,0,.25);
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-mode-tabs button,
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-protocol-tabs button { color:#91a3b9; background:transparent; }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-mode-tabs button.is-active,
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-protocol-tabs button.is-active { color:#fff; background:#2997ff; }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-app {
        border-color:rgba(96,124,157,.28);
        background:#142136;
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-app:hover { border-color:rgba(58,166,255,.48); background:#182840; }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-app.is-active {
        border-color:#2997ff;
        background:rgba(29,99,157,.34);
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-app-detail {
        border-color:rgba(77,152,207,.24);
        background:linear-gradient(135deg,rgba(18,54,77,.82),rgba(17,30,46,.94));
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-mapping { background:rgba(7,16,28,.38); }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-model-option {
        border-color:rgba(96,124,157,.28);
        background:#142136;
      }
      .dark #${PAGE_ROOT_ID} .s2-guide-page .s2-model-option.is-active {
        border-color:#2997ff;
        background:rgba(29,99,157,.34);
      }
      .dark #${PAGE_ROOT_ID} .s2-model-recommendation {
        border-color:rgba(58,166,255,.22);
        color:#a9b8ca;
        background:rgba(22,70,108,.34);
      }
      .dark #${PAGE_ROOT_ID} .s2-model-recommendation strong { color:#78c5ff; }
      .dark #${PAGE_ROOT_ID} .s2-model-price { color:#8fa2b8; }
      @media (max-width: 960px) {
        #${PAGE_ROOT_ID} .s2-dashboard-grid { grid-template-columns:1fr; }
        #${PAGE_ROOT_ID} .s2-guide-layout { grid-template-columns:11rem minmax(0,1fr); }
        #${PAGE_ROOT_ID} .s2-guide-page .s2-guide-layout { grid-template-columns:14rem minmax(0,1fr); }
        #${PAGE_ROOT_ID} .s2-app-grid { grid-template-columns:repeat(2,minmax(0,1fr)); }
      }
      @media (max-width: 760px) {
        #${PAGE_ROOT_ID} .s2-hero { grid-template-columns:1fr; padding:1rem; }
        #${PAGE_ROOT_ID} .s2-guide-page .s2-hero { min-height:auto; grid-template-columns:1fr; gap:1.15rem; padding:1.25rem; border-radius:1.15rem; }
        #${PAGE_ROOT_ID} .s2-guide-page .s2-hero h1 { font-size:1.85rem; }
        #${PAGE_ROOT_ID} .s2-guide-page .s2-hero p { font-size:.92rem; }
        #${PAGE_ROOT_ID} .s2-guide-page .s2-asset-card { min-height:auto; }
        #${PAGE_ROOT_ID} .s2-guide-trust { gap:.7rem 1rem; }
        #${PAGE_ROOT_ID} .s2-asset-card { width:100%; }
        #${PAGE_ROOT_ID} .s2-metrics { grid-template-columns:1fr; }
        #${PAGE_ROOT_ID} .s2-guide-layout { grid-template-columns:1fr; }
        #${PAGE_ROOT_ID} .s2-guide-page .s2-guide-layout { grid-template-columns:1fr; }
        #${PAGE_ROOT_ID} .s2-steps { overflow:visible; }
        #${PAGE_ROOT_ID} .s2-steps-list { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:.25rem; }
        #${PAGE_ROOT_ID} .s2-step { grid-template-columns:1.65rem minmax(0,1fr); }
        #${PAGE_ROOT_ID} .s2-copy-field { flex-wrap:wrap; }
        #${PAGE_ROOT_ID} .s2-copy-field code { flex:1 1 calc(100% - 3rem); }
        #${PAGE_ROOT_ID} .s2-panel-actions .s2-btn { flex:1 1 auto; }
        #${PAGE_ROOT_ID} .s2-mapping { align-items:flex-start; flex-direction:column; }
        #${PAGE_ROOT_ID} .s2-mapping strong { width:100%; text-align:left; }
        #${PAGE_ROOT_ID} .s2-panel-head { align-items:flex-start; flex-wrap:wrap; }
      }
      @media (max-width: 520px) {
        #${PAGE_ROOT_ID} .s2-page { min-height:calc(100vh - 7rem); }
        #${PAGE_ROOT_ID} .s2-hero h1 { font-size:1.38rem; }
        #${PAGE_ROOT_ID} .s2-hero-actions .s2-btn { flex:1 1 100%; }
        #${PAGE_ROOT_ID} .s2-app-grid, #${PAGE_ROOT_ID} .s2-model-grid { grid-template-columns:1fr; }
        #${PAGE_ROOT_ID} .s2-mode-tabs, #${PAGE_ROOT_ID} .s2-protocol-tabs { flex-wrap:wrap; }
        #${PAGE_ROOT_ID} .s2-mode-tabs button, #${PAGE_ROOT_ID} .s2-protocol-tabs button { flex:1 1 42%; }
        #${PAGE_ROOT_ID} .s2-panel, #${PAGE_ROOT_ID} .s2-guide-panel { padding:.8rem; }
        #${PAGE_ROOT_ID} .s2-guide-page .s2-guide-panel { min-height:auto; padding:1rem; }
        #${PAGE_ROOT_ID} .s2-guide-page .s2-step { min-height:4.5rem; padding:.65rem; }
        #${PAGE_ROOT_ID} .s2-guide-page .s2-step small { display:none; }
        #${PAGE_ROOT_ID} .s2-guide-support { display:none; }
        #${PAGE_ROOT_ID} .s2-section-head { align-items:flex-start; flex-direction:column; }
        #${PAGE_ROOT_ID} .s2-app-detail-head { flex-direction:column; }
        #${PAGE_ROOT_ID} .s2-panel-actions .s2-btn { flex:1 1 100%; }
        #${PAGE_ROOT_ID} .s2-toast { left:.75rem; right:.75rem; bottom:.75rem; text-align:center; }
      }
    `;
    pageStyle = style;
    document.head.appendChild(style);
  }

  function dashboardEmptyHtml() {
    return `
      <div class="s2-dashboard-empty">
        <div>
          <span class="s2-dashboard-icon" style="margin:0 auto">${icon('rocket', 19)}</span>
          <h3>从第一次模型调用开始</h3>
          <p>创建 API 密钥并完成一次请求后，这里会自动生成 Token 趋势、模型偏好和响应性能。</p>
          <button class="s2-btn s2-btn-primary" type="button" data-route="/keys">${icon('key', 16)}创建 API 密钥</button>
        </div>
      </div>`;
  }

  function buildDashboardHtml(data) {
    if (readObjectField(data, 'loading', false)) {
      return `
        <section class="s2-page s2-dashboard-page" data-sub2api-page="dashboard" data-dashboard-loading>
          <article class="s2-dashboard-hero" data-dashboard-hero>
            <div>
              <span class="s2-dashboard-skeleton" style="width:8rem"></span>
              <span class="s2-dashboard-skeleton is-title"></span>
              <span class="s2-dashboard-skeleton is-copy"></span>
            </div>
            <div class="s2-dashboard-balance">
              <span class="s2-dashboard-skeleton" style="width:5rem"></span>
              <span class="s2-dashboard-skeleton is-value" style="width:80%"></span>
              <span class="s2-dashboard-skeleton" style="width:6rem"></span>
            </div>
          </article>
          <div class="s2-dashboard-metrics">
            ${Array.from({ length: 3 }, () => `
              <article class="s2-dashboard-metric">
                <span class="s2-dashboard-skeleton" style="width:5rem"></span>
                <span class="s2-dashboard-skeleton is-value"></span>
                <span class="s2-dashboard-skeleton" style="width:7rem"></span>
              </article>`).join('')}
          </div>
          <div class="s2-dashboard-analysis">
            <article class="s2-dashboard-panel"><span class="s2-dashboard-skeleton is-chart"></span></article>
            <article class="s2-dashboard-panel"><span class="s2-dashboard-skeleton is-chart"></span></article>
          </div>
        </section>`;
    }

    const user = readObjectField(data, 'user', {}) || {};
    const stats = readObjectField(data, 'stats', {}) || {};
    const trendPayload = readObjectField(data, 'trend', []);
    const modelPayload = readObjectField(data, 'models', []);
    const days = readObjectField(data, 'days', 7) === 30 ? 30 : 7;
    const trend = Array.isArray(trendPayload) ? trendPayload : [];
    const models = Array.isArray(modelPayload) ? modelPayload : [];
    const hasUsage =
      Number(stats.today_requests || 0) > 0 ||
      trend.some((item) => Number(item.total_tokens || item.requests || 0) > 0);
    const chart = buildTrendSvg(trend, days);
    const modelDonut = buildModelDonutHtml(models);

    return `
      <section class="s2-page s2-dashboard-page" data-sub2api-page="dashboard">
        <article class="s2-dashboard-hero" data-dashboard-hero>
          <div class="s2-dashboard-hero-copy">
            <div class="s2-dashboard-eyebrow">${icon('sparkles', 17)}API 能力中心</div>
            <h1>让每一次模型调用，都清晰可控</h1>
            <p>统一管理密钥、用量和消费，实时掌握账户状态与模型表现。</p>
            <div class="s2-dashboard-actions">
              <button class="s2-btn s2-btn-primary" type="button" data-route="/keys">${icon('key', 16)}创建 API 密钥</button>
              <button class="s2-btn" type="button" data-overlay-view="guide">${icon('book', 16)}接入指南</button>
            </div>
          </div>
          <div class="s2-dashboard-balance">
            <span class="s2-dashboard-balance-label">可用余额</span>
            <div class="s2-dashboard-balance-value" data-dashboard-balance>${formatMoney(user.balance)}</div>
            <span class="s2-dashboard-status">账户状态正常</span>
          </div>
        </article>

        <div class="s2-dashboard-metrics">
          <article class="s2-dashboard-metric">
            <div class="s2-dashboard-metric-head"><span class="s2-dashboard-metric-label">今日请求</span><span class="s2-dashboard-icon">${icon('send', 17)}</span></div>
            <div class="s2-dashboard-metric-value" data-dashboard-requests>${formatNumber(stats.today_requests)}</div>
            <div class="s2-dashboard-metric-note"><span class="s2-dashboard-metric-accent">当前 ${formatNumber(stats.rpm)} RPM</span></div>
          </article>
          <article class="s2-dashboard-metric">
            <div class="s2-dashboard-metric-head"><span class="s2-dashboard-metric-label">今日 Token</span><span class="s2-dashboard-icon">${icon('database', 17)}</span></div>
            <div class="s2-dashboard-metric-value">${formatCompactNumber(stats.today_tokens)}</div>
            <div class="s2-dashboard-metric-note" data-dashboard-token-note>输入 ${formatCompactNumber(stats.today_input_tokens)} · 输出 ${formatCompactNumber(stats.today_output_tokens)}</div>
          </article>
          <article class="s2-dashboard-metric">
            <div class="s2-dashboard-metric-head"><span class="s2-dashboard-metric-label">平均响应</span><span class="s2-dashboard-icon">${icon('timer', 17)}</span></div>
            <div class="s2-dashboard-metric-value">${formatDuration(stats.average_duration_ms)}</div>
            <div class="s2-dashboard-metric-note">今日消费 ${formatMoney(stats.today_actual_cost)}</div>
          </article>
        </div>

        <div class="s2-dashboard-analysis">
          <article class="s2-dashboard-panel">
            <div class="s2-dashboard-panel-head">
              <div><div class="s2-dashboard-panel-title">调用活跃度</div><div class="s2-dashboard-panel-note">近 ${days} 天请求与 Token 变化</div></div>
              <div class="s2-dashboard-periods" role="group" aria-label="趋势时间范围">
                <button class="s2-btn" type="button" data-dashboard-days="7" aria-pressed="${days === 7}">7 天</button>
                <button class="s2-btn" type="button" data-dashboard-days="30" aria-pressed="${days === 30}">30 天</button>
              </div>
            </div>
            ${hasUsage && chart ? chart : dashboardEmptyHtml()}
          </article>
          <article class="s2-dashboard-panel s2-dashboard-model-panel">
            <div class="s2-dashboard-panel-head"><div><div class="s2-dashboard-panel-title">模型偏好</div><div class="s2-dashboard-panel-note">请求量占比</div></div></div>
            ${modelDonut}
          </article>
        </div>
      </section>`;
  }

  function selectedGuideKey() {
    const keys = pageState.keys.filter((key) => key?.status !== 'inactive');
    return (
      keys.find((key) => String(key.id) === String(pageState.selectedKeyId)) ||
      keys[0] ||
      pageState.keys[0] ||
      null
    );
  }

  function normalizeModelPlatform(value) {
    const normalized = String(value || '').trim().toLowerCase();
    if (!normalized) return '';
    if (
      normalized.includes('anthropic') ||
      normalized.includes('claude')
    ) {
      return 'anthropic';
    }
    if (
      normalized.includes('openai') ||
      normalized.includes('codex') ||
      normalized.includes('gpt')
    ) {
      return 'openai';
    }
    if (
      normalized.includes('gemini') ||
      normalized.includes('google') ||
      normalized.includes('antigravity')
    ) {
      return 'gemini';
    }
    return normalized;
  }

  function modelNamePlatform(name) {
    const value = String(name || '').toLowerCase();
    if (value.includes('claude')) return 'anthropic';
    if (value.includes('gemini')) return 'gemini';
    if (
      value.includes('gpt') ||
      value.includes('o1') ||
      value.includes('o3') ||
      value.includes('o4') ||
      value.includes('codex')
    ) {
      return 'openai';
    }
    return '';
  }

  function platformLabel(platform) {
    const labels = {
      openai: 'OpenAI',
      anthropic: 'Anthropic',
      gemini: 'Gemini',
    };
    return labels[platform] || String(platform || '当前');
  }

  function selectedGuideGroup(apiKey) {
    const group = apiKey?.group || {};
    const platform = normalizeModelPlatform(
      group.platform || apiKey?.platform,
    );
    return {
      name: group.name || platformLabel(platform),
      platform,
    };
  }

  function fallbackModelsForPlatform(platform) {
    const fallback = {
      openai: ['gpt-5.5', 'gpt-5.4', 'gpt-5.4-mini'],
      anthropic: ['claude-sonnet-4-6', 'claude-opus-4-6'],
      gemini: ['gemini-2.5-pro', 'gemini-2.5-flash'],
    };
    return fallback[platform] || [];
  }

  function recommendedModelsForKey(apiKey) {
    const group = selectedGuideGroup(apiKey);
    const catalog = pageState.modelCatalog
      .filter((item) => item && item.name)
      .filter((item) => String(item.type || 'text').toLowerCase() !== 'image')
      .filter(
        (item) => normalizeModelPlatform(item.vendor) === group.platform,
      );
    const preferredNames = {
      openai: [PAGE_CONFIG.ccSwitchCodexModel, 'gpt-5.4', 'gpt-5.4-mini'],
      anthropic: ['claude-sonnet-4-6', 'claude-opus-4-6'],
      gemini: ['gemini-2.5-pro', 'gemini-2.5-flash'],
    }[group.platform] || [];

    if (catalog.length) {
      return catalog
        .map((item, sourceIndex) => ({ ...item, sourceIndex }))
        .sort((left, right) => {
          const leftIndex = preferredNames.indexOf(left.name);
          const rightIndex = preferredNames.indexOf(right.name);
          const leftRank = leftIndex === -1 ? 100 + left.sourceIndex : leftIndex;
          const rightRank = rightIndex === -1 ? 100 + right.sourceIndex : rightIndex;
          return leftRank - rightRank;
        })
        .slice(0, 8);
    }

    const usageNames = pageState.models
      .map((item) => item.model || item.name || item.requested_model)
      .filter(Boolean)
      .filter((name) => modelNamePlatform(name) === group.platform);
    const fallbackNames = usageNames.length
      ? usageNames
      : fallbackModelsForPlatform(group.platform);
    return Array.from(new Set(fallbackNames)).slice(0, 8).map((name) => ({
      name,
      vendor: platformLabel(group.platform),
      type: 'text',
    }));
  }

  function formatModelPrice(value) {
    if (value === null || value === undefined || value === '') return null;
    const number = Number(value);
    if (!Number.isFinite(number)) return null;
    // 保留极小价格的有效数字，避免固定小数位把价格显示成 $0.000。
    return number === 0 ? '0' : String(number);
  }

  function modelPriceText(model) {
    const input = formatModelPrice(model?.input);
    const output = formatModelPrice(model?.output);
    if (input === null && output === null) {
      return '点击复制模型 ID';
    }
    const parts = [];
    if (input !== null) parts.push(`输入 $${input}`);
    if (output !== null) parts.push(`输出 $${output}`);
    return `${parts.join(' · ')} / 1M Token`;
  }

  function guideApps(apiKey, settings) {
    const baseUrl = String(
      settings?.api_base_url || window.location.origin,
    ).replace(/\/+$/, '');
    const platform = apiKey?.group?.platform || 'anthropic';
    const ccSwitchConfig = resolveCcSwitchImportConfig(platform, 'claude', baseUrl);
    return {
      ccswitch: {
        name: 'CC Switch',
        subtitle: '一键导入 · 推荐',
        icon: 'refresh',
        title: 'CC Switch 一键导入',
        description: '自动写入地址、密钥、模型与余额查询配置。',
        badge: '推荐',
        type:
          platform === 'openai'
            ? 'OpenAI 密钥 → Codex'
            : platform === 'gemini'
              ? 'Gemini 密钥 → Gemini CLI'
              : platform === 'grok'
                ? 'Grok 密钥 → Grok Build'
                : 'Anthropic 密钥 → Claude Code',
        endpoint: ccSwitchConfig.endpoint,
        usage: '/v1/usage · 每 30 分钟',
        action: '一键导入 CC Switch',
      },
      claude: {
        name: 'Claude Code',
        subtitle: '环境变量接入',
        icon: 'terminal',
        title: 'Claude Code 接入',
        description: '生成 ANTHROPIC_BASE_URL 与认证环境变量。',
        badge: '官方 CLI',
        type: 'Anthropic Messages',
        endpoint: baseUrl,
        usage: '仪表盘实时统计',
        action: '复制环境变量',
      },
      codex: {
        name: 'Codex CLI',
        subtitle: 'Provider 配置',
        icon: 'terminal',
        title: 'Codex CLI 接入',
        description: '生成自定义 Provider 与 Responses API 配置。',
        badge: '代码助手',
        type: 'OpenAI Responses',
        endpoint: `${baseUrl}/v1`,
        usage: '仪表盘实时统计',
        action: '复制 Codex 配置',
      },
      cherry: {
        name: 'Cherry Studio',
        subtitle: 'OpenAI 兼容',
        icon: 'book',
        title: 'Cherry Studio 接入',
        description: '选择 OpenAI 兼容服务商并填写地址与密钥。',
        badge: '桌面客户端',
        type: 'OpenAI Compatible',
        endpoint: `${baseUrl}/v1`,
        usage: '客户端模型列表',
        action: '复制配置参数',
      },
      opencode: {
        name: 'OpenCode',
        subtitle: '自定义 Provider',
        icon: 'code',
        title: 'OpenCode 接入',
        description: '创建自定义 Provider，并绑定模型与 API Key。',
        badge: '终端工具',
        type: '自定义 OpenAI Provider',
        endpoint: `${baseUrl}/v1`,
        usage: '仪表盘实时统计',
        action: '复制 Provider 配置',
      },
      cline: {
        name: 'Cline / Roo Code',
        subtitle: 'IDE 插件接入',
        icon: 'model',
        title: 'Cline / Roo Code 接入',
        description: '在 IDE 插件中选择 OpenAI Compatible Provider。',
        badge: 'IDE 插件',
        type: 'OpenAI Compatible',
        endpoint: `${baseUrl}/v1`,
        usage: '仪表盘实时统计',
        action: '复制插件参数',
      },
    };
  }

  function protocolDefinitions() {
    const baseUrl = String(
      pageState.settings?.api_base_url || window.location.origin,
    ).replace(/\/+$/, '');
    return {
      openai: {
        name: 'OpenAI',
        path: '/v1/chat/completions',
        auth: 'Authorization: Bearer',
        code: `curl "${baseUrl}/v1/chat/completions" ^
  -H "Authorization: Bearer YOUR_API_KEY" ^
  -H "Content-Type: application/json" ^
  -d '{"model":"gpt-4.1","messages":[{"role":"user","content":"你好"}]}'`,
      },
      responses: {
        name: 'Responses',
        path: '/v1/responses',
        auth: 'Authorization: Bearer',
        code: `curl "${baseUrl}/v1/responses" ^
  -H "Authorization: Bearer YOUR_API_KEY" ^
  -H "Content-Type: application/json" ^
  -d '{"model":"gpt-4.1","input":"你好"}'`,
      },
      anthropic: {
        name: 'Anthropic',
        path: '/v1/messages',
        auth: 'x-api-key',
        code: `curl "${baseUrl}/v1/messages" ^
  -H "x-api-key: YOUR_API_KEY" ^
  -H "anthropic-version: 2023-06-01" ^
  -H "Content-Type: application/json" ^
  -d '{"model":"claude-sonnet-4","max_tokens":1024,"messages":[{"role":"user","content":"你好"}]}'`,
      },
      gemini: {
        name: 'Gemini',
        path: '/v1beta/models/{model}:generateContent',
        auth: 'x-goog-api-key',
        code: `curl "${baseUrl}/v1beta/models/gemini-2.5-pro:generateContent" ^
  -H "x-goog-api-key: YOUR_API_KEY" ^
  -H "Content-Type: application/json" ^
  -d '{"contents":[{"parts":[{"text":"你好"}]}]}'`,
      },
    };
  }

  function appManualConfig(appKey, apiKey, settings) {
    const apps = guideApps(apiKey, settings);
    const app = apps[appKey] || apps.ccswitch;
    const key = String(apiKey?.key || 'YOUR_API_KEY');
    if (appKey === 'claude') {
      return `ANTHROPIC_BASE_URL=${app.endpoint}\nANTHROPIC_AUTH_TOKEN=${key}`;
    }
    if (appKey === 'codex') {
      return `Base URL: ${app.endpoint}\nAPI Key: ${key}\nAPI Mode: Responses\nModel: ${PAGE_CONFIG.ccSwitchCodexModel}`;
    }
    return `Provider: ${app.type}\nBase URL: ${app.endpoint}\nAPI Key: ${key}`;
  }

  function guideStepNavigation() {
    const steps = [
      ['1', '准备凭证', '复制地址和密钥'],
      ['2', '选择接入方式', '应用或 API'],
      ['3', '选择模型', '确认模型权限'],
      ['4', '完成接入', '查看数据与日志'],
    ];
    return steps
      .map(
        ([step, title, note]) => `
          <button class="s2-step ${pageState.guideStep === Number(step) ? 'is-active' : ''}" type="button" data-guide-step="${step}">
            <span class="s2-step-number">${step}</span>
            <span><strong>${title}</strong><small>${note}</small></span>
          </button>`,
      )
      .join('');
  }

  function guideKeySelectHtml(apiKey) {
    const hasKeys = pageState.keys.length > 0;
    const selectedGroup = selectedGuideGroup(apiKey);
    const options = pageState.keys
      .map((key) => {
        const active = String(key.id) === String(apiKey?.id);
        const group = selectedGuideGroup(key);
        return `
          <button class="s2-key-select-option ${active ? 'is-active' : ''}" type="button" role="option" aria-selected="${active}" data-guide-key-option="${escapeHtml(key.id)}">
            <span class="s2-key-select-icon">${icon('key', 16)}</span>
            <span class="s2-key-select-copy"><strong>${escapeHtml(key.name || `密钥 ${key.id}`)}</strong><small>${escapeHtml(group.name)} · ${escapeHtml(maskApiKey(key.key))}</small></span>
            ${active ? icon('check', 18) : ''}
          </button>`;
      })
      .join('');
    return `
      <div class="s2-key-select ${pageState.keyMenuOpen ? 'is-open' : ''}">
        <button class="s2-key-select-trigger" type="button" data-guide-key-toggle aria-haspopup="listbox" aria-expanded="${pageState.keyMenuOpen}" ${hasKeys ? '' : 'disabled'}>
          <span class="s2-key-select-icon">${icon('key', 18)}</span>
          <span class="s2-key-select-copy"><strong>${escapeHtml(apiKey?.name || '尚未创建 API Key')}</strong><small>${escapeHtml(selectedGroup.name)} · ${escapeHtml(maskApiKey(apiKey?.key))}</small></span>
          <span class="s2-key-select-chevron">${icon('chevron', 18)}</span>
        </button>
        ${pageState.keyMenuOpen ? `<div class="s2-key-select-menu" role="listbox" aria-label="选择 API Key">${options}</div>` : ''}
      </div>`;
  }

  function guideStepOne(apiKey) {
    const baseUrl = String(
      pageState.settings?.api_base_url || window.location.origin,
    ).replace(/\/+$/, '');
    return `
      <div class="s2-section-head"><div><h2>准备 API 地址与密钥</h2><p>密钥只在当前浏览器内使用，不会发送到第三方服务。</p></div><span class="s2-badge">步骤 1 / 4</span></div>
      <div class="s2-fields">
        <div><span class="s2-field-label">选择 API Key</span>${guideKeySelectHtml(apiKey)}</div>
        <div><span class="s2-field-label">默认 API 地址</span><div class="s2-copy-field">${icon('model', 16)}<code>${escapeHtml(baseUrl)}</code><button class="s2-btn" type="button" data-copy-text="${escapeHtml(baseUrl)}">${icon('copy', 14)}复制</button></div></div>
        <div><span class="s2-field-label">API Key</span><div class="s2-copy-field">${icon('key', 16)}<code>${escapeHtml(maskApiKey(apiKey?.key))}</code><button class="s2-btn" type="button" data-copy-secret="true">${icon('copy', 14)}复制</button></div></div>
      </div>
      <div class="s2-note">${icon('shield', 16)}<span>不要把 API Key 提交到 Git 仓库，也不要直接暴露在浏览器前端。怀疑泄露时请立即禁用并重新生成。</span></div>
      <div class="s2-panel-actions"><button class="s2-btn" type="button" data-route="/keys">${icon('key', 15)}管理密钥</button><button class="s2-btn s2-btn-primary" type="button" data-guide-step="2">下一步${icon('arrow', 15)}</button></div>`;
  }

  function guideAppsPanel(apiKey) {
    const apps = guideApps(apiKey, pageState.settings);
    const selected = apps[pageState.selectedApp] || apps.ccswitch;
    const cards = Object.entries(apps)
      .map(
        ([key, app]) => `
          <button class="s2-app ${key === pageState.selectedApp ? 'is-active' : ''}" type="button" data-guide-app="${key}">
            <span class="s2-icon-box">${icon(app.icon, 16)}</span>
            <span class="s2-app-copy"><strong>${escapeHtml(app.name)}</strong><small>${escapeHtml(app.subtitle)}</small></span>
          </button>`,
      )
      .join('');
    return `
      <div class="s2-app-grid">${cards}</div>
      <div class="s2-app-detail">
        <div class="s2-app-detail-head"><div><h3>${escapeHtml(selected.title)}</h3><p>${escapeHtml(selected.description)}</p></div><span class="s2-badge">${escapeHtml(selected.badge)}</span></div>
        <div class="s2-mapping"><span>接入类型</span><strong>${escapeHtml(selected.type)}</strong></div>
        <div class="s2-mapping"><span>服务地址</span><strong>${escapeHtml(selected.endpoint)}</strong></div>
        <div class="s2-mapping"><span>用量查询</span><strong>${escapeHtml(selected.usage)}</strong></div>
        <div class="s2-panel-actions"><button class="s2-btn s2-btn-primary" type="button" data-guide-app-action="${escapeHtml(pageState.selectedApp)}">${icon(pageState.selectedApp === 'ccswitch' ? 'external' : 'copy', 15)}${escapeHtml(selected.action)}</button></div>
      </div>`;
  }

  function guideApiPanel() {
    const protocols = protocolDefinitions();
    const selected = protocols[pageState.selectedProtocol] || protocols.openai;
    const tabs = Object.entries(protocols)
      .map(
        ([key, protocol]) => `<button class="${key === pageState.selectedProtocol ? 'is-active' : ''}" type="button" data-guide-protocol="${key}">${escapeHtml(protocol.name)}</button>`,
      )
      .join('');
    return `
      <div class="s2-protocol-tabs">${tabs}</div>
      <div class="s2-mapping"><span>接口路径</span><strong>${escapeHtml(selected.path)}</strong></div>
      <div class="s2-mapping"><span>认证方式</span><strong>${escapeHtml(selected.auth)}</strong></div>
      <pre class="s2-code"><code>${escapeHtml(selected.code)}</code></pre>
      <div class="s2-panel-actions"><button class="s2-btn" type="button" data-copy-text="${escapeHtml(selected.code)}">${icon('copy', 15)}复制代码</button><button class="s2-btn s2-btn-primary" type="button" data-test-connection>${icon('play', 15)}测试连接</button></div>`;
  }

  function guideStepTwo(apiKey) {
    return `
      <div class="s2-section-head"><div><h2>选择接入方式</h2><p>优先使用应用一键导入；需要自行开发时再使用 API 或 SDK。</p></div><span class="s2-badge">步骤 2 / 4</span></div>
      <div class="s2-mode-tabs">
        <button class="${pageState.integrationMode === 'apps' ? 'is-active' : ''}" type="button" data-guide-mode="apps">${icon('model', 15)}应用对接</button>
        <button class="${pageState.integrationMode === 'api' ? 'is-active' : ''}" type="button" data-guide-mode="api">${icon('code', 15)}API / SDK</button>
      </div>
      ${pageState.integrationMode === 'apps' ? guideAppsPanel(apiKey) : guideApiPanel()}
      <div class="s2-panel-actions"><button class="s2-btn" type="button" data-guide-step="1">上一步</button><button class="s2-btn s2-btn-primary" type="button" data-guide-step="3">下一步${icon('arrow', 15)}</button></div>`;
  }

  function guideStepThree() {
    const apiKey = selectedGuideKey();
    const group = selectedGuideGroup(apiKey);
    const models = recommendedModelsForKey(apiKey);
    const cards = models
      .map(
        (model, index) => `
          <button class="s2-model-option ${pageState.selectedModel === model.name ? 'is-active' : ''}" type="button" data-guide-model="${escapeHtml(model.name)}">
            <span><strong>${escapeHtml(model.name)}</strong><small>${index === 0 ? `${escapeHtml(group.name)}推荐模型` : `${escapeHtml(platformLabel(group.platform))} · ${escapeHtml(model.type || 'text')}`}</small><span class="s2-model-price">${escapeHtml(modelPriceText(model))}</span></span>
            ${index === 0 ? '<span class="s2-badge">推荐</span>' : icon('copy', 15)}
          </button>`,
      )
      .join('');
    return `
      <div class="s2-section-head"><div><h2>选择可用模型</h2><p>已按照当前 API Key 所属分组筛选对应模型。</p></div><span class="s2-badge">步骤 3 / 4</span></div>
      <div class="s2-model-recommendation"><span>当前密钥分组</span><strong>${escapeHtml(group.name)} · ${escapeHtml(platformLabel(group.platform))}</strong></div>
      <div class="s2-model-grid">${cards || '<div class="s2-empty" style="grid-column:1/-1;min-height:10rem"><p>当前分组暂未配置可推荐模型，请联系管理员确认模型权限。</p></div>'}</div>
      <div class="s2-note">${icon('shield', 16)}<span>接口返回 403 时，请确认当前密钥拥有该模型所在分组的访问权限。</span></div>
      <div class="s2-panel-actions"><button class="s2-btn" type="button" data-guide-step="2">上一步</button><button class="s2-btn s2-btn-primary" type="button" data-guide-step="4">完成接入${icon('arrow', 15)}</button></div>`;
  }

  function guideStepFour() {
    return `
      <div class="s2-success">
        <div>
          <span class="s2-success-mark">${icon('check', 24)}</span>
          <h2>接入完成</h2>
          <p>发起请求后，Token、消费和响应时间会自动同步到仪表盘。你可以随时返回接入指南切换应用或协议。</p>
          <div class="s2-panel-actions" style="justify-content:center"><button class="s2-btn s2-btn-primary" type="button" data-route="/usage">${icon('activity', 15)}查看使用记录</button><button class="s2-btn" type="button" data-overlay-view="dashboard">返回仪表盘</button></div>
        </div>
      </div>`;
  }

  function buildGuideHtml(loading) {
    const apiKey = selectedGuideKey();
    let panel = '';
    if (loading) {
      panel = '<div class="s2-empty"><div><span class="s2-icon-box" style="margin:0 auto">' + icon('refresh', 18) + '</span><h3>正在准备接入信息</h3><p>正在读取站点地址、API Key 和可用模型。</p></div></div>';
    } else if (pageState.guideStep === 1) {
      panel = guideStepOne(apiKey);
    } else if (pageState.guideStep === 2) {
      panel = guideStepTwo(apiKey);
    } else if (pageState.guideStep === 3) {
      panel = guideStepThree();
    } else {
      panel = guideStepFour();
    }
    return `
      <section class="s2-page s2-guide-page" data-sub2api-page="guide">
        <div class="s2-guide-breadcrumb"><button class="s2-btn s2-btn-ghost" type="button" data-overlay-view="dashboard">${icon('back', 14)}仪表盘</button><span>/</span><span>接入指南</span></div>
        <article class="s2-card s2-hero">
          <div class="s2-guide-hero-copy">
            <div class="s2-eyebrow">${icon('sparkles', 18)}开发者快速开始</div>
            <h1>5 分钟接入你的第一个模型</h1>
            <p>一个 API Key，即可调用平台支持的 OpenAI、Claude 与 Gemini 模型。跟随四个步骤完成首次请求。</p>
            <div class="s2-guide-trust">
              <span class="s2-guide-trust-item">${icon('timer', 18)}约 5 分钟</span>
              <span class="s2-guide-trust-item">${icon('shield', 18)}标准协议兼容</span>
              <span class="s2-guide-trust-item">${icon('terminal', 18)}7×24 技术支持</span>
            </div>
          </div>
          <div class="s2-asset-card">
            <div class="s2-guide-status-head"><span class="s2-label">接入准备</span><span class="s2-badge">环境正常</span></div>
            <div class="s2-asset-value">API 服务可用</div>
            <span class="s2-guide-status-note">默认线路 · 实时可用</span>
          </div>
        </article>
        <div class="s2-guide-layout">
          <aside class="s2-card s2-steps">
            <div class="s2-steps-title">接入进度</div>
            <div class="s2-steps-list">${guideStepNavigation()}</div>
            <div class="s2-guide-support"><strong>遇到问题？</strong><p>携带请求 ID 联系技术支持，可以更快定位问题。</p></div>
          </aside>
          <article class="s2-card s2-guide-panel">${panel}</article>
        </div>
      </section>`;
  }

  function showPageToast(message, version = pageRequestVersion, viewVersion = pageViewVersion) {
    if (!isCurrentView(version, viewVersion)) return;
    root.querySelector('.s2-toast')?.remove();
    const toast = document.createElement('div');
    toast.className = 's2-toast';
    toast.setAttribute('role', 'status');
    toast.textContent = message;
    root.appendChild(toast);
    schedule(() => toast.remove(), 2200);
  }

  async function copyText(value) {
    const text = String(value || '');
    if (!text) return false;
    const version = pageRequestVersion;
    const viewVersion = pageViewVersion;
    try {
      if (window.navigator.clipboard?.writeText) {
        await window.navigator.clipboard.writeText(text);
        return true;
      }
    } catch (error) {
      debugLog('Clipboard API 不可用，尝试兼容复制', error);
    }
    if (!isCurrentView(version, viewVersion)) return false;
    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.setAttribute('readonly', '');
    textarea.style.position = 'fixed';
    textarea.style.opacity = '0';
    root.appendChild(textarea);
    textarea.select();
    try {
      return Boolean(document.execCommand?.('copy'));
    } catch (error) {
      debugLog('兼容复制失败', error);
      return false;
    } finally {
      textarea.remove();
    }
  }

  async function copyWithToast(value, successMessage, failureMessage) {
    const version = pageRequestVersion;
    const viewVersion = pageViewVersion;
    const copied = await copyText(value);
    showPageToast(copied ? successMessage : failureMessage, version, viewVersion);
  }

  function rerenderGuide() {
    if (!isCurrentPage(pageRequestVersion, 'guide')) return;
    pageViewVersion += 1;
    root.innerHTML = buildGuideHtml(false);
  }

  async function testGuideConnection() {
    const apiKey = selectedGuideKey();
    if (!apiKey?.key) {
      showPageToast('请先创建或选择 API Key');
      return;
    }
    const version = pageRequestVersion;
    const viewVersion = pageViewVersion;
    const connectionVersion = ++connectionRequestVersion;
    const baseUrl = String(
      pageState.settings?.api_base_url || window.location.origin,
    ).replace(/\/+$/, '');
    try {
      const response = await fetchWithTimeout(`${baseUrl}/v1/models`, {
        headers: { Authorization: `Bearer ${apiKey.key}` },
      });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      if (connectionVersion === connectionRequestVersion) {
        showPageToast('连接成功，API Key 与服务地址可用', version, viewVersion);
      }
    } catch (error) {
      if (connectionVersion === connectionRequestVersion) {
        const message = error?.name === 'AbortError' ? '请求超时，请重试' : error?.message || '请检查配置';
        showPageToast(`连接失败：${message}`, version, viewVersion);
      }
    }
  }

  async function handlePageClick(event) {
    if (destroyed || !(event.target instanceof Element)) return;
    const control = event.target.closest(
      '[data-overlay-view], [data-route], [data-dashboard-days], [data-guide-step], [data-guide-mode], [data-guide-app], [data-guide-app-action], [data-guide-protocol], [data-guide-model], [data-guide-key-toggle], [data-guide-key-option], [data-copy-text], [data-copy-secret], [data-test-connection]',
    );
    if (!control || !root.contains(control)) {
      if (pageState.keyMenuOpen) {
        pageState.keyMenuOpen = false;
        rerenderGuide();
      }
      return;
    }

    if (control.dataset.dashboardDays) {
      const days = Number(control.dataset.dashboardDays) === 30 ? 30 : 7;
      if (pageState[DASHBOARD_DAYS_STATE_KEY] === days) return;
      pageState[DASHBOARD_DAYS_STATE_KEY] = days;
      if (root.dataset.pageMode === 'dashboard') loadDashboardIntoRoot();
      return;
    }
    if (control.hasAttribute('data-guide-key-toggle')) {
      pageState.keyMenuOpen = !pageState.keyMenuOpen;
      rerenderGuide();
      return;
    }
    if (control.dataset.guideKeyOption) {
      pageState.selectedKeyId = control.dataset.guideKeyOption;
      pageState.selectedModel = '';
      pageState.keyMenuOpen = false;
      rerenderGuide();
      return;
    }
    if (control.dataset.overlayView) {
      navigate(control.dataset.overlayView === 'guide' ? '/dashboard?view=guide' : '/dashboard');
      return;
    }
    if (control.dataset.route) {
      navigate(control.dataset.route);
      return;
    }
    if (control.dataset.guideStep) {
      pageState.guideStep = Math.min(4, Math.max(1, Number(control.dataset.guideStep) || 1));
      pageState.keyMenuOpen = false;
      rerenderGuide();
      return;
    }
    if (control.dataset.guideMode) {
      pageState.integrationMode = control.dataset.guideMode;
      rerenderGuide();
      return;
    }
    if (control.dataset.guideApp) {
      pageState.selectedApp = control.dataset.guideApp;
      rerenderGuide();
      return;
    }
    if (control.dataset.guideProtocol) {
      pageState.selectedProtocol = control.dataset.guideProtocol;
      rerenderGuide();
      return;
    }
    if (control.dataset.guideModel) {
      pageState.selectedModel = control.dataset.guideModel;
      rerenderGuide();
      await copyWithToast(control.dataset.guideModel, '模型 ID 已复制', '请选择并复制模型 ID');
      return;
    }
    if (control.hasAttribute('data-copy-secret')) {
      await copyWithToast(selectedGuideKey()?.key || '', 'API Key 已复制', '暂无可复制的 API Key');
      return;
    }
    if (control.dataset.copyText) {
      await copyWithToast(control.dataset.copyText, '内容已复制', '复制失败，请手动选择文本');
      return;
    }
    if (control.hasAttribute('data-test-connection')) {
      await testGuideConnection();
      return;
    }
    if (control.dataset.guideAppAction) {
      const key = selectedGuideKey();
      if (!key?.key) {
        showPageToast('请先创建或选择 API Key');
        return;
      }
      if (control.dataset.guideAppAction === 'ccswitch') {
        const deepLink = buildCcSwitchImportUrl(key, pageState.settings || {}, 'claude');
        const version = pageRequestVersion;
        const viewVersion = pageViewVersion;
        showPageToast('正在唤起 CC Switch');
        schedule(() => {
          if (!isCurrentView(version, viewVersion)) return;
          try {
            window.location.assign(deepLink);
          } catch (error) {
            debugLog('无法唤起 CC Switch', error);
          }
        }, 50);
        return;
      }
      const config = appManualConfig(control.dataset.guideAppAction, key, pageState.settings || {});
      await copyWithToast(config, '应用配置已复制', '复制失败，请手动配置');
    }
  }

  function handlePageChange(event) {
    if (!(event.target instanceof Element)) return;
    const select = event.target.closest('[data-guide-key]');
    if (!select || !root.contains(select)) return;
    pageState.selectedKeyId = select.value;
    pageState.selectedModel = '';
    rerenderGuide();
  }

  function handlePageKeydown(event) {
    if (event.key === 'Escape' && pageState.keyMenuOpen) {
      pageState.keyMenuOpen = false;
      rerenderGuide();
      root.querySelector('[data-guide-key-toggle]')?.focus();
    }
  }

  function loadDashboardIntoRoot() {
    if (destroyed || root.dataset.pageMode !== 'dashboard') return;
    const days = pageState[DASHBOARD_DAYS_STATE_KEY] === 30 ? 30 : 7;
    const requestVersion = ++pageRequestVersion;
    pageViewVersion += 1;
    root.innerHTML = buildDashboardHtml({ loading: true, days });
    loadDashboardData(days)
      .then((data) => {
        if (!isCurrentPage(requestVersion, 'dashboard')) return;
        root.innerHTML = buildDashboardHtml(data);
      })
      .catch((error) => {
        debugLog('加载仪表盘数据失败', error);
        if (!isCurrentPage(requestVersion, 'dashboard')) return;
        root.innerHTML = buildDashboardHtml({
          user: {}, stats: {}, trend: [], models: [], days, failed: true,
        });
      });
  }

  function render(mode) {
    if (destroyed || (mode !== 'dashboard' && mode !== 'guide')) return;
    if (root.parentElement === target && root.dataset.pageMode === mode) return;
    pageRequestVersion += 1;
    pageViewVersion += 1;
    clearPendingWork();
    injectPageStyles();
    if (root.parentElement !== target) target.appendChild(root);
    root.dataset.pageMode = mode;
    pageState.keyMenuOpen = false;
    if (mode === 'guide') {
      const requestVersion = pageRequestVersion;
      root.innerHTML = buildGuideHtml(true);
      loadGuideData()
        .then((data) => {
          if (!isCurrentPage(requestVersion, 'guide')) return;
          pageState.settings = data.settings;
          pageState.keys = data.keys;
          pageState.models = data.models;
          pageState.modelCatalog = data.modelCatalog;
          if (!pageState.selectedKeyId && data.keys[0]) {
            pageState.selectedKeyId = String(data.keys[0].id);
          }
          root.innerHTML = buildGuideHtml(false);
        })
        .catch((error) => {
          debugLog('加载接入指南数据失败', error);
          if (!isCurrentPage(requestVersion, 'guide')) return;
          root.innerHTML = buildGuideHtml(false);
        });
    } else {
      loadDashboardIntoRoot();
    }
  }

  function destroy() {
    if (destroyed) return;
    destroyed = true;
    pageRequestVersion += 1;
    pageViewVersion += 1;
    clearPendingWork();
    root.removeEventListener('click', handlePageClick);
    root.removeEventListener('change', handlePageChange);
    root.removeEventListener('keydown', handlePageKeydown);
    root.remove();
    root.replaceChildren();
    pageStyle?.remove();
    pageStyle = null;
    pageState.settings = null;
    pageState.keys = [];
    pageState.models = [];
    pageState.modelCatalog = [];
    pageState.selectedKeyId = '';
    pageState.selectedModel = '';
  }

  root.addEventListener('click', handlePageClick);
  root.addEventListener('change', handlePageChange);
  root.addEventListener('keydown', handlePageKeydown);
  return { render, destroy };
}
