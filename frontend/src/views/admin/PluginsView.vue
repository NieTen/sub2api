<template>
  <AppLayout>
    <div class="space-y-6">
      <section
        class="flex flex-col gap-4 border-b border-gray-200 pb-5 dark:border-dark-700 sm:flex-row sm:items-end sm:justify-between"
      >
        <div class="min-w-0">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t("admin.plugins.title") }}
          </h2>
          <p class="mt-1 max-w-3xl text-sm text-gray-500 dark:text-gray-400">
            {{ t("admin.plugins.description") }}
          </p>
          <div
            class="mt-3 flex flex-wrap gap-2 text-xs text-gray-600 dark:text-gray-300"
          >
            <span class="rounded bg-gray-100 px-2 py-1 dark:bg-dark-700">{{
              t("admin.plugins.onlyOpenAI")
            }}</span>
            <span class="rounded bg-gray-100 px-2 py-1 dark:bg-dark-700">{{
              t("admin.plugins.noAccountCoupling")
            }}</span>
          </div>
        </div>

        <div class="flex flex-shrink-0 items-center gap-2">
          <input
            ref="fileInput"
            class="hidden"
            type="file"
            accept=".s2plugin,application/zip"
            @change="handleFileSelected"
          />
          <button
            type="button"
            class="btn btn-primary"
            :disabled="uploading"
            @click="fileInput?.click()"
          >
            <Icon name="upload" size="sm" />
            {{ uploading ? t("common.processing") : t("admin.plugins.upload") }}
          </button>
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="loading"
            :title="t('common.refresh')"
            @click="loadPlugins"
          >
            <Icon name="refresh" size="sm" />
            <span class="sr-only">{{ t("common.refresh") }}</span>
          </button>
        </div>
      </section>

      <p class="text-xs text-gray-500 dark:text-gray-400">
        {{ t("admin.plugins.uploadHint") }}
      </p>

      <div
        class="border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800 dark:border-blue-900/60 dark:bg-blue-950/30 dark:text-blue-200"
      >
        <p>{{ t("admin.plugins.runtimeNotice") }}</p>
        <p class="mt-1">{{ t("admin.plugins.menuNotice") }}</p>
      </div>

      <div
        v-if="loading"
        class="flex min-h-48 items-center justify-center text-sm text-gray-500"
      >
        {{ t("common.loading") }}
      </div>

      <div
        v-else-if="plugins.length === 0"
        class="flex min-h-56 flex-col items-center justify-center border border-dashed border-gray-300 px-6 text-center dark:border-dark-600"
      >
        <Icon name="cube" size="xl" class="text-gray-400" />
        <p class="mt-3 font-medium text-gray-800 dark:text-gray-200">
          {{ t("admin.plugins.empty") }}
        </p>
        <p class="mt-1 max-w-lg text-sm text-gray-500 dark:text-gray-400">
          {{ t("admin.plugins.emptyHint") }}
        </p>
      </div>

      <div v-else class="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <article
          v-for="plugin in plugins"
          :key="plugin.id"
          class="card overflow-hidden border border-gray-200 dark:border-dark-700"
        >
          <div
            class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-100 p-5 dark:border-dark-700"
          >
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h3
                  class="truncate text-base font-semibold text-gray-900 dark:text-white"
                >
                  {{ plugin.name }}
                </h3>
                <span class="font-mono text-xs text-gray-500"
                  >v{{ plugin.version }}</span
                >
                <span
                  class="rounded px-2 py-0.5 text-xs font-medium"
                  :class="stateClass(plugin.state)"
                >
                  {{ t(`admin.plugins.${plugin.state}`) }}
                </span>
              </div>
              <p class="mt-1 text-xs text-gray-500">
                {{ plugin.plugin_key
                }}<span v-if="plugin.author"> · {{ plugin.author }}</span>
              </p>
              <p
                v-if="plugin.description"
                class="mt-2 text-sm text-gray-600 dark:text-gray-300"
              >
                {{ plugin.description }}
              </p>
            </div>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              @click="openConfiguration(plugin)"
            >
              <Icon name="cog" size="sm" />
              {{ t("admin.plugins.configure") }}
            </button>
          </div>

          <div class="grid grid-cols-1 gap-x-6 gap-y-4 p-5 md:grid-cols-2">
            <div>
              <p class="text-xs font-medium uppercase text-gray-500">
                {{ t("admin.plugins.compatibility") }}
              </p>
              <div class="mt-2 flex items-center gap-2">
                <span
                  class="rounded px-2 py-0.5 text-xs font-medium"
                  :class="compatibilityClass(plugin.compatibility.status)"
                >
                  {{ t(`admin.plugins.${plugin.compatibility.status}`) }}
                </span>
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  plugin.compatibility.message
                }}</span>
              </div>
              <dl
                class="mt-3 grid grid-cols-[auto,1fr] gap-x-3 gap-y-1 text-xs"
              >
                <dt class="text-gray-500">
                  {{ t("admin.plugins.currentVersion") }}
                </dt>
                <dd class="font-mono text-gray-800 dark:text-gray-200">
                  {{ plugin.compatibility.current_sub2api_version }}
                </dd>
                <dt class="text-gray-500">
                  {{ t("admin.plugins.requiredVersion") }}
                </dt>
                <dd class="font-mono text-gray-800 dark:text-gray-200">
                  {{ plugin.compatibility.required_sub2api_version }}
                </dd>
                <dt class="text-gray-500">
                  {{ t("admin.plugins.recommendedVersion") }}
                </dt>
                <dd class="font-mono text-gray-800 dark:text-gray-200">
                  {{ plugin.compatibility.recommended_sub2api_version || "-" }}
                </dd>
              </dl>
            </div>

            <div>
              <p class="text-xs font-medium uppercase text-gray-500">
                {{ t("admin.plugins.runtime") }}
              </p>
              <div class="mt-2 flex flex-wrap gap-2 text-xs">
                <span
                  class="rounded px-2 py-0.5"
                  :class="
                    plugin.runtime_healthy
                      ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
                      : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
                  "
                >
                  {{
                    plugin.runtime_healthy
                      ? t("admin.plugins.healthy")
                      : t("admin.plugins.unhealthy")
                  }}
                </span>
                <span
                  class="rounded bg-gray-100 px-2 py-0.5 text-gray-600 dark:bg-dark-700 dark:text-gray-300"
                >
                  {{ t("admin.plugins.signature") }}:
                  {{ t(`admin.plugins.${plugin.signature_status}`) }}
                </span>
              </div>
              <p
                v-if="plugin.last_error"
                class="mt-3 break-words text-xs text-red-600 dark:text-red-400"
              >
                {{ plugin.last_error }}
              </p>
              <p
                v-else-if="plugin.runtime_message"
                class="mt-3 break-words text-xs text-gray-500"
              >
                {{ plugin.runtime_message }}
              </p>
            </div>

            <div
              v-if="supportsHostAdaptation(plugin)"
              class="rounded-lg border border-gray-200 px-3 py-3 dark:border-dark-600 md:col-span-2"
            >
              <div class="flex items-center justify-between gap-4">
                <span class="text-sm font-medium text-gray-800 dark:text-gray-200">
                  {{ t("admin.plugins.hostAdaptation") }}
                </span>
                <button
                  type="button"
                  role="switch"
                  :aria-checked="plugin.host_adaptation_enabled === true"
                  :aria-label="t('admin.plugins.hostAdaptation')"
                  :aria-describedby="`host-adaptation-hint-${plugin.id}`"
                  :disabled="busyID === plugin.id || plugin.state === 'starting'"
                  class="relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-600 disabled:cursor-wait disabled:opacity-50"
                  :class="plugin.host_adaptation_enabled ? 'bg-primary-600' : 'bg-gray-300 dark:bg-dark-500'"
                  @click="toggleHostAdaptation(plugin)"
                >
                  <span
                    class="inline-block h-4 w-4 rounded-full bg-white transition-transform"
                    :class="plugin.host_adaptation_enabled ? 'translate-x-6' : 'translate-x-1'"
                  />
                </button>
              </div>
              <p :id="`host-adaptation-hint-${plugin.id}`" class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400">
                {{ t("admin.plugins.hostAdaptationHint") }}
              </p>
            </div>

            <div class="md:col-span-2">
              <label
                :for="`plugin-rollout-${plugin.id}`"
                class="flex items-center justify-between gap-4 text-xs font-medium text-gray-600 dark:text-gray-300"
              >
                <span>{{ t("admin.plugins.rollout") }}</span>
                <span class="w-11 text-right font-mono"
                  >{{
                    rolloutValues[plugin.id] ?? currentRollout(plugin)
                  }}%</span
                >
              </label>
              <input
                :id="`plugin-rollout-${plugin.id}`"
                :value="rolloutValues[plugin.id] ?? currentRollout(plugin)"
                type="range"
                min="1"
                max="100"
                step="1"
                class="mt-2 w-full accent-primary-600"
                :disabled="hasEnabledBinding(plugin)"
                @input="setRollout(plugin.id, $event)"
              />
            </div>
          </div>

          <div
            class="flex flex-wrap justify-end gap-2 border-t border-gray-100 px-5 py-4 dark:border-dark-700"
          >
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="busyID === plugin.id"
              @click="testPlugin(plugin)"
            >
              <Icon name="beaker" size="sm" />
              {{ t("admin.plugins.test") }}
            </button>
            <button
              v-if="hasEnabledBinding(plugin)"
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="busyID === plugin.id"
              @click="disablePlugin(plugin)"
            >
              <Icon name="ban" size="sm" />
              {{ t("admin.plugins.disable") }}
            </button>
            <button
              v-else
              type="button"
              class="btn btn-primary btn-sm"
              :disabled="
                busyID === plugin.id ||
                plugin.state === 'starting' ||
                !plugin.compatibility.compatible
              "
              @click="enablePlugin(plugin)"
            >
              <Icon name="play" size="sm" />
              {{ t("admin.plugins.enable") }}
            </button>
            <button
              type="button"
              class="btn btn-danger btn-sm"
              :disabled="busyID === plugin.id || hasEnabledBinding(plugin)"
              @click="uninstallPlugin(plugin)"
            >
              <Icon name="trash" size="sm" />
              {{ t("admin.plugins.uninstall") }}
            </button>
          </div>
        </article>
      </div>

      <BaseDialog
        :show="configPlugin !== null"
        :title="
          t('admin.plugins.configTitle', { name: configPlugin?.name || '' })
        "
        width="full"
        @close="closeConfiguration"
      >
        <div
          class="relative min-h-[520px] overflow-hidden bg-gray-50 dark:bg-dark-900"
          :style="{ height: `${iframeHeight}px` }"
        >
          <div
            v-if="uiLoading"
            class="absolute inset-0 z-10 flex items-center justify-center text-sm text-gray-500"
          >
            {{ t("admin.plugins.loadingUI") }}
          </div>
          <div
            v-if="uiError"
            class="absolute inset-0 z-20 flex flex-col items-center justify-center p-8 text-center"
          >
            <Icon name="exclamationTriangle" size="xl" class="text-amber-500" />
            <p class="mt-3 font-medium text-gray-800 dark:text-gray-200">
              {{ t("admin.plugins.uiUnavailable") }}
            </p>
            <p class="mt-1 max-w-xl text-sm text-gray-500">{{ uiError }}</p>
          </div>
          <iframe
            v-if="uiSession"
            :key="uiSession.url"
            ref="pluginFrame"
            :src="uiSession.url"
            sandbox="allow-scripts"
            referrerpolicy="no-referrer"
            class="h-full w-full border-0 bg-white dark:bg-dark-900"
            :title="
              t('admin.plugins.configTitle', { name: configPlugin?.name || '' })
            "
            @load="handlePluginFrameLoad"
          />
        </div>
      </BaseDialog>

      <TotpStepUpDialog :controller="pluginStepUp" />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import {
  adminAPI,
  type PluginInstallation,
  type PluginUISession,
} from "@/api/admin";
import { useAppStore } from "@/stores";
import AppLayout from "@/components/layout/AppLayout.vue";
import BaseDialog from "@/components/common/BaseDialog.vue";
import Icon from "@/components/icons/Icon.vue";
import TotpStepUpDialog from "@/components/auth/TotpStepUpDialog.vue";
import {
  isStepUpBlocked,
  isStepUpCancelled,
  stepUpBlockReason,
  useStepUp,
} from "@/composables/useStepUp";

