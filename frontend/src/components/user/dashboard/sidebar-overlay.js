import { buildCcSwitchImportDeeplink, OPENAI_CC_SWITCH_CODEX_MODEL, resolveCcSwitchImportConfig } from '@/utils/ccswitchImport'
import { formatDateLocalInput } from '@/utils/format'
import dashboardArtworkUrl from '@/assets/dashboard-cubes.svg'
import dashboardReferenceStyles from './dashboard-reference.css?inline'
import guideReferenceStyles from './guide-reference.css?inline'

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
    availableModels: [],
    guideModelsLoading: false,
    guideModelsError: '',
    guideModelsKeyId: '',
    guideModelProtocol: 'all',
    guideSettingsError: '',
  };
  const root = document.createElement('div');
  root.id = PAGE_ROOT_ID;
  let pageStyle = null;
  let pageRequestVersion = 0;
  let pageViewVersion = 0;
  let connectionRequestVersion = 0;
  let guideModelsRequestVersion = 0;
  let guideDataRequestVersion = 0;
  let guideModelsController = null;
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
    const yesterday = new Date();
    yesterday.setDate(yesterday.getDate() - 1);
    const yesterdayDate = formatDateLocalInput(yesterday);
    const results = await Promise.allSettled([
      pageApiFetch('/auth/me'),
      pageApiFetch('/usage/dashboard/stats'),
      pageApiFetch(`/usage/dashboard/trend${query}`),
      pageApiFetch(`/usage/dashboard/models${query}`),
      pageApiFetch(`/usage/stats?start_date=${range.end}&end_date=${range.end}`),
      pageApiFetch(`/usage/stats?start_date=${yesterdayDate}&end_date=${yesterdayDate}`),
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
      today: results[4].status === 'fulfilled' ? results[4].value : null,
      yesterday: results[5].status === 'fulfilled' ? results[5].value : null,
      unavailable: results.map((result) => result.status === 'rejected'),
      partialFailure: results.some((result) => result.status === 'rejected'),
      failed: results.every((result) => result.status === 'rejected'),
    };
  }

  async function loadGuideData() {
    const results = await Promise.allSettled([
      pageApiFetch('/settings/public'),
      pageApiFetch('/keys?page=1&page_size=100'),
      pageApiFetch('/settings/home-models'),
    ]);
    const settings =
      results[0].status === 'fulfilled' ? results[0].value || {} : {};
    const keyPayload =
      results[1].status === 'fulfilled' ? results[1].value || {} : {};
    return {
      settings,
      keys: Array.isArray(keyPayload) ? keyPayload : keyPayload.items || [],
      error: results[0].status === 'rejected' || results[1].status === 'rejected'
        ? '接入信息加载失败，请重新获取。' : '',
      modelCatalog:
        results[2].status === 'fulfilled' && Array.isArray(results[2].value)
          ? results[2].value
          : [],
    };
  }

  async function loadGuideInitialData() {
    const pageVersion = pageRequestVersion;
    const dataVersion = ++guideDataRequestVersion;
    root.innerHTML = buildGuideHtml(true);
    try {
      const data = await loadGuideData();
      if (dataVersion !== guideDataRequestVersion || !isCurrentPage(pageVersion, 'guide')) return;
      pageState.settings = data.settings;
      pageState.keys = data.keys;
      pageState.modelCatalog = data.modelCatalog;
      pageState.guideSettingsError = data.error;
      if (!pageState.keys.some((key) => String(key.id) === pageState.selectedKeyId)) {
        pageState.selectedKeyId = String((data.keys.find((key) => key.status === 'active') || data.keys[0])?.id || '');
      }
      void loadGuideModels();
    } catch (error) {
      if (dataVersion !== guideDataRequestVersion || !isCurrentPage(pageVersion, 'guide')) return;
      debugLog('加载接入指南数据失败', error);
      pageState.guideSettingsError = '接入信息加载失败，请重新获取。';
      void loadGuideModels();
    }
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
      return {
        ...(rowsByDate.get(date) || {
          requests: 0,
          total_tokens: 0,
          actual_cost: 0,
        }),
        date,
      };
    });
  }

  function nonnegativeNumber(value) {
    const number = Number(value);
    return Number.isFinite(number) && number > 0 ? number : 0;
  }

  function buildSmoothPath(points) {
    if (!points.length) return '';
    let path = `M ${points[0].x} ${points[0].y}`;
    for (let index = 0; index < points.length - 1; index += 1) {
      const previous = points[index - 1] || points[index];
      const current = points[index];
      const next = points[index + 1];
      const afterNext = points[index + 2] || next;
      // 将控制点限制在相邻值之间，避免平滑曲线产生负用量。
      const clampY = (value) => Math.min(Math.max(current.y, next.y), Math.max(Math.min(current.y, next.y), value));
      const controlOneX = current.x + (next.x - previous.x) / 6;
      const controlOneY = clampY(current.y + (next.y - previous.y) / 6);
      const controlTwoX = next.x - (afterNext.x - current.x) / 6;
      const controlTwoY = clampY(next.y - (afterNext.y - current.y) / 6);
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
    const rows = normalizeDashboardTrend(trend, safeDays);
    const requests = rows.map((item) => nonnegativeNumber(item.requests));
    const tokens = rows.map((item) => nonnegativeNumber(item.total_tokens));
    if (![...requests, ...tokens].some((value) => value > 0)) return '';
    const width = 680;
    const height = 245;
    const left = 40;
    const right = 50;
    const top = 28;
    const baseline = 210;
    const plotWidth = width - left - right;
    const plotHeight = baseline - top;
    const requestMax = Math.max(...requests, 1);
    const tokenMax = Math.max(...tokens, 1);
    const pointX = (index) => left + plotWidth * index / (rows.length - 1);
    const requestPoints = requests.map((value, index) => ({ x: pointX(index), y: baseline - value / requestMax * plotHeight }));
    const tokenPoints = tokens.map((value, index) => ({ x: pointX(index), y: baseline - value / tokenMax * plotHeight }));
    const requestPath = buildSmoothPath(requestPoints);
    const tokenPath = buildSmoothPath(tokenPoints);
    const areaPath = (path) => `${path} L ${left + plotWidth} ${baseline} L ${left} ${baseline} Z`;
    const grids = Array.from({ length: 4 }, (_, index) => {
      const ratio = index / 3;
      const y = baseline - ratio * plotHeight;
      return `<line x1="${left}" y1="${y}" x2="${left + plotWidth}" y2="${y}"></line>`;
    }).join('');
    const scaleLabels = Array.from({ length: 4 }, (_, index) => {
      const ratio = index / 3;
      const y = baseline - ratio * plotHeight + 4;
      return `<text x="${left - 9}" y="${y}" text-anchor="end">${formatCompactNumber(Math.round(requestMax * ratio))}</text><text x="${width - right + 9}" y="${y}" text-anchor="start">${formatCompactNumber(Math.round(tokenMax * ratio))}</text>`;
    }).join('');
    const labels = rows.map((row, index) => {
      if (safeDays !== 7 && index !== 0 && index !== rows.length - 1 && index % 5 !== 0) return '';
      return `<text x="${pointX(index)}" y="235" text-anchor="middle">${escapeHtml(safeDays === 7 ? dashboardWeekday(row.date) : row.date.slice(5))}</text>`;
    }).join('');
    const step = plotWidth / (rows.length - 1);
    const hitAreas = rows.map((row, index) => `<rect data-trend-point="${escapeHtml(row.date)}" data-trend-x="${pointX(index)}" data-trend-request-y="${requestPoints[index].y}" data-trend-token-y="${tokenPoints[index].y}" data-trend-requests="${requests[index]}" data-trend-tokens="${tokens[index]}" x="${Math.max(left, pointX(index) - step / 2)}" y="${top}" width="${index === 0 || index === rows.length - 1 ? step / 2 : step}" height="${plotHeight}" tabindex="0" role="button" aria-label="${escapeHtml(row.date)}，请求 ${formatNumber(requests[index])} 次，Token ${formatNumber(tokens[index])}"></rect>`).join('');
    return `
      <div class="s2-chart-legend"><span><i class="s2-chart-dot"></i>请求次数（左轴）</span><span><i class="s2-chart-dot is-token"></i>Token 消耗（右轴）</span></div>
      <div class="s2-chart-wrap">
        <svg class="s2-trend-chart" data-dashboard-range="${safeDays}" data-dashboard-start-date="${rows[0].date}" data-dashboard-end-date="${rows[rows.length - 1].date}" viewBox="0 0 ${width} ${height}" role="group" aria-label="近 ${safeDays} 天请求与 Token 趋势，选择日期查看用量">
          <defs>
            <linearGradient id="${PAGE_ROOT_ID}-requests-area" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#2678ff" stop-opacity=".22"/><stop offset="1" stop-color="#2678ff" stop-opacity=".01"/></linearGradient>
            <linearGradient id="${PAGE_ROOT_ID}-tokens-area" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#ab78ff" stop-opacity=".22"/><stop offset="1" stop-color="#ab78ff" stop-opacity=".01"/></linearGradient>
          </defs>
          <g class="s2-chart-grid">${grids}</g>
          <path d="${areaPath(requestPath)}" fill="url(#${PAGE_ROOT_ID}-requests-area)"></path>
          <path d="${areaPath(tokenPath)}" fill="url(#${PAGE_ROOT_ID}-tokens-area)"></path>
          <path class="s2-chart-line" data-trend-series="requests" d="${requestPath}"></path>
          <path class="s2-chart-line is-tokens" data-trend-series="tokens" d="${tokenPath}"></path>
          <g class="s2-chart-labels">${scaleLabels}${labels}</g>
          <g data-trend-markers visibility="hidden"><line class="s2-chart-tracker" y1="${top}" y2="${baseline}"></line><circle class="s2-chart-point" r="5"></circle><circle class="s2-chart-point is-tokens" r="5"></circle></g>
          ${hitAreas}
        </svg>
        <div class="s2-chart-tooltip-bubble" data-trend-tooltip role="status" hidden></div>
      </div>`;
  }

  function handleTrendPoint(event) {
    if (!(event.target instanceof Element)) return;
    const point = event.target.closest('[data-trend-point]');
    if (!point || !root.contains(point)) return;
    const chart = point.closest('.s2-chart-wrap');
    const tooltip = chart.querySelector('[data-trend-tooltip]');
    const markers = chart.querySelector('[data-trend-markers]');
    if (!tooltip || !markers) return;
    const x = Number(point.dataset.trendX);
    tooltip.style.setProperty('--s2-tooltip-x', `${Math.min(82, Math.max(18, x / 680 * 100))}%`);
    tooltip.innerHTML = `<strong>${escapeHtml(point.dataset.trendPoint)} · ${dashboardWeekday(point.dataset.trendPoint)}</strong><span><i class="s2-chart-dot"></i>请求 ${formatNumber(point.dataset.trendRequests)} 次</span><span><i class="s2-chart-dot is-token"></i>Token ${formatCompactNumber(point.dataset.trendTokens)}</span>`;
    tooltip.hidden = false;
    markers.setAttribute('visibility', 'visible');
    const tracker = markers.querySelector('line');
    tracker.setAttribute('x1', String(x));
    tracker.setAttribute('x2', String(x));
    markers.querySelectorAll('circle').forEach((circle, index) => {
      circle.setAttribute('cx', String(x));
      circle.setAttribute('cy', index === 0 ? point.dataset.trendRequestY : point.dataset.trendTokenY);
    });
  }

  function normalizeModelRows(models) {
    const rows = models.map((item) => ({
      name: item.model || item.name || item.requested_model || '未知模型',
      value: nonnegativeNumber(item.requests ?? item.request_count ?? item.total_requests),
    })).filter((item) => item.value > 0).sort((a, b) => b.value - a.value);
    const total = rows.reduce((sum, item) => sum + item.value, 0);
    if (!total) return [];
    const grouped = rows.slice(0, 3);
    if (rows.length > 3) grouped.push({ name: '其他', value: rows.slice(3).reduce((sum, item) => sum + item.value, 0) });
    const result = grouped.map((item) => ({ ...item, ratio: item.value / total, percent: Math.floor(item.value / total * 100) }));
    // 最大余数法保证展示百分比总和为 100%，圆环长度始终使用真实比例。
    const remainderOrder = result.map((item, index) => ({ index, remainder: item.ratio * 100 - item.percent })).sort((a, b) => b.remainder - a.remainder);
    const remaining = 100 - result.reduce((sum, item) => sum + item.percent, 0);
    for (let index = 0; index < remaining; index += 1) result[remainderOrder[index].index].percent += 1;
    return result;
  }

  function buildModelDonutHtml(models) {
    const rows = normalizeModelRows(Array.isArray(models) ? models : []);
    const circumference = 2 * Math.PI * 44;
    const colors = ['#3187ff', '#ff8969', '#27d5bd', '#ac96ff'];
    let offset = 0;
    const segments = rows.map((row, index) => {
      const length = row.ratio * circumference;
      const segment = `<circle class="s2-donut-segment" cx="60" cy="60" r="44" stroke="${colors[index]}" stroke-dasharray="${length} ${circumference - length}" stroke-dashoffset="${-offset}"><title>${escapeHtml(row.name)}：${row.percent}% · ${formatNumber(row.value)} 次请求</title></circle>`;
      offset += length;
      return segment;
    }).join('');
    const legends = rows.map((item, index) => `<div class="s2-donut-legend" data-dashboard-model-legend title="${escapeHtml(item.name)} · ${formatNumber(item.value)} 次请求"><span class="s2-donut-swatch" style="--s2-swatch:${colors[index]}"></span><span class="s2-donut-label">${escapeHtml(item.name)}</span><span class="s2-donut-percent">${item.percent}%</span></div>`).join('');
    return `<div class="s2-donut-wrap ${rows.length ? '' : 'is-empty'}"><svg class="s2-donut" data-dashboard-donut viewBox="0 0 120 120" role="img" aria-label="模型请求占比圆环图"><title>模型请求占比</title><circle class="s2-donut-base" cx="60" cy="60" r="44"></circle>${segments}<text class="s2-donut-value" x="60" y="60">${rows.length ? '100%' : '0%'}</text><text class="s2-donut-caption" x="60" y="76">总请求量</text></svg><div class="s2-donut-copy">${legends || '<div class="s2-donut-empty">暂无模型调用</div>'}</div></div>`;
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
    ).replace(/\/+$/, '').replace(/\/v1$/, '');
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
    const deepLink = new URL(buildCcSwitchImportDeeplink({
      baseUrl,
      platform: guideClientPlatform(apiKey),
      clientType,
      providerName: String(settings?.site_name || 'sub2api').trim() || 'sub2api',
      apiKey: String(apiKey?.key || ''),
      usageScript,
    }));
    // CC Switch 的 Codex / Grok 导入支持 model 参数，沿用共享工具生成其余字段。
    if (pageState.selectedModel && ['codex', 'grokbuild'].includes(deepLink.searchParams.get('app'))) {
      deepLink.searchParams.set('model', pageState.selectedModel);
    }
    return deepLink.toString();
  }
  function injectPageStyles() {
    if (pageStyle) return;
    const style = document.createElement('style');
    style.id = PAGE_STYLE_ID;
    style.textContent = `${dashboardReferenceStyles}\n${guideReferenceStyles}`
      .replaceAll('s2-page-design-root', PAGE_ROOT_ID);
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

    const user = data.user || {};
    const stats = data.stats || {};
    const today = data.today;
    const yesterday = data.yesterday;
    const unavailable = data.unavailable || [];
    const days = data.days === 30 ? 30 : 7;
    const trend = Array.isArray(data.trend) ? data.trend : [];
    const models = Array.isArray(data.models) ? data.models : [];
    const metric = (dailyField, fallbackField) => today?.[dailyField] ?? stats[fallbackField];
    const requestCount = metric('total_requests', 'today_requests');
    const tokenCount = metric('total_tokens', 'today_tokens');
    const duration = metric('average_duration_ms', 'average_duration_ms');
    const display = (value, formatter) => value == null ? '—' : formatter(value);
    const change = (name, field, invert = false) => {
      let label = '暂无对比';
      let kind = 'is-neutral';
      if (name === 'duration' && today?.total_requests === 0) {
        label = '今日暂无数据';
      } else if (name === 'duration' && yesterday?.total_requests === 0) {
        label = '昨日暂无数据';
      } else if (today?.[field] != null && yesterday?.[field] != null) {
        const before = nonnegativeNumber(yesterday[field]);
        const current = nonnegativeNumber(today[field]);
        if (before === 0) label = '昨日暂无数据';
        else {
          const percent = (current - before) / before * 100;
          label = percent === 0 ? '持平' : `${percent > 0 ? '↑' : '↓'} ${Math.abs(percent).toFixed(1).replace(/\.0$/, '')}%`;
          kind = percent === 0 ? 'is-neutral' : (invert ? percent > 0 : percent < 0) ? 'is-negative' : '';
        }
      }
      return `<span class="s2-change ${kind}" data-dashboard-change="${name}">${label}</span>${kind === 'is-neutral' ? '' : '<span>较昨日</span>'}`;
    };
    const spark = (field, isTokens = false) => {
      const values = normalizeDashboardTrend(trend, days).slice(-7).map((row) => nonnegativeNumber(row[field]));
      const max = Math.max(...values, 1);
      if (!values.some((value) => value > 0)) return '';
      return `<svg class="s2-metric-spark ${isTokens ? 'is-tokens' : ''}" viewBox="0 0 96 43" role="img" aria-label="近七天${isTokens ? 'Token' : '请求'}变化"><title>${values.map((value) => formatNumber(value)).join('、')}</title>${values.map((value, index) => `<rect x="${index * 13 + 4}" y="${41 - value / max * 37}" width="6" height="${value / max * 37}" rx="3" fill="currentColor"></rect>`).join('')}</svg>`;
    };
    const chart = buildTrendSvg(trend, days);
    const accountStatus = user.status === 'active' ? '账户状态正常' : user.status === 'disabled' ? '账户已停用' : '账户状态待确认';
    const alert = data.failed || data.partialFailure ? `<div class="s2-dashboard-alert" role="alert"><span>${data.failed ? '仪表盘加载失败，请重试。' : '部分数据加载失败，暂时无法显示完整用量。'}</span><button class="s2-btn" data-dashboard-retry type="button">${icon('refresh', 14)}重新加载</button></div>` : '';
    const chartError = unavailable[2] || data.failed;
    const modelError = unavailable[3] || data.failed;
    return `
      <section class="s2-page s2-dashboard-page" data-sub2api-page="dashboard">
        <article class="s2-dashboard-hero" data-dashboard-hero>
          <img class="s2-dashboard-art" src="${dashboardArtworkUrl}" alt="" aria-hidden="true">
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
            <button class="s2-dashboard-balance-link" type="button" data-route="/profile" aria-label="查看账户余额">${icon('arrow', 13)}</button>
            <div class="s2-dashboard-balance-value" data-dashboard-balance>${display(user.balance, formatMoney)}</div>
            <span class="s2-dashboard-status ${user.status === 'active' ? '' : 'is-unknown'}">${accountStatus}</span>
          </div>
        </article>
        ${alert}
        <div class="s2-dashboard-metrics">
          <article class="s2-dashboard-metric">
            <div class="s2-dashboard-metric-head"><span class="s2-dashboard-metric-label"><span class="s2-dashboard-icon">${icon('activity', 15)}</span>今日请求</span><span class="s2-dashboard-icon">${icon('send', 16)}</span></div>
            <div class="s2-dashboard-metric-value" data-dashboard-requests>${display(requestCount, formatNumber)}</div>
            <div class="s2-dashboard-metric-note">${change('requests', 'total_requests')}</div>
            ${spark('requests')}
          </article>
          <article class="s2-dashboard-metric">
            <div class="s2-dashboard-metric-head"><span class="s2-dashboard-metric-label"><span class="s2-dashboard-icon">${icon('database', 15)}</span>今日 Token</span><span class="s2-dashboard-icon">${icon('database', 16)}</span></div>
            <div class="s2-dashboard-metric-value" data-dashboard-tokens>${display(tokenCount, formatCompactNumber)}</div>
            <div class="s2-dashboard-metric-note">${change('tokens', 'total_tokens')}</div>
            <div class="s2-dashboard-metric-detail" data-dashboard-token-note>输入 ${display(metric('total_input_tokens', 'today_input_tokens'), formatCompactNumber)} · 输出 ${display(metric('total_output_tokens', 'today_output_tokens'), formatCompactNumber)}</div>
            ${spark('total_tokens', true)}
          </article>
          <article class="s2-dashboard-metric">
            <div class="s2-dashboard-metric-head"><span class="s2-dashboard-metric-label"><span class="s2-dashboard-icon">${icon('timer', 15)}</span>${today?.average_duration_ms != null ? '平均响应' : '平均响应（累计）'}</span><span class="s2-dashboard-icon">${icon('timer', 16)}</span></div>
            <div class="s2-dashboard-metric-value" data-dashboard-duration>${today?.total_requests === 0 ? '—' : display(duration, formatDuration)}</div>
            <div class="s2-dashboard-metric-note">${change('duration', 'average_duration_ms', true)}</div>
            <div class="s2-dashboard-metric-detail">今日消费 ${display(metric('total_actual_cost', 'today_actual_cost'), formatMoney)}</div>
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
            ${chartError ? '<div class="s2-dashboard-empty"><p>趋势数据加载失败，请点击上方重新加载。</p></div>' : chart || dashboardEmptyHtml()}
          </article>
          <article class="s2-dashboard-panel s2-dashboard-model-panel">
            <div class="s2-dashboard-panel-head"><div><div class="s2-dashboard-panel-title">模型偏好</div><div class="s2-dashboard-panel-note">请求量占比</div></div><button class="s2-dashboard-detail" type="button" data-route="/dashboard?view=classic">查看详情 ${icon('arrow', 13)}</button></div>
            ${modelError ? '<div class="s2-dashboard-empty"><p>模型数据加载失败，请点击上方重新加载。</p></div>' : buildModelDonutHtml(models)}
          </article>
        </div>
      </section>`;
  }
  function selectedGuideKey() {
    const keys = pageState.keys.filter((key) => key?.status === 'active');
    return (
      pageState.keys.find((key) => String(key.id) === String(pageState.selectedKeyId)) ||
      keys[0] ||
      pageState.keys[0] ||
      null
    );
  }

  function guideBaseUrl() {
    const raw = String(pageState.settings?.api_base_url || window.location.origin).trim();
    try {
      const url = new URL(raw, window.location.origin);
      if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) return '';
      url.search = '';
      url.hash = '';
      return url.toString().replace(/\/+$/, '').replace(/\/v1$/, '');
    } catch {
      return '';
    }
  }

  function guideModelsEndpoint(apiKey) {
    const baseUrl = guideBaseUrl();
    if (!baseUrl) throw new Error('API 地址配置无效，请联系管理员');
    return apiKey?.group?.platform === 'antigravity'
      ? `${baseUrl}/antigravity/v1/models` : `${baseUrl}/v1/models`;
  }

  async function loadGuideModels() {
    guideModelsController?.abort();
    const version = ++guideModelsRequestVersion;
    const pageVersion = pageRequestVersion;
    const apiKey = selectedGuideKey();
    const keyId = String(apiKey?.id || '');
    pageState.availableModels = [];
    pageState.guideModelsKeyId = keyId;
    pageState.guideModelsError = '';
    pageState.selectedModel = '';
    pageState.guideModelProtocol = 'all';
    if (!apiKey?.key || apiKey.status !== 'active' || pageState.guideSettingsError) {
      pageState.guideModelsLoading = false;
      pageState.guideModelsError = pageState.guideSettingsError || (apiKey ? '当前密钥不可用，请选择有效密钥或前往密钥管理。' : '');
      rerenderGuide();
      return;
    }
    const controller = new AbortController();
    guideModelsController = controller;
    pendingControllers.add(controller);
    const timer = schedule(() => controller.abort(), PAGE_CONFIG.requestTimeoutMs);
    pageState.guideModelsLoading = true;
    rerenderGuide();
    try {
      const response = await window.fetch(guideModelsEndpoint(apiKey), {
        headers: { Authorization: `Bearer ${apiKey.key}` },
        signal: controller.signal,
      });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const payload = await response.json();
      const rows = Array.isArray(payload) ? payload : payload?.data || payload?.models;
      if (!Array.isArray(rows)) throw new Error('模型列表格式无效');
      // 仅采用当前密钥网关返回的模型，展示目录不能替代分组访问权限。
      const seen = new Set();
      const models = rows.flatMap((item) => {
        const name = String(typeof item === 'string' ? item : item?.id || item?.slug || item?.name || '').replace(/^models\//, '').trim();
        if (!name || seen.has(name)) return [];
        seen.add(name);
        return [{ name, vendor: item?.owned_by || '', type: 'text' }];
      });
      if (version !== guideModelsRequestVersion || !isCurrentPage(pageVersion, 'guide')) return;
      pageState.availableModels = models;
      pageState.selectedModel = models[0]?.name || '';
      syncGuideProtocol();
    } catch (error) {
      if (version !== guideModelsRequestVersion || !isCurrentPage(pageVersion, 'guide')) return;
      pageState.guideModelsError = error?.name === 'AbortError'
        ? '模型列表请求超时，请重试。'
        : '无法获取当前密钥的模型列表，请检查密钥权限和服务地址后重试。';
    } finally {
      pendingControllers.delete(controller);
      pendingTimers.delete(timer);
      window.clearTimeout(timer);
      if (guideModelsController === controller) guideModelsController = null;
      if (version === guideModelsRequestVersion && isCurrentPage(pageVersion, 'guide')) {
        pageState.guideModelsLoading = false;
        rerenderGuide();
      }
    }
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
    if (value.includes('grok')) return 'grok';
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
      grok: 'Grok',
      composite: '综合路由',
      antigravity: 'Antigravity',
    };
    return labels[platform] || String(platform || '当前');
  }

  function selectedGuideGroup(apiKey) {
    const group = apiKey?.group || {};
    const platform = String(group.platform || apiKey?.platform || '');
    return {
      name: group.name || platformLabel(platform),
      platform,
    };
  }

  function recommendedModelsForKey(apiKey) {
    const group = selectedGuideGroup(apiKey);
    if (String(apiKey?.id || '') !== pageState.guideModelsKeyId) return [];
    const catalog = new Map(pageState.modelCatalog.filter((item) => item?.name).map((item) => [item.name, item]));
    return pageState.availableModels.map((item) => {
      const vendorPlatform = normalizeModelPlatform(catalog.get(item.name)?.vendor || item.vendor);
      return {
        ...item,
        ...catalog.get(item.name),
        platform: modelNamePlatform(item.name) || (['openai', 'anthropic', 'gemini', 'grok'].includes(vendorPlatform) ? vendorPlatform : group.platform),
      };
    });
  }

  function guideClientPlatform(apiKey) {
    const platform = apiKey?.group?.platform || 'anthropic';
    if (platform !== 'composite') return platform;
    return recommendedModelsForKey(apiKey).find((model) => model.name === pageState.selectedModel)?.platform || 'anthropic';
  }

  function guideClientType(apiKey) {
    const model = recommendedModelsForKey(apiKey).find((item) => item.name === pageState.selectedModel);
    return apiKey?.group?.platform === 'antigravity' && model?.platform === 'gemini' ? 'gemini' : 'claude';
  }

  function syncGuideProtocol() {
    const model = recommendedModelsForKey(selectedGuideKey()).find((item) => item.name === pageState.selectedModel);
    if (!model) return;
    if (model.platform === 'openai' && pageState.selectedProtocol === 'responses') return;
    pageState.selectedProtocol = ['anthropic', 'gemini'].includes(model.platform) ? model.platform : 'openai';
  }

  function hasVerifiedGuideModel() {
    return !pageState.guideModelsLoading && !pageState.guideModelsError && !pageState.guideSettingsError
      && selectedGuideKey()?.status === 'active'
      && recommendedModelsForKey(selectedGuideKey()).some((model) => model.name === pageState.selectedModel);
  }

  function guideProtocolUnavailable(protocol, apiKey = selectedGuideKey()) {
    const group = apiKey?.group || {};
    if (group.claude_code_only) return '当前分组仅允许 Claude Code 客户端，请选择应用对接中的 Claude Code。';
    if (protocol === 'anthropic' && guideClientPlatform(apiKey) === 'openai' && group.allow_messages_dispatch === false) {
      return '当前分组未开启 Anthropic Messages 接入，请使用 OpenAI 或 Responses 协议。';
    }
    if (protocol === 'gemini' && group.platform && !['gemini', 'antigravity', 'composite'].includes(group.platform)) {
      return '当前分组不支持 Gemini 原生协议，请切换 Gemini 或 Antigravity 密钥。';
    }
    return '';
  }

  function guideAppUnavailable(appKey, apiKey) {
    const group = apiKey?.group || {};
    const ccConfig = resolveCcSwitchImportConfig(guideClientPlatform(apiKey), guideClientType(apiKey), guideBaseUrl());
    const isClaude = appKey === 'claude' || (appKey === 'ccswitch' && ccConfig.app === 'claude');
    if (group.claude_code_only && !isClaude) return '当前分组仅允许 Claude Code 客户端，请选择 Claude Code 接入。';
    if (isClaude && guideClientPlatform(apiKey) === 'openai' && group.allow_messages_dispatch === false) {
      return '当前分组未开启 Claude Code 接入，请使用 Codex CLI 或其他 OpenAI 兼容客户端。';
    }
    return '';
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
      return '价格以实际计费为准';
    }
    const parts = [];
    if (input !== null) parts.push(`输入 $${input}`);
    if (output !== null) parts.push(`输出 $${output}`);
    return `参考价 · ${parts.join(' · ')} / 1M Token`;
  }

  function guideApps(apiKey, settings) {
    const baseUrl = String(
      settings?.api_base_url || window.location.origin,
    ).replace(/\/+$/, '').replace(/\/v1$/, '');
    const nativeBaseUrl = apiKey?.group?.platform === 'antigravity' ? `${baseUrl}/antigravity` : baseUrl;
    const platform = guideClientPlatform(apiKey);
    const ccSwitchConfig = resolveCcSwitchImportConfig(platform, guideClientType(apiKey), baseUrl);
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
            : ccSwitchConfig.app === 'gemini'
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
        endpoint: nativeBaseUrl,
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
    ).replace(/\/+$/, '').replace(/\/v1$/, '');
    const model = pageState.selectedModel || 'YOUR_MODEL';
    // Antigravity 专属路由仅承载原生协议；OpenAI 兼容请求继续使用根 /v1 转换接口。
    const nativePrefix = selectedGuideKey()?.group?.platform === 'antigravity' ? '/antigravity' : '';
    // PowerShell 单引号字符串使用两个单引号转义，避免模型或地址被解释为命令。
    const quote = (value) => `'${String(value).replace(/'/g, "''")}'`;
    const command = (path, headers, body) => [
      `curl.exe ${quote(`${baseUrl}${path}`)}`,
      '--request POST',
      ...headers.map((header) => `--header ${quote(header)}`),
      `--header ${quote('Content-Type: application/json')}`,
      `--data-raw ${quote(JSON.stringify(body))}`,
    ].join(' `\n  ');
    return {
      openai: {
        name: 'OpenAI',
        path: '/v1/chat/completions',
        auth: 'Authorization: Bearer',
        code: command('/v1/chat/completions', ['Authorization: Bearer YOUR_API_KEY'], {
          model, messages: [{ role: 'user', content: '你好' }],
        }),
      },
      responses: {
        name: 'Responses',
        path: '/v1/responses',
        auth: 'Authorization: Bearer',
        code: command('/v1/responses', ['Authorization: Bearer YOUR_API_KEY'], { model, input: '你好' }),
      },
      anthropic: {
        name: 'Anthropic',
        path: `${nativePrefix}/v1/messages`,
        auth: 'x-api-key',
        code: command(`${nativePrefix}/v1/messages`, ['x-api-key: YOUR_API_KEY', 'anthropic-version: 2023-06-01'], {
          model, max_tokens: 1024, messages: [{ role: 'user', content: '你好' }],
        }),
      },
      gemini: {
        name: 'Gemini',
        path: `${nativePrefix}/v1beta/models/{model}:generateContent`,
        auth: 'x-goog-api-key',
        code: command(`${nativePrefix}/v1beta/models/${encodeURIComponent(model)}:generateContent`, ['x-goog-api-key: YOUR_API_KEY'], {
          contents: [{ parts: [{ text: '你好' }] }],
        }),
      },
    };
  }

  function appManualConfig(appKey, apiKey, settings) {
    const apps = guideApps(apiKey, settings);
    const app = apps[appKey] || apps.ccswitch;
    const key = String(apiKey?.key || 'YOUR_API_KEY');
    const model = pageState.selectedModel || 'YOUR_MODEL';
    if (appKey === 'claude') {
      return `ANTHROPIC_BASE_URL=${app.endpoint}\nANTHROPIC_AUTH_TOKEN=${key}\nANTHROPIC_MODEL=${model}`;
    }
    if (appKey === 'codex') {
      return `Base URL: ${app.endpoint}\nAPI Key: ${key}\nAPI Mode: Responses\nModel: ${model}`;
    }
    return `Provider: ${app.type}\nBase URL: ${app.endpoint}\nAPI Key: ${key}\nModel: ${model}`;
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
          <button class="s2-step ${pageState.guideStep === Number(step) ? 'is-active' : ''} ${pageState.guideStep > Number(step) ? 'is-complete' : ''}" type="button" data-guide-step="${step}" ${pageState.guideStep === Number(step) ? 'aria-current="step"' : ''} ${step === '4' && !pageState.selectedModel ? 'disabled' : ''}>
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
            <span class="s2-key-select-copy"><strong>${escapeHtml(key.name || `密钥 ${key.id}`)}${key.status === 'active' ? '' : ' · 不可用'}</strong><small>${escapeHtml(group.name)} · ${escapeHtml(maskApiKey(key.key))}</small></span>
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
      <div class="s2-section-head"><div><h2>准备 API 地址与密钥</h2><p>选择项目密钥，复制服务地址即可开始接入。</p></div><span class="s2-badge">步骤 1 / 4</span></div>
      <div class="s2-fields">
        <div><span class="s2-field-label">选择 API Key</span>${guideKeySelectHtml(apiKey)}</div>
        <div><span class="s2-field-label">默认 API 地址</span><div class="s2-copy-field">${icon('model', 16)}<code>${escapeHtml(baseUrl)}</code><button class="s2-btn" type="button" data-copy-text="${escapeHtml(baseUrl)}">${icon('copy', 14)}复制</button></div></div>
        <div><span class="s2-field-label">API Key</span><div class="s2-copy-field">${icon('key', 16)}<code>${escapeHtml(maskApiKey(apiKey?.key))}</code><button class="s2-btn" type="button" data-copy-secret="true" ${apiKey?.key ? '' : 'disabled'}>${icon('copy', 14)}复制</button></div></div>
      </div>
      <div class="s2-note">${icon('shield', 16)}<span>不要把 API Key 提交到 Git 仓库，也不要直接暴露在浏览器前端。怀疑泄露时请立即禁用并重新生成。</span></div>
      <div class="s2-panel-actions"><button class="s2-btn s2-btn-quiet" type="button" data-route="/keys">${icon('key', 15)}${apiKey ? '管理密钥' : '创建 API 密钥'}</button><button class="s2-btn s2-btn-primary" type="button" data-guide-step="2" ${apiKey?.key && apiKey.status === 'active' ? '' : 'disabled'}>下一步${icon('arrow', 15)}</button></div>`;
  }

  function guideAppsPanel(apiKey) {
    const apps = guideApps(apiKey, pageState.settings);
    const selected = apps[pageState.selectedApp] || apps.ccswitch;
    const unavailable = guideAppUnavailable(pageState.selectedApp, apiKey);
    const ready = hasVerifiedGuideModel() && !unavailable;
    const cards = Object.entries(apps)
      .map(
        ([key, app]) => `
          <button class="s2-app ${key === pageState.selectedApp ? 'is-active' : ''}" type="button" data-guide-app="${key}">
            <span class="s2-icon-box">${icon(app.icon, 16)}</span>
            <span class="s2-app-copy"><strong>${escapeHtml(app.name)}</strong><small>${escapeHtml(app.subtitle)}</small></span>
            <span class="s2-app-selected">${key === pageState.selectedApp ? icon('check', 11) : ''}</span>
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
        ${pageState.selectedModel ? `<div class="s2-mapping"><span>当前模型</span><strong>${escapeHtml(pageState.selectedModel)}</strong></div>` : ''}
        ${ready ? '' : `<p class="s2-guide-config-note" role="status">${escapeHtml(unavailable || '请先在「选择模型」中读取并选择当前密钥的可用模型。')}</p>`}
        <div class="s2-panel-actions"><button class="s2-btn s2-btn-primary" type="button" data-guide-app-action="${escapeHtml(pageState.selectedApp)}" ${ready ? '' : 'disabled'}>${icon(pageState.selectedApp === 'ccswitch' ? 'external' : 'copy', 15)}${escapeHtml(selected.action)}</button></div>
      </div>`;
  }

  function guideApiPanel() {
    const protocols = protocolDefinitions();
    const selected = protocols[pageState.selectedProtocol] || protocols.openai;
    const unavailable = guideProtocolUnavailable(pageState.selectedProtocol);
    const tabs = Object.entries(protocols)
      .map(
        ([key, protocol]) => `<button class="${key === pageState.selectedProtocol ? 'is-active' : ''}" type="button" data-guide-protocol="${key}">${escapeHtml(protocol.name)}</button>`,
      )
      .join('');
    return `
      <div class="s2-protocol-tabs">${tabs}</div>
      <div class="s2-mapping"><span>接口路径</span><strong>${escapeHtml(selected.path)}</strong></div>
      <div class="s2-mapping"><span>认证方式</span><strong>${escapeHtml(selected.auth)}</strong></div>
      ${unavailable ? `<div class="s2-guide-error" role="status">${escapeHtml(unavailable)}</div>` : `<div class="s2-code-toolbar"><span>PowerShell 7 · curl.exe</span><small>将 YOUR_API_KEY 替换为当前密钥</small></div><pre class="s2-code"><code>${escapeHtml(selected.code)}</code></pre>`}
      <div class="s2-panel-actions"><button class="s2-btn" type="button" data-guide-api-copy data-copy-text="${unavailable ? '' : escapeHtml(selected.code)}" ${unavailable ? 'disabled' : ''}>${icon('copy', 15)}复制代码</button><button class="s2-btn s2-btn-primary" type="button" data-test-connection ${unavailable ? 'disabled' : ''}>${icon('play', 15)}测试连接</button></div>`;
  }

  function guideStepTwo(apiKey) {
    return `
      <div class="s2-section-head"><div><h2>选择接入方式</h2><p>优先使用应用一键导入；需要自行开发时再使用 API 或 SDK。</p></div><span class="s2-badge">步骤 2 / 4</span></div>
      <div class="s2-mode-tabs">
        <button class="${pageState.integrationMode === 'apps' ? 'is-active' : ''}" type="button" data-guide-mode="apps">${icon('model', 15)}应用对接</button>
        <button class="${pageState.integrationMode === 'api' ? 'is-active' : ''}" type="button" data-guide-mode="api">${icon('code', 15)}API / SDK</button>
      </div>
      ${pageState.integrationMode === 'apps' ? guideAppsPanel(apiKey) : guideApiPanel()}
      <div class="s2-panel-actions"><button class="s2-btn" type="button" data-guide-step="1">${icon('back', 14)}上一步</button><button class="s2-btn s2-btn-next" type="button" data-guide-step="3">下一步${icon('arrow', 15)}</button></div>`;
  }

  function guideStepThree() {
    const apiKey = selectedGuideKey();
    const group = selectedGuideGroup(apiKey);
    const allModels = recommendedModelsForKey(apiKey);
    const filter = pageState.guideModelProtocol;
    const models = allModels.filter((model) => filter === 'all' || model.platform === (filter === 'responses' ? 'openai' : filter));
    const modelTabs = [
      ['all', '全部'], ['openai', 'OpenAI'], ['responses', 'Responses'], ['anthropic', 'Anthropic'], ['gemini', 'Gemini'],
    ].filter(([value]) => value === 'all' || allModels.some((model) => model.platform === (value === 'responses' ? 'openai' : value)))
      .map(([value, label]) => `<button type="button" class="${value === filter ? 'is-active' : ''}" data-guide-model-protocol="${value}" aria-pressed="${value === filter}">${label}</button>`).join('');
    const cards = models
      .map(
        (model, index) => `
          <button class="s2-model-option ${pageState.selectedModel === model.name ? 'is-active' : ''}" type="button" data-guide-model="${escapeHtml(model.name)}" aria-pressed="${pageState.selectedModel === model.name}">
            <span class="s2-model-copy"><strong>${escapeHtml(model.name)}${index === 0 ? '<span class="s2-model-tag">推荐</span>' : ''}</strong><small>${escapeHtml(platformLabel(model.platform))} · ${escapeHtml(model.type || 'text')}</small><span class="s2-model-price">${escapeHtml(modelPriceText(model))}</span></span>
            <span class="s2-model-check">${pageState.selectedModel === model.name ? icon('check', 10) : ''}</span>
          </button>`,
      )
      .join('');
    const loading = pageState.guideModelsLoading;
    const error = pageState.guideModelsError;
    const content = loading
      ? `<div class="s2-guide-model-state" role="status">${icon('refresh', 22)}<strong>正在读取可用模型</strong><p>根据当前密钥查询分组允许的模型。</p></div>`
      : error
        ? `<div class="s2-guide-model-state" role="alert"><strong>暂时无法读取模型</strong><p>${escapeHtml(error)}</p><button type="button" class="s2-btn" data-guide-models-retry>${icon('refresh', 14)}重新获取</button></div>`
        : cards
          ? `<div class="s2-protocol-tabs" aria-label="筛选模型协议">${modelTabs}</div><div class="s2-model-grid">${cards}</div>`
          : `<div class="s2-guide-model-state"><strong>${apiKey ? '当前分组暂无可用模型' : '请先创建 API 密钥'}</strong><p>${apiKey ? '请联系管理员确认分组模型配置，或切换其他密钥。' : '创建密钥后即可查询对应分组的模型。'}</p><button type="button" class="s2-btn" data-route="/keys">${icon('key', 14)}管理密钥</button></div>`;
    return `
      <div class="s2-section-head"><div><h2>选择可用模型</h2><p>读取当前 API Key 的模型列表，点击模型即可复制 ID。</p></div><span class="s2-badge">步骤 3 / 4</span></div>
      <div class="s2-model-recommendation"><span>当前密钥分组</span><strong>${escapeHtml(group.name)} · ${escapeHtml(platformLabel(group.platform))}</strong></div>
      <div class="s2-guide-model-key">${guideKeySelectHtml(apiKey)}</div>
      ${content}
      <div class="s2-note">${icon('shield', 16)}<span>参考价来自站点展示配置，实际费用以分组计费为准。接口返回 403 时，请确认当前密钥与模型的访问权限。</span></div>
      <div class="s2-panel-actions"><button class="s2-btn" type="button" data-guide-step="2">${icon('back', 14)}上一步</button><button class="s2-btn s2-btn-primary" type="button" data-guide-step="4" ${pageState.selectedModel && !loading && !error ? '' : 'disabled'}>完成接入${icon('arrow', 15)}</button></div>`;
  }

  function guideStepFour() {
    return `
      <div class="s2-success">
        <div>
          <span class="s2-success-mark">${icon('check', 24)}</span>
          <span class="s2-badge">步骤 4 / 4</span>
          <h2>接入准备完成</h2>
          <p>${pageState.selectedModel ? `已选择 ${escapeHtml(pageState.selectedModel)}。` : ''}将配置应用到客户端并发起首次请求后，Token、消费和响应时间将同步到仪表盘。</p>
          <div class="s2-success-summary"><span>当前密钥</span><strong>${escapeHtml(selectedGuideKey()?.name || '尚未选择')}</strong><span>所选模型</span><strong>${escapeHtml(pageState.selectedModel || '尚未选择')}</strong></div>
          <div class="s2-panel-actions"><button class="s2-btn s2-btn-primary" type="button" data-guide-step="2">${icon('copy', 15)}复制或导入配置</button><button class="s2-btn" type="button" data-route="/usage">${icon('activity', 15)}查看使用记录</button><button class="s2-btn s2-btn-quiet" type="button" data-overlay-view="dashboard">返回仪表盘</button></div>
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
    const illustration = `<svg class="s2-guide-illustration" viewBox="0 0 280 190" fill="none" aria-hidden="true">
      <defs><linearGradient id="${PAGE_ROOT_ID}-guide-top" x1="65" y1="20" x2="231" y2="123" gradientUnits="userSpaceOnUse"><stop stop-color="#f9fcff"/><stop offset="1" stop-color="#9ec5ff"/></linearGradient><linearGradient id="${PAGE_ROOT_ID}-guide-side" x1="129" y1="44" x2="180" y2="178" gradientUnits="userSpaceOnUse"><stop stop-color="#c8dcff"/><stop offset="1" stop-color="#7d9cf7" stop-opacity=".5"/></linearGradient><linearGradient id="${PAGE_ROOT_ID}-guide-front" x1="56" y1="68" x2="148" y2="157" gradientUnits="userSpaceOnUse"><stop stop-color="white" stop-opacity=".98"/><stop offset="1" stop-color="#d7e3ff" stop-opacity=".8"/></linearGradient><filter id="${PAGE_ROOT_ID}-guide-shadow"><feGaussianBlur stdDeviation="9"/></filter></defs>
      <ellipse cx="151" cy="165" rx="95" ry="11" fill="#4d81ee" opacity=".16" filter="url(#${PAGE_ROOT_ID}-guide-shadow)"/>
      <path d="m161 19 37-18 38 20-37 20Z" fill="url(#${PAGE_ROOT_ID}-guide-top)" stroke="white" stroke-opacity=".8"/>
      <path d="m161 19 38 22v17l-38-21Z" fill="#b7d1ff"/><path d="m199 41 37-20v17l-37 21Z" fill="#9fc1fc"/>
      <path d="m157 72 56-31 45 27-57 32Z" fill="url(#${PAGE_ROOT_ID}-guide-top)" stroke="white" stroke-opacity=".8"/>
      <path d="m201 100 57-32v66l-57 32Z" fill="url(#${PAGE_ROOT_ID}-guide-side)"/><path d="m157 72 44 28v66l-44-26Z" fill="#e5eeff"/>
      <path d="m53 64 72-39 71 41-71 40Z" fill="url(#${PAGE_ROOT_ID}-guide-top)" stroke="white" stroke-opacity=".8"/>
      <path d="m125 106 71-40v70l-71 41Z" fill="url(#${PAGE_ROOT_ID}-guide-side)" stroke="white" stroke-opacity=".55"/>
      <path d="m53 64 72 42v71l-72-42Z" fill="url(#${PAGE_ROOT_ID}-guide-front)" stroke="white" stroke-opacity=".9"/>
      <path d="M91 92c-10-5-18 0-18 10 0 8 4 14 10 18v19l11 7v-7l6 4v-8l-6-4v-6c9 2 14-3 14-11 0-9-7-18-17-22Z" fill="white"/><ellipse cx="91" cy="106" rx="5" ry="7" transform="rotate(-26 91 106)" fill="#b7b7f9"/>
      <path d="m210 94 28-16m-28 29 20-12m-20 25 25-14" stroke="white" stroke-width="3" stroke-linecap="round" opacity=".8"/>
      <path d="m32 103 17-9 17 10-17 10Zm0 0v18l17 10v-17m0 17 17-9v-18" fill="#e8f1ff" stroke="white" opacity=".8"/>
    </svg>`;
    return `
      <section class="s2-page s2-guide-page" data-sub2api-page="guide">
        <div class="s2-guide-breadcrumb">${icon('external', 15)}<button class="s2-btn s2-btn-ghost" type="button" data-overlay-view="dashboard">仪表盘</button><span>/</span><span>接入指南</span></div>
        <div class="s2-guide-layout">
          <aside class="s2-steps" aria-label="接入步骤">
            <div class="s2-steps-list">${guideStepNavigation()}</div>
            <div class="s2-guide-support"><strong>${icon('shield', 15)}遇到问题？</strong><p>携带请求 ID 联系技术支持，可以更快定位问题。</p></div>
          </aside>
          <div class="s2-guide-main">
            ${pageState.guideStep === 1 ? `<div class="s2-guide-banner"><div class="s2-guide-banner-copy"><div class="s2-eyebrow">${icon('sparkles', 14)}开始接入</div><h1>5 分钟接入你的第一个模型</h1><p>一个 API Key，即可调用平台支持的 OpenAI、Claude 与 Gemini 模型。跟随四个步骤完成首次请求。</p></div>${illustration}</div>` : ''}
            ${pageState.guideSettingsError ? `<div class="s2-guide-error" role="alert"><span>${escapeHtml(pageState.guideSettingsError)}</span><button type="button" class="s2-btn" data-guide-models-retry>${icon('refresh', 14)}重新获取</button></div>` : ''}
            <article class="s2-guide-panel">${panel}</article>
          </div>
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
    const unavailable = guideProtocolUnavailable(pageState.selectedProtocol, apiKey);
    if (unavailable) {
      showPageToast(unavailable);
      return;
    }
    if (!apiKey?.key || apiKey.status !== 'active') {
      showPageToast('请先创建或选择有效的 API Key');
      return;
    }
    const version = pageRequestVersion;
    const viewVersion = pageViewVersion;
    const connectionVersion = ++connectionRequestVersion;
    try {
      const response = await fetchWithTimeout(guideModelsEndpoint(apiKey), {
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
      '[data-overlay-view], [data-route], [data-dashboard-days], [data-dashboard-retry], [data-guide-step], [data-guide-mode], [data-guide-app], [data-guide-app-action], [data-guide-protocol], [data-guide-model], [data-guide-model-protocol], [data-guide-models-retry], [data-guide-key-toggle], [data-guide-key-option], [data-copy-text], [data-copy-secret], [data-test-connection]',
    );
    if (!control || !root.contains(control)) {
      if (pageState.keyMenuOpen) {
        pageState.keyMenuOpen = false;
        rerenderGuide();
      }
      return;
    }

    if (control.hasAttribute('data-dashboard-retry')) {
      loadDashboardIntoRoot();
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
      void loadGuideModels();
      return;
    }
    if (control.hasAttribute('data-guide-models-retry')) {
      if (pageState.guideSettingsError) void loadGuideInitialData();
      else void loadGuideModels();
      return;
    }
    if (control.dataset.guideModelProtocol) {
      pageState.guideModelProtocol = control.dataset.guideModelProtocol;
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
      root.scrollIntoView?.({ block: 'start', behavior: 'auto' });
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
      if (!recommendedModelsForKey(selectedGuideKey()).some((model) => model.name === control.dataset.guideModel)) return;
      pageState.selectedModel = control.dataset.guideModel;
      syncGuideProtocol();
      rerenderGuide();
      await copyWithToast(control.dataset.guideModel, '模型 ID 已复制', '请选择并复制模型 ID');
      return;
    }
    if (control.hasAttribute('data-copy-secret')) {
      await copyWithToast(selectedGuideKey()?.key || '', 'API Key 已复制', '暂无可复制的 API Key');
      return;
    }
    if (control.dataset.copyText) {
      if (control.hasAttribute('data-guide-api-copy') && guideProtocolUnavailable(pageState.selectedProtocol)) return;
      await copyWithToast(control.dataset.copyText, '内容已复制', '复制失败，请手动选择文本');
      return;
    }
    if (control.hasAttribute('data-test-connection')) {
      await testGuideConnection();
      return;
    }
    if (control.dataset.guideAppAction) {
      const key = selectedGuideKey();
      if (!key?.key || key.status !== 'active') {
        showPageToast('请先创建或选择有效的 API Key');
        return;
      }
      if (!hasVerifiedGuideModel()) {
        showPageToast('请先读取并选择当前密钥的可用模型');
        return;
      }
      const unavailable = guideAppUnavailable(control.dataset.guideAppAction, key);
      if (unavailable) {
        showPageToast(unavailable);
        return;
      }
      if (control.dataset.guideAppAction === 'ccswitch') {
        const deepLink = buildCcSwitchImportUrl(key, pageState.settings || {}, guideClientType(key));
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
    void loadGuideModels();
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
      void loadGuideInitialData();
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
    root.removeEventListener('pointerover', handleTrendPoint);
    root.removeEventListener('mouseover', handleTrendPoint);
    root.removeEventListener('focusin', handleTrendPoint);
    root.remove();
    root.replaceChildren();
    pageStyle?.remove();
    pageStyle = null;
    pageState.settings = null;
    pageState.keys = [];
    pageState.models = [];
    pageState.modelCatalog = [];
    pageState.availableModels = [];
    guideModelsController = null;
    pageState.selectedKeyId = '';
    pageState.selectedModel = '';
  }

  root.addEventListener('click', handlePageClick);
  root.addEventListener('change', handlePageChange);
  root.addEventListener('keydown', handlePageKeydown);
  root.addEventListener('pointerover', handleTrendPoint);
  root.addEventListener('mouseover', handleTrendPoint);
  root.addEventListener('focusin', handleTrendPoint);
  return { render, destroy };
}