interface PluginBridgeMessage {
  source?: string;
  bridge_token?: string;
  type?: string;
  request_id?: string;
  config?: unknown;
  action?: unknown;
  height?: unknown;
  level?: unknown;
  message?: unknown;
}

interface PluginBridgeContext {
  pluginID: number;
  generation: number;
  session: PluginUISession;
  frame: Window;
}

interface PluginBridgeRequest {
  context: PluginBridgeContext;
  message: PluginBridgeMessage;
  requestID: string;
  timeout: number;
}

const { t } = useI18n();
const appStore = useAppStore();
const pluginStepUp = useStepUp();
const plugins = ref<PluginInstallation[]>([]);
const loading = ref(false);
const uploading = ref(false);
const busyID = ref<number | null>(null);
const fileInput = ref<HTMLInputElement | null>(null);
const rolloutValues = ref<Record<number, number>>({});
const configPlugin = ref<PluginInstallation | null>(null);
const uiSession = ref<PluginUISession | null>(null);
const pluginFrame = ref<HTMLIFrameElement | null>(null);
const uiLoading = ref(false);
const uiError = ref("");
const iframeHeight = ref(640);
const pluginFrameLoaded = ref(false);
const pendingBridgeRequests = new Map<string, PluginBridgeRequest>();
const completedBridgeRequests = new Map<string, { type: string; payload: Record<string, unknown> }>();
let configurationGeneration = 0;

function errorMessage(error: unknown): string {
  if (typeof error === "object" && error !== null && "message" in error) {
    return String(
      (error as { message?: unknown }).message || t("common.unknownError"),
    );
  }
  return t("common.unknownError");
}

function reportSensitiveActionError(error: unknown): void {
  if (isStepUpCancelled(error)) return;
  if (isStepUpBlocked(error)) {
    appStore.showError(
      stepUpBlockReason(error) === "STEP_UP_ADMIN_API_KEY_FORBIDDEN"
        ? t("stepUp.adminApiKeyForbidden")
        : t("stepUp.notEnabled"),
    );
    return;
  }
  appStore.showError(errorMessage(error));
}

async function loadPlugins(): Promise<void> {
  loading.value = true;
  try {
    plugins.value = await adminAPI.plugins.list();
    if (configPlugin.value) {
      const refreshed = plugins.value.find((plugin) => plugin.id === configPlugin.value?.id);
      if (!refreshed || refreshed.host_adaptation_enabled !== configPlugin.value.host_adaptation_enabled) closeConfiguration();
      else configPlugin.value = refreshed;
    }
    for (const plugin of plugins.value) {
      rolloutValues.value[plugin.id] = currentRollout(plugin);
    }
  } catch (error: unknown) {
    appStore.showError(errorMessage(error));
  } finally {
    loading.value = false;
  }
}

async function handleFileSelected(event: Event): Promise<void> {
  const target = event.target as HTMLInputElement;
  const file = target.files?.[0];
  target.value = "";
  if (!file || !file.name.toLowerCase().endsWith(".s2plugin")) {
    appStore.showError(t("admin.plugins.fileRequired"));
    return;
  }
  uploading.value = true;
  try {
    await pluginStepUp.run(() => adminAPI.plugins.upload(file));
    appStore.showSuccess(t("admin.plugins.uploadSuccess"));
    await loadPlugins();
  } catch (error: unknown) {
    reportSensitiveActionError(error);
  } finally {
    uploading.value = false;
  }
}

function currentRollout(plugin: PluginInstallation): number {
  return (
    plugin.bindings.find(
      (binding) => binding.capability === "openai.oauth.outbound_transport.v1",
    )?.rollout_percent || 100
  );
}

function hasEnabledBinding(plugin: PluginInstallation): boolean {
  return plugin.bindings.some((binding) => binding.enabled);
}

function supportsHostAdaptation(plugin: PluginInstallation): boolean {
  return plugin.manifest.capabilities.some(
    (capability) => capability.id === "openai.oauth.outbound_transport.v1",
  );
}

async function toggleHostAdaptation(plugin: PluginInstallation): Promise<void> {
  if (busyID.value === plugin.id) return;
  busyID.value = plugin.id;
  try {
    const updated = await pluginStepUp.run(() =>
      adminAPI.plugins.setHostAdaptation(plugin.id, !plugin.host_adaptation_enabled),
    );
    plugins.value = plugins.value.map((item) => item.id === updated.id ? updated : item);
    // 适配切换会重建运行上下文；已打开的配置页必须重新获取相应的沙箱权限。
    if (configPlugin.value?.id === plugin.id) closeConfiguration();
    if (hasEnabledBinding(updated) && !updated.runtime_healthy && updated.runtime_message) {
      appStore.showError(updated.runtime_message);
    } else {
      appStore.showSuccess(t(updated.host_adaptation_enabled
        ? "admin.plugins.hostAdaptationEnabled"
        : "admin.plugins.hostAdaptationDisabled"));
    }
  } catch (error: unknown) {
    reportSensitiveActionError(error);
  } finally {
    busyID.value = null;
  }
}

function setRollout(id: number, event: Event): void {
  const value = Number((event.target as HTMLInputElement).value);
  rolloutValues.value[id] = Math.min(100, Math.max(1, value));
}

async function enablePlugin(plugin: PluginInstallation): Promise<void> {
  let acceptUntested = false;
  if (!plugin.compatibility.tested) {
    acceptUntested = window.confirm(t("admin.plugins.confirmUntested"));
    if (!acceptUntested) return;
  }
  busyID.value = plugin.id;
  try {
    await pluginStepUp.run(() =>
      adminAPI.plugins.enable(
        plugin.id,
        rolloutValues.value[plugin.id] || 100,
        acceptUntested,
      ),
    );
    appStore.showSuccess(t("admin.plugins.enableSuccess"));
    await loadPlugins();
  } catch (error: unknown) {
    reportSensitiveActionError(error);
  } finally {
    busyID.value = null;
  }
}

async function disablePlugin(plugin: PluginInstallation): Promise<void> {
  if (!window.confirm(t("admin.plugins.confirmDisable"))) return;
  busyID.value = plugin.id;
  try {
    await pluginStepUp.run(() => adminAPI.plugins.disable(plugin.id));
    appStore.showSuccess(t("admin.plugins.disableSuccess"));
    await loadPlugins();
  } catch (error: unknown) {
    reportSensitiveActionError(error);
  } finally {
    busyID.value = null;
  }
}

async function uninstallPlugin(plugin: PluginInstallation): Promise<void> {
  if (!window.confirm(t("admin.plugins.confirmUninstall"))) return;
  busyID.value = plugin.id;
  try {
    await pluginStepUp.run(() => adminAPI.plugins.remove(plugin.id));
    appStore.showSuccess(t("admin.plugins.uninstallSuccess"));
    await loadPlugins();
  } catch (error: unknown) {
    reportSensitiveActionError(error);
  } finally {
    busyID.value = null;
  }
}

async function testPlugin(plugin: PluginInstallation): Promise<void> {
  busyID.value = plugin.id;
  try {
    const result = await pluginStepUp.run(() =>
      adminAPI.plugins.test(plugin.id),
    );
    if (result.success)
      appStore.showSuccess(result.message || t("admin.plugins.testSuccess"));
    else appStore.showError(result.message || t("common.error"));
  } catch (error: unknown) {
    reportSensitiveActionError(error);
  } finally {
    busyID.value = null;
  }
}

async function openConfiguration(plugin: PluginInstallation): Promise<void> {
  invalidateBridgeSession();
  const generation = configurationGeneration;
  configPlugin.value = plugin;
  uiSession.value = null;
  pluginFrameLoaded.value = false;
  uiLoading.value = true;
  uiError.value = "";
  iframeHeight.value = 640;
  try {
    const session = await adminAPI.plugins.createUISession(plugin.id);
    if (generation === configurationGeneration) uiSession.value = session;
  } catch (error: unknown) {
    if (generation !== configurationGeneration) return;
    uiLoading.value = false;
    uiError.value = errorMessage(error);
  }
}

function closeConfiguration(): void {
  invalidateBridgeSession();
  pluginFrameLoaded.value = false;
  configPlugin.value = null;
  uiSession.value = null;
  uiLoading.value = false;
  uiError.value = "";
}

function invalidateBridgeSession(): void {
  configurationGeneration += 1;
  for (const request of pendingBridgeRequests.values()) window.clearTimeout(request.timeout);
  pendingBridgeRequests.clear();
  completedBridgeRequests.clear();
}

function handlePluginFrameLoad(event: Event): void {
  if (event.target !== pluginFrame.value) return;
  // 插件自行导航也会触发加载；使旧文档的请求失效，避免迟到响应和二次验证重试穿透。
  if (pluginFrameLoaded.value) invalidateBridgeSession();
  pluginFrameLoaded.value = true;
  uiLoading.value = false;
}

function isCurrentBridge(context: PluginBridgeContext): boolean {
  return context.generation === configurationGeneration &&
    context.pluginID === configPlugin.value?.id &&
    context.session === uiSession.value &&
    context.frame === pluginFrame.value?.contentWindow;
}

function requireCurrentBridge(context: PluginBridgeContext, adaptation = false): void {
  if (!isCurrentBridge(context)) throw new Error(t("admin.plugins.bridgeRejected"));
  if (adaptation && (!configPlugin.value?.host_adaptation_enabled || !supportsHostAdaptation(configPlugin.value))) {
    throw new Error(t("admin.plugins.hostAdaptationRequired"));
  }
}

function isBridgeObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" &&
    Object.prototype.toString.call(value) === "[object Object]";
}

function requirePendingBridge(request: PluginBridgeRequest, adaptation = false): void {
  requireCurrentBridge(request.context, adaptation);
  if (pendingBridgeRequests.get(request.requestID) !== request) {
    throw new Error(t("admin.plugins.bridgeExpired"));
  }
}

function sendBridgePayload(
  context: PluginBridgeContext,
  type: string,
  requestID: string,
  payload: Record<string, unknown>,
): void {
  if (!isCurrentBridge(context)) return;
  context.frame.postMessage(
    {
      source: "sub2api-plugin-host",
      bridge_token: context.session.bridge_token,
      type: `${type}.result`,
      request_id: requestID,
      ...payload,
    },
    // 沙箱具有不透明来源，只能用星号；窗口、令牌和会话代次共同限定接收文档。
    "*",
  );
}

function postBridgeResult(request: PluginBridgeRequest, payload: Record<string, unknown>): boolean {
  if (!isCurrentBridge(request.context) || pendingBridgeRequests.get(request.requestID) !== request) return false;
  window.clearTimeout(request.timeout);
  pendingBridgeRequests.delete(request.requestID);
  completedBridgeRequests.set(request.requestID, { type: request.message.type!, payload });
  // 缓存近期响应供重复消息重放，避免再次执行动作，同时限制长期打开页面的内存占用。
  if (completedBridgeRequests.size > 256) {
    completedBridgeRequests.delete(completedBridgeRequests.keys().next().value!);
  }
  sendBridgePayload(request.context, request.message.type!, request.requestID, payload);
  return true;
}

async function handleBridgeMessage(event: MessageEvent): Promise<void> {
  if (
    !uiSession.value ||
    !configPlugin.value ||
    event.source !== pluginFrame.value?.contentWindow ||
    event.origin !== "null"
  )
    return;
  const message = event.data as PluginBridgeMessage;
  if (
    !isBridgeObject(message) ||
    message.source !== "sub2api-plugin-ui" ||
    message.bridge_token !== uiSession.value.bridge_token
  )
    return;

  const context: PluginBridgeContext = {
    pluginID: configPlugin.value.id,
    generation: configurationGeneration,
    session: uiSession.value,
    frame: pluginFrame.value!.contentWindow!,
  };

  const requestID = typeof message.request_id === "string" ? message.request_id.trim() : "";
  const expectsResponse =
    message.type === "config.load" ||
    message.type === "config.save" ||
    message.type === "config.test" ||
    message.type === "plugin.status" ||
    message.type === "plugin.action" ||
    message.type === "plugin.resources";
  let request: PluginBridgeRequest | undefined;
  if (expectsResponse) {
    if (!requestID || requestID.length > 128 || pendingBridgeRequests.has(requestID)) return;
    const completed = completedBridgeRequests.get(requestID);
    if (completed) {
      if (completed.type === message.type) sendBridgePayload(context, completed.type, requestID, completed.payload);
      return;
    }
    request = { context, message, requestID, timeout: 0 };
    const pending = request;
    request.timeout = window.setTimeout(() => {
      postBridgeResult(pending, { ok: false, error: t("admin.plugins.bridgeExpired") });
    }, 30_000);
    pendingBridgeRequests.set(requestID, request);
  }

  try {
    switch (message.type) {
      case "sub2api.plugin.ready":
        uiLoading.value = false;
        break;
      case "config.load": {
        const config = await adminAPI.plugins.getConfig(context.pluginID);
        postBridgeResult(request!, { ok: true, config });
        break;
      }
      case "config.save": {
        if (!isBridgeObject(message.config)) {
          throw new Error(t("admin.plugins.bridgeRejected"));
        }
        const input = message.config;
        const config = await pluginStepUp.run(() => {
          requirePendingBridge(request!);
          return adminAPI.plugins.saveConfig(context.pluginID, input);
        });
        if (postBridgeResult(request!, { ok: true, config })) appStore.showSuccess(t("common.saved"));
        break;
      }
      case "config.test": {
        const result = await pluginStepUp.run(() => {
          requirePendingBridge(request!);
          return adminAPI.plugins.test(context.pluginID);
        });
        // 成功结果交由插件展示，仅在当前会话测试失败时显示宿主提示。
        if (postBridgeResult(request!, { ok: result.success, result }) && !result.success)
          appStore.showError(result.message || t("common.error"));
        break;
      }
      case "plugin.resources": {
        requireCurrentBridge(context, true);
        const resources = await adminAPI.plugins.resources(context.pluginID);
        postBridgeResult(request!, { ok: true, resources });
        break;
      }
      case "plugin.action": {
        requireCurrentBridge(context, true);
        if (!isBridgeObject(message.action)) throw new Error(t("admin.plugins.bridgeRejected"));
        // 动作 ID 取自已验证的 Bridge 信封，插件正文不能覆盖它。
        const action = { ...message.action, request_id: requestID };
        const result = await pluginStepUp.run(() => {
          requirePendingBridge(request!, true);
          return adminAPI.plugins.action(context.pluginID, action);
        });
        postBridgeResult(request!, { ok: result.accepted, result });
        break;
      }
      case "plugin.status": {
        // 状态查询没有副作用，无需二次验证，结果由插件界面自行呈现。
        const result = await adminAPI.plugins.status(context.pluginID);
        postBridgeResult(request!, { ok: true, result });
        break;
      }
      case "ui.resize": {
        const height = Number(message.height);
        if (Number.isFinite(height))
          iframeHeight.value = Math.min(960, Math.max(520, Math.round(height)));
        break;
      }
      case "ui.notify": {
        const text =
          typeof message.message === "string"
            ? message.message.slice(0, 500)
            : "";
        if (!text) break;
        if (message.level === "error") appStore.showError(text);
        else if (message.level === "success") appStore.showSuccess(text);
        else appStore.showInfo(text);
        break;
      }
    }
  } catch (error: unknown) {
    if (!isCurrentBridge(context)) return;
    if (isStepUpBlocked(error)) reportSensitiveActionError(error);
    if (request) postBridgeResult(request, {
      ok: false,
      error: isStepUpCancelled(error) ? t("common.cancel") : errorMessage(error),
    });
  }
}

function stateClass(state: PluginInstallation["state"]): string {
  if (state === "enabled")
    return "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300";
  if (state === "error" || state === "incompatible")
    return "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300";
  if (state === "starting")
    return "bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300";
  return "bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300";
}

function compatibilityClass(
  status: PluginInstallation["compatibility"]["status"],
): string {
  if (status === "compatible")
    return "bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300";
  if (status === "untested")
    return "bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300";
  return "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300";
}

onMounted(() => {
  window.addEventListener("message", handleBridgeMessage);
  void loadPlugins();
});

onBeforeUnmount(() => {
  window.removeEventListener("message", handleBridgeMessage);
  invalidateBridgeSession();
});
</script>
